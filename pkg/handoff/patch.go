package handoff

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Siddhant-K-code/distill/internal/artifact"
)

var hunkHeaderPattern = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@(?: .*)?$`)

type changedRange struct {
	start int
	end   int
}

type patchLine struct {
	text    string
	newline bool
}

type patchHunk struct {
	oldStart int
	oldCount int
	newStart int
	newCount int
	body     []string
}

func parseAndApplyPatch(diff, targetPath string, target []byte) ([]byte, []changedRange, error) {
	if len(diff) > 1<<20 {
		return nil, nil, fmt.Errorf("patch exceeds the 1 MiB v0 alpha limit")
	}
	if !utf8.ValidString(diff) || strings.ContainsRune(diff, 0) {
		return nil, nil, fmt.Errorf("patch must be valid non-NUL UTF-8")
	}
	if strings.Contains(diff, "\r") || !strings.HasSuffix(diff, "\n") {
		return nil, nil, fmt.Errorf("patch must use LF and end with LF")
	}
	lines := strings.Split(strings.TrimSuffix(diff, "\n"), "\n")
	if len(lines) < 3 {
		return nil, nil, fmt.Errorf("patch is incomplete")
	}
	if lines[0] != "--- a/"+targetPath || lines[1] != "+++ b/"+targetPath {
		return nil, nil, fmt.Errorf("patch headers must target only a/%s and b/%s", targetPath, targetPath)
	}

	var hunks []patchHunk
	for index := 2; index < len(lines); {
		match := hunkHeaderPattern.FindStringSubmatch(lines[index])
		if match == nil {
			return nil, nil, fmt.Errorf("unsupported patch metadata or malformed hunk header %q", lines[index])
		}
		hunk, err := parseHunkHeader(match)
		if err != nil {
			return nil, nil, err
		}
		index++
		for index < len(lines) && !strings.HasPrefix(lines[index], "@@ ") {
			line := lines[index]
			if line == "" {
				return nil, nil, fmt.Errorf("patch body line lacks a unified-diff prefix")
			}
			switch line[0] {
			case ' ', '+', '-':
			default:
				return nil, nil, fmt.Errorf("unsupported patch body line %q", line)
			}
			hunk.body = append(hunk.body, line)
			index++
		}
		if len(hunk.body) == 0 {
			return nil, nil, fmt.Errorf("empty patch hunk")
		}
		hunks = append(hunks, hunk)
	}
	if len(hunks) == 0 {
		return nil, nil, fmt.Errorf("patch contains no hunks")
	}
	return applyHunks(hunks, target)
}

func parseHunkHeader(match []string) (patchHunk, error) {
	values := make([]int, 4)
	for index, group := range []string{match[1], match[2], match[3], match[4]} {
		if group == "" {
			values[index] = 1
			continue
		}
		value, err := strconv.Atoi(group)
		if err != nil {
			return patchHunk{}, fmt.Errorf("invalid hunk coordinate")
		}
		values[index] = value
	}
	if values[1] == 0 && match[2] == "" {
		values[1] = 1
	}
	if values[3] == 0 && match[4] == "" {
		values[3] = 1
	}
	if values[0] < 0 || values[1] < 0 || values[2] < 0 || values[3] < 0 {
		return patchHunk{}, fmt.Errorf("negative hunk coordinate")
	}
	if values[1] > 0 && values[0] == 0 || values[3] > 0 && values[2] == 0 {
		return patchHunk{}, fmt.Errorf("non-empty hunk ranges are 1-based")
	}
	return patchHunk{oldStart: values[0], oldCount: values[1], newStart: values[2], newCount: values[3]}, nil
}

func applyHunks(hunks []patchHunk, target []byte) ([]byte, []changedRange, error) {
	targetLines := splitPatchLines(target)
	result := make([]patchLine, 0, len(targetLines))
	cursor := 0
	delta := 0
	var ranges []changedRange
	changed := false

	for _, hunk := range hunks {
		start := hunk.oldStart
		if hunk.oldCount > 0 {
			start--
		}
		if start < cursor || start > len(targetLines) {
			return nil, nil, fmt.Errorf("patch hunks overlap, are unordered, or start outside the target")
		}
		expectedNewIndex := start + delta
		actualNewIndex := hunk.newStart
		if hunk.newCount > 0 {
			actualNewIndex--
		}
		if actualNewIndex != expectedNewIndex {
			return nil, nil, fmt.Errorf("new hunk coordinate does not match prior changes")
		}
		result = append(result, targetLines[cursor:start]...)
		cursor = start

		oldSeen := 0
		newSeen := 0
		for _, bodyLine := range hunk.body {
			prefix, text := bodyLine[0], bodyLine[1:]
			switch prefix {
			case ' ':
				if cursor >= len(targetLines) || targetLines[cursor].text != text {
					return nil, nil, fmt.Errorf("patch context does not match target")
				}
				if !targetLines[cursor].newline {
					return nil, nil, fmt.Errorf("patching an unterminated final line is unsupported")
				}
				result = append(result, targetLines[cursor])
				cursor++
				oldSeen++
				newSeen++
			case '-':
				if cursor >= len(targetLines) || targetLines[cursor].text != text {
					return nil, nil, fmt.Errorf("patch removal does not match target")
				}
				if !targetLines[cursor].newline {
					return nil, nil, fmt.Errorf("patching an unterminated final line is unsupported")
				}
				ranges = append(ranges, changedRange{start: cursor, end: cursor + 1})
				cursor++
				oldSeen++
				changed = true
			case '+':
				result = append(result, patchLine{text: text, newline: true})
				ranges = append(ranges, changedRange{start: cursor, end: cursor})
				newSeen++
				changed = true
			}
		}
		if oldSeen != hunk.oldCount || newSeen != hunk.newCount {
			return nil, nil, fmt.Errorf(
				"hunk body counts old=%d new=%d, header declares old=%d new=%d",
				oldSeen, newSeen, hunk.oldCount, hunk.newCount,
			)
		}
		delta += hunk.newCount - hunk.oldCount
	}
	if !changed {
		return nil, nil, fmt.Errorf("patch makes no changes")
	}
	result = append(result, targetLines[cursor:]...)
	return joinPatchLines(result), mergeRanges(ranges), nil
}

func splitPatchLines(data []byte) []patchLine {
	if len(data) == 0 {
		return nil
	}
	parts := bytes.SplitAfter(data, []byte("\n"))
	lines := make([]patchLine, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		newline := part[len(part)-1] == '\n'
		if newline {
			part = part[:len(part)-1]
		}
		lines = append(lines, patchLine{text: string(part), newline: newline})
	}
	return lines
}

func joinPatchLines(lines []patchLine) []byte {
	var buffer bytes.Buffer
	for _, line := range lines {
		buffer.WriteString(line.text)
		if line.newline {
			buffer.WriteByte('\n')
		}
	}
	return buffer.Bytes()
}

func mergeRanges(ranges []changedRange) []changedRange {
	if len(ranges) < 2 {
		return ranges
	}
	merged := []changedRange{ranges[0]}
	for _, current := range ranges[1:] {
		last := &merged[len(merged)-1]
		if current.start <= last.end {
			if current.end > last.end {
				last.end = current.end
			}
			continue
		}
		merged = append(merged, current)
	}
	return merged
}

func rangesOverlap(left, right changedRange) bool {
	if left.start == left.end && right.start == right.end {
		return left.start == right.start
	}
	if left.start == left.end {
		return left.start >= right.start && left.start <= right.end
	}
	if right.start == right.end {
		return right.start >= left.start && right.start <= left.end
	}
	return left.start < right.end && right.start < left.end
}

func validatePatchDigest(patch Patch) error {
	normalized, err := artifact.NormalizeText([]byte(patch.UnifiedDiff))
	if err != nil || string(normalized) != patch.UnifiedDiff {
		return fmt.Errorf("patch is not normalized UTF-8/LF")
	}
	if artifact.DigestBytes([]byte(patch.UnifiedDiff)) != patch.SHA256 {
		return fmt.Errorf("patch sha256 mismatch")
	}
	return nil
}
