package handoff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type verifiedCandidate struct {
	candidate Candidate
	patched   []byte
	ranges    []changedRange
}

func renderReview(request Request, proposal Proposal, requestSHA256, proposalSHA256 string) []byte {
	var buffer bytes.Buffer
	buffer.WriteString("# Distill Handoff Review\n\n")
	buffer.WriteString("> **Review required.** Distill verified this package but did not approve or apply any change.\n\n")
	fmt.Fprintf(&buffer, "- Request ID: `%s`\n", request.RequestID)
	fmt.Fprintf(&buffer, "- Request SHA-256: `%s`\n", requestSHA256)
	fmt.Fprintf(&buffer, "- Proposal SHA-256: `%s`\n", proposalSHA256)
	fmt.Fprintf(&buffer, "- Outcome: `%s`\n", proposal.Outcome)
	fmt.Fprintf(&buffer, "- Route: `%s`\n", proposal.Route)
	fmt.Fprintf(&buffer, "- Proposal reason: %s\n\n", quoted(proposal.Reason))
	if proposal.Outcome == OutcomeNoDecision {
		buffer.WriteString("## No committed decision found\n\n")
		buffer.WriteString("The external proposal reported no reviewable decision. Distill verified only the proposal envelope and frozen request relationship.\n")
		return buffer.Bytes()
	}

	for index, candidate := range proposal.Candidates {
		fmt.Fprintf(&buffer, "## %d. `%s`\n\n", index+1, candidate.ID)
		fmt.Fprintf(&buffer, "- Type: `%s`\n", candidate.DecisionType)
		fmt.Fprintf(&buffer, "- Route: `%s`\n", candidate.Route)
		fmt.Fprintf(&buffer, "- Target: %s\n", quoted(candidate.Target.Path))
		fmt.Fprintf(&buffer, "- Target original SHA-256: `%s`\n", candidate.Target.OriginalSHA256)
		fmt.Fprintf(&buffer, "- Target normalized SHA-256: `%s`\n", candidate.Target.NormalizedSHA256)
		fmt.Fprintf(&buffer, "- Patch SHA-256: `%s`\n", candidate.Patch.SHA256)
		fmt.Fprintf(&buffer, "- Evidence: lines %d-%d, bytes %d-%d\n\n",
			candidate.Evidence.StartLine, candidate.Evidence.EndLine,
			candidate.Evidence.StartByte, candidate.Evidence.EndByte,
		)
		buffer.WriteString("**Decision**\n\n")
		writeQuotedBlock(&buffer, candidate.DecisionText)
		buffer.WriteString("\n**Why review is required**\n\n")
		writeQuotedBlock(&buffer, candidate.Reason)
		buffer.WriteString("\n**Exact evidence**\n\n")
		writeIndented(&buffer, candidate.Evidence.Quote)
		buffer.WriteString("\n**Proposed unified diff**\n\n")
		writeIndented(&buffer, candidate.Patch.UnifiedDiff)
	}
	return buffer.Bytes()
}

func quoted(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func writeQuotedBlock(buffer *bytes.Buffer, value string) {
	for _, line := range strings.Split(value, "\n") {
		buffer.WriteString("> ")
		buffer.WriteString(line)
		buffer.WriteByte('\n')
	}
}

func writeIndented(buffer *bytes.Buffer, value string) {
	for _, line := range strings.SplitAfter(value, "\n") {
		if line == "" {
			continue
		}
		buffer.WriteString("    ")
		buffer.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			buffer.WriteByte('\n')
		}
	}
	buffer.WriteByte('\n')
}
