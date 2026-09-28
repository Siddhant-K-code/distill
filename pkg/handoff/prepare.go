package handoff

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/Siddhant-K-code/distill/internal/artifact"
)

type preparedInput struct {
	identity InputIdentity
	data     []byte
}

func Prepare(conversationPath, docsDirectory, outputDirectory string) (Summary, error) {
	conversation, conversationAbsolute, err := readConversation(conversationPath)
	if err != nil {
		return Summary{}, err
	}
	docsRoot, documents, err := readDocuments(docsDirectory)
	if err != nil {
		return Summary{}, err
	}
	if inside, err := artifact.ExistingPathWithin(docsRoot, conversationAbsolute); err != nil {
		return Summary{}, fmt.Errorf("compare conversation and docs paths: %w", err)
	} else if inside {
		return Summary{}, fmt.Errorf("conversation must be outside the docs tree")
	}

	outputAbsolute, err := filepath.Abs(outputDirectory)
	if err != nil {
		return Summary{}, fmt.Errorf("resolve output path: %w", err)
	}
	outputParent, err := artifact.ResolveDirectory(filepath.Dir(outputAbsolute))
	if err != nil {
		return Summary{}, fmt.Errorf("resolve output parent: %w", err)
	}
	if inside, err := artifact.ExistingPathWithin(docsRoot, outputParent); err != nil {
		return Summary{}, fmt.Errorf("compare output and docs paths: %w", err)
	} else if inside {
		return Summary{}, fmt.Errorf("output directory must be outside the docs tree")
	}

	request := Request{
		SchemaVersion: RequestSchemaVersion,
		Identities:    currentIdentities(),
		Conversation:  conversation.identity,
		Documents:     make([]InputIdentity, len(documents)),
		Instructions: ArtifactIdentity{
			Path: InstructionsFileName, SHA256: artifact.DigestBytes(instructionsBytes), Bytes: int64(len(instructionsBytes)),
		},
		ProposalSchema: ArtifactIdentity{
			Path: ProposalSchemaName, SHA256: artifact.DigestBytes(proposalSchemaBytes), Bytes: int64(len(proposalSchemaBytes)),
		},
	}
	for index, document := range documents {
		request.Documents[index] = document.identity
	}
	request.RequestID, err = requestID(request)
	if err != nil {
		return Summary{}, err
	}
	requestBytes, err := artifact.CanonicalJSON(request)
	if err != nil {
		return Summary{}, err
	}

	files := map[string][]byte{
		RequestFileName:      requestBytes,
		InstructionsFileName: instructionsBytes,
		ProposalSchemaName:   proposalSchemaBytes,
		ConversationPath:     conversation.data,
	}
	for _, document := range documents {
		files[document.identity.BundlePath] = document.data
	}
	files[ChecksumsFileName] = artifact.RenderChecksums(files)
	requestSHA256 := artifact.DigestBytes(requestBytes)

	if err := artifact.PublishPrivateDirectory(outputDirectory, files, func(directory string) error {
		_, _, verifyErr := loadRequestBundle(filepath.Join(directory, RequestFileName), requestSHA256)
		return verifyErr
	}); err != nil {
		return Summary{}, err
	}
	return Summary{
		RequestID: request.RequestID, RequestSHA256: requestSHA256, DocumentCount: len(documents),
	}, nil
}

func readConversation(inputPath string) (preparedInput, string, error) {
	if filepath.Ext(inputPath) != ".md" {
		return preparedInput{}, "", fmt.Errorf(
			"conversation has unsupported file type %q; v0 alpha accepts only .md",
			filepath.Ext(inputPath),
		)
	}
	parent, err := artifact.ResolveDirectory(filepath.Dir(inputPath))
	if err != nil {
		return preparedInput{}, "", fmt.Errorf("resolve conversation parent: %w", err)
	}
	absolute := filepath.Join(parent, filepath.Base(inputPath))
	input, err := readMarkdownFile(absolute, ConversationPath, "conversation.md")
	if err != nil {
		return preparedInput{}, "", fmt.Errorf("conversation: %w", err)
	}
	return input, absolute, nil
}

func readDocuments(inputDirectory string) (string, []preparedInput, error) {
	root, err := artifact.ResolveDirectory(inputDirectory)
	if err != nil {
		return "", nil, fmt.Errorf("resolve docs directory: %w", err)
	}
	var documents []preparedInput
	canonicalPaths := make(map[string]string)
	var portablePaths []string
	err = filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filePath == root {
			return nil
		}
		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		portable := filepath.ToSlash(relative)
		canonical, err := artifact.CanonicalPortablePath(portable)
		if err != nil {
			return fmt.Errorf("docs path %q: %w", portable, err)
		}
		if canonical != portable {
			return fmt.Errorf("docs path %q is not canonical; use %q", portable, canonical)
		}
		if err := validateDisplayText("docs path", canonical, false, false); err != nil {
			return err
		}
		portablePaths = append(portablePaths, canonical)
		info, err := os.Lstat(filePath)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("docs path %q is a symbolic link", portable)
		}
		if info.IsDir() {
			return artifact.ValidateTrustedInfo(info, filePath, true)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("docs path %q is not a regular file", portable)
		}
		if previous, ok := canonicalPaths[canonical]; ok {
			return fmt.Errorf("duplicate canonical docs path %q from %q and %q", canonical, previous, portable)
		}
		canonicalPaths[canonical] = portable
		input, err := readMarkdownFile(filePath, "inputs/docs/"+canonical, canonical)
		if err != nil {
			return fmt.Errorf("document %q: %w", portable, err)
		}
		documents = append(documents, input)
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	if err := validateCaseFoldPaths(portablePaths); err != nil {
		return "", nil, err
	}
	if len(documents) == 0 {
		return "", nil, fmt.Errorf("docs directory contains no Markdown files")
	}
	sort.Slice(documents, func(i, j int) bool {
		return documents[i].identity.Path < documents[j].identity.Path
	})
	return root, documents, nil
}

