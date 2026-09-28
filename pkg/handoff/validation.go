package handoff

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
)

var unicodeCaseFolder = cases.Fold()

func validateDisplayText(label, value string, allowNewline, allowTab bool) error {
	for _, r := range value {
		if r == '\n' && allowNewline || r == '\t' && allowTab {
			continue
		}
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return fmt.Errorf("%s contains unsafe control or format character U+%04X", label, r)
		}
	}
	return nil
}

func validateCaseFoldPaths(paths []string) error {
	seen := make(map[string]string)
	for _, portable := range paths {
		parts := strings.Split(portable, "/")
		for length := 1; length <= len(parts); length++ {
			candidate := strings.Join(parts[:length], "/")
			folded := unicodeCaseFolder.String(candidate)
			if previous, ok := seen[folded]; ok && previous != candidate {
				return fmt.Errorf("case-folded path collision between %q and %q", previous, candidate)
			}
			seen[folded] = candidate
		}
	}
	return nil
}
