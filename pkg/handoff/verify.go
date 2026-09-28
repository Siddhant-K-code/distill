package handoff

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"

	"github.com/Siddhant-K-code/distill/internal/artifact"
)

func VerifyWithExpectedRequest(
	requestPath,
	proposalPath,
	outputDirectory,
	expectedRequestSHA256 string,
) (Summary, error) {
	if !artifact.ValidDigest(expectedRequestSHA256) {
		return Summary{}, fmt.Errorf("expected request SHA-256 must be 64 lowercase hexadecimal characters")
	}
	bundle, requestBytes, err := loadRequestBundle(requestPath, expectedRequestSHA256)
	if err != nil {
		return Summary{}, err
	}
	proposal, proposalBytes, err := loadProposal(proposalPath)
	if err != nil {
		return Summary{}, err
	}
	if err := validateProposalEnvelope(bundle, proposal); err != nil {
		return Summary{}, err
	}
	outputAbsolute, err := filepath.Abs(outputDirectory)
	if err != nil {
		return Summary{}, fmt.Errorf("resolve output path: %w", err)
	}
	outputParent, err := artifact.ResolveDirectory(filepath.Dir(outputAbsolute))
	if err != nil {
		return Summary{}, fmt.Errorf("resolve output parent: %w", err)
	}
	if inside, err := artifact.ExistingPathWithin(bundle.root, outputParent); err != nil {
		return Summary{}, fmt.Errorf("compare output and request paths: %w", err)
	} else if inside {
		return Summary{}, fmt.Errorf("output directory must be outside the immutable request bundle")
	}
	verified, err := verifyCandidates(bundle, proposal)
	if err != nil {
		return Summary{}, err
	}

	requestSHA256 := artifact.DigestBytes(requestBytes)
	proposalSHA256 := artifact.DigestBytes(proposalBytes)
	review := renderReview(bundle.request, proposal, requestSHA256, proposalSHA256)
	receipt := Receipt{
		SchemaVersion: ReceiptSchemaVersion, RequestID: bundle.request.RequestID,
		RequestSHA256: requestSHA256, ProposalSHA256: proposalSHA256,
		Outcome: proposal.Outcome, Route: RequireReview, Reason: proposal.Reason,
		CandidateCount: len(verified), Candidates: make([]CandidateReceipt, 0, len(verified)),
		Review:  ArtifactIdentity{Path: ReviewFileName, SHA256: artifact.DigestBytes(review), Bytes: int64(len(review))},
		Patches: make([]ArtifactIdentity, 0, len(verified)),
	}
	files := map[string][]byte{ReviewFileName: review}
	for _, result := range verified {
		patchPath := "patches/" + result.candidate.ID + ".patch"
		patchBytes := []byte(result.candidate.Patch.UnifiedDiff)
		files[patchPath] = patchBytes
		receipt.Patches = append(receipt.Patches, ArtifactIdentity{
			Path: patchPath, SHA256: artifact.DigestBytes(patchBytes), Bytes: int64(len(patchBytes)),
		})
		receipt.Candidates = append(receipt.Candidates, CandidateReceipt{
			ID: result.candidate.ID, DecisionType: result.candidate.DecisionType,
			Evidence: result.candidate.Evidence, Target: result.candidate.Target,
			PatchSHA256:             result.candidate.Patch.SHA256,
			PatchedNormalizedSHA256: artifact.DigestBytes(result.patched),
			Route:                   RequireReview,
		})
	}
	receiptBytes, err := artifact.CanonicalJSON(receipt)
	if err != nil {
		return Summary{}, err
	}
	files[ReceiptFileName] = receiptBytes
	files[ChecksumsFileName] = artifact.RenderChecksums(files)

	if err := artifact.PublishPrivateDirectory(outputDirectory, files, func(directory string) error {
		return verifyPublishedFiles(directory, files)
	}); err != nil {
		return Summary{}, err
	}
	return Summary{
		RequestID: bundle.request.RequestID, RequestSHA256: requestSHA256,
		ProposalSHA256: proposalSHA256, ReceiptSHA256: artifact.DigestBytes(receiptBytes),
		Outcome: proposal.Outcome, DocumentCount: len(bundle.request.Documents), CandidateCount: len(verified),
	}, nil
}