func readMarkdownFile(filePath, bundlePath, logicalPath string) (preparedInput, error) {
	if filepath.Ext(logicalPath) != ".md" {
		return preparedInput{}, fmt.Errorf("unsupported file type %q; v0 alpha accepts only .md", filepath.Ext(logicalPath))
	}
	info, err := os.Lstat(filePath)
	if err != nil {
		return preparedInput{}, fmt.Errorf("inspect file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return preparedInput{}, fmt.Errorf("must be a regular file, not a symlink")
	}
	if err := artifact.ValidateTrustedInfo(info, filePath, false); err != nil {
		return preparedInput{}, err
	}
	original, err := artifact.ReadRegularFile(filePath, info)
	if err != nil {
		return preparedInput{}, fmt.Errorf("read file: %w", err)
	}
	normalized, err := artifact.NormalizeText(original)
	if err != nil {
		return preparedInput{}, fmt.Errorf("normalize file: %w", err)
	}
	if err := validateDisplayText("Markdown input", string(normalized), true, true); err != nil {
		return preparedInput{}, err
	}
	return preparedInput{
		identity: InputIdentity{
			Path: logicalPath, BundlePath: bundlePath,
			OriginalSHA256: artifact.DigestBytes(original), OriginalBytes: int64(len(original)),
			NormalizedSHA256: artifact.DigestBytes(normalized), NormalizedBytes: int64(len(normalized)),
		},
		data: normalized,
	}, nil
}

func requestID(request Request) (string, error) {
	request.RequestID = ""
	data, err := artifact.CanonicalJSON(request)
	if err != nil {
		return "", err
	}
	return artifact.DigestBytes(append([]byte("distill-handoff/request/v0alpha1\n"), data...)), nil
}

func validateRequest(request Request) error {
	if request.SchemaVersion != RequestSchemaVersion {
		return fmt.Errorf("schema_version: got %q, want %q", request.SchemaVersion, RequestSchemaVersion)
	}
	if !reflect.DeepEqual(request.Identities, currentIdentities()) {
		return fmt.Errorf("request implementation identities do not match this binary")
	}
	expectedID, err := requestID(request)
	if err != nil {
		return err
	}
	if request.RequestID != expectedID {
		return fmt.Errorf("request_id mismatch")
	}
	if request.Conversation.Path != "conversation.md" || request.Conversation.BundlePath != ConversationPath {
		return fmt.Errorf("conversation paths are invalid")
	}
	if len(request.Documents) == 0 {
		return fmt.Errorf("documents must not be empty")
	}
	if request.Instructions.Path != InstructionsFileName ||
		request.Instructions.SHA256 != artifact.DigestBytes(instructionsBytes) ||
		request.Instructions.Bytes != int64(len(instructionsBytes)) {
		return fmt.Errorf("instructions identity mismatch")
	}
	if request.ProposalSchema.Path != ProposalSchemaName ||
		request.ProposalSchema.SHA256 != artifact.DigestBytes(proposalSchemaBytes) ||
		request.ProposalSchema.Bytes != int64(len(proposalSchemaBytes)) {
		return fmt.Errorf("proposal schema identity mismatch")
	}
	previous := ""
	documentPaths := make([]string, 0, len(request.Documents))
	for _, document := range request.Documents {
		canonical, err := artifact.CanonicalPortablePath(document.Path)
		if err != nil {
			return fmt.Errorf("document path %q: %w", document.Path, err)
		}
		if canonical != document.Path || filepath.Ext(document.Path) != ".md" {
			return fmt.Errorf("document path %q is not canonical Markdown", document.Path)
		}
		if err := validateDisplayText("document path", document.Path, false, false); err != nil {
			return err
		}
		if document.Path <= previous {
			return fmt.Errorf("document paths must be unique and sorted")
		}
		if document.BundlePath != "inputs/docs/"+document.Path {
			return fmt.Errorf("document bundle path mismatch for %q", document.Path)
		}
		documentPaths = append(documentPaths, document.Path)
		previous = document.Path
	}
	if err := validateCaseFoldPaths(documentPaths); err != nil {
		return err
	}
	for _, input := range append([]InputIdentity{request.Conversation}, request.Documents...) {
		if !artifact.ValidDigest(input.OriginalSHA256) || !artifact.ValidDigest(input.NormalizedSHA256) ||
			input.OriginalBytes < 0 || input.NormalizedBytes < 0 {
			return fmt.Errorf("invalid input identity for %q", input.Path)
		}
	}
	return nil
}

func cleanReason(value string) bool {
	return strings.TrimSpace(value) != ""
}