func verifyCandidates(bundle *requestBundle, proposal Proposal) ([]verifiedCandidate, error) {
	results := make([]verifiedCandidate, 0, len(proposal.Candidates))
	seenIDs := make(map[string]bool)
	seenPatches := make(map[string]bool)
	seenSemantic := make(map[string]bool)
	rangesByTarget := make(map[string][]changedRange)
	documentIdentities := make(map[string]InputIdentity, len(bundle.request.Documents))
	for _, document := range bundle.request.Documents {
		documentIdentities[document.Path] = document
	}

	for _, candidate := range proposal.Candidates {
		if err := validateCandidateShape(candidate); err != nil {
			return nil, err
		}
		if seenIDs[candidate.ID] {
			return nil, fmt.Errorf("duplicate candidate id %q", candidate.ID)
		}
		seenIDs[candidate.ID] = true
		if seenPatches[candidate.Patch.SHA256] {
			return nil, fmt.Errorf("duplicate patch digest %q", candidate.Patch.SHA256)
		}
		seenPatches[candidate.Patch.SHA256] = true
		semanticKey := fmt.Sprintf("%s\x00%d\x00%d\x00%s", candidate.DecisionText,
			candidate.Evidence.StartByte, candidate.Evidence.EndByte, candidate.Target.Path)
		if seenSemantic[semanticKey] {
			return nil, fmt.Errorf("duplicate semantic candidate %q", candidate.ID)
		}
		seenSemantic[semanticKey] = true

		if err := verifyEvidence(bundle, candidate); err != nil {
			return nil, err
		}
		identity, ok := documentIdentities[candidate.Target.Path]
		if !ok {
			return nil, fmt.Errorf("candidate %q target is not in the frozen docs tree", candidate.ID)
		}
		if candidate.Target.OriginalSHA256 != identity.OriginalSHA256 ||
			candidate.Target.NormalizedSHA256 != identity.NormalizedSHA256 {
			return nil, fmt.Errorf("candidate %q target digest is stale or fabricated", candidate.ID)
		}
		if err := validatePatchDigest(candidate.Patch); err != nil {
			return nil, fmt.Errorf("candidate %q: %w", candidate.ID, err)
		}
		patched, ranges, err := parseAndApplyPatch(
			candidate.Patch.UnifiedDiff, candidate.Target.Path, bundle.documents[candidate.Target.Path],
		)
		if err != nil {
			return nil, fmt.Errorf("candidate %q patch: %w", candidate.ID, err)
		}
		for _, existing := range rangesByTarget[candidate.Target.Path] {
			for _, current := range ranges {
				if rangesOverlap(existing, current) {
					return nil, fmt.Errorf("candidate %q patch overlaps another candidate for %q", candidate.ID, candidate.Target.Path)
				}
			}
		}
		rangesByTarget[candidate.Target.Path] = append(rangesByTarget[candidate.Target.Path], ranges...)
		results = append(results, verifiedCandidate{candidate: candidate, patched: patched, ranges: ranges})
	}
	return results, nil
}

func verifyEvidence(bundle *requestBundle, candidate Candidate) error {
	evidence := candidate.Evidence
	if evidence.SourceNormalizedSHA256 != bundle.request.Conversation.NormalizedSHA256 {
		return fmt.Errorf("candidate %q evidence source digest mismatch", candidate.ID)
	}
	if evidence.StartByte < 0 || evidence.EndByte <= evidence.StartByte ||
		evidence.EndByte > int64(len(bundle.conversation)) {
		return fmt.Errorf("candidate %q evidence byte range is invalid", candidate.ID)
	}
	start, end := int(evidence.StartByte), int(evidence.EndByte)
	if !utf8.Valid(bundle.conversation[:start]) || !utf8.Valid(bundle.conversation[:end]) {
		return fmt.Errorf("candidate %q evidence range is not on UTF-8 boundaries", candidate.ID)
	}
	if !bytes.Equal(bundle.conversation[start:end], []byte(evidence.Quote)) {
		return fmt.Errorf("candidate %q evidence quote does not match exact conversation bytes", candidate.ID)
	}
	startLine := 1 + bytes.Count(bundle.conversation[:start], []byte("\n"))
	endLine := 1 + bytes.Count(bundle.conversation[:end-1], []byte("\n"))
	if evidence.StartLine != startLine || evidence.EndLine != endLine {
		return fmt.Errorf("candidate %q evidence line range mismatch; want %d-%d", candidate.ID, startLine, endLine)
	}
	return nil
}

func verifyPublishedFiles(directory string, expected map[string][]byte) error {
	rootInfo, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if err := artifact.ValidatePrivateInfo(rootInfo, directory, true); err != nil {
		return err
	}
	actual := make(map[string][]byte, len(expected))
	expectedDirectories := allowedDirectories(expected)
	err = filepath.WalkDir(directory, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filePath == directory {
			return nil
		}
		relative, err := filepath.Rel(directory, filePath)
		if err != nil {
			return err
		}
		portable := filepath.ToSlash(relative)
		info, err := os.Lstat(filePath)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("published path %q is a symbolic link", portable)
		}
		if info.IsDir() {
			if _, ok := expectedDirectories[portable]; !ok {
				return fmt.Errorf("unexpected published directory %q", portable)
			}
			return artifact.ValidatePrivateInfo(info, filePath, true)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("published path %q is not regular", portable)
		}
		if err := artifact.ValidatePrivateInfo(info, filePath, false); err != nil {
			return err
		}
		if _, ok := expected[portable]; !ok {
			return fmt.Errorf("unexpected published file %q", portable)
		}
		data, err := artifact.ReadRegularFile(filePath, info)
		if err != nil {
			return err
		}
		actual[portable] = data
		return nil
	})
	if err != nil {
		return err
	}
	if len(actual) != len(expected) {
		return fmt.Errorf("published file count mismatch")
	}
	names := make([]string, 0, len(expected))
	for name, expectedData := range expected {
		if !bytes.Equal(actual[name], expectedData) {
			return fmt.Errorf("published file %q bytes mismatch", name)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if _, err := parseChecksums(actual[ChecksumsFileName], len(expected)-1); err != nil {
		return err
	}
	return nil
}
