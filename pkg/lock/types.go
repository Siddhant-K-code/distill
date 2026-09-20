// Package lock builds and verifies deterministic, offline context artifacts.
package lock

const (
	ConfigSchemaVersion   = "distill-lock/config/v0"
	LockSchemaVersion     = "distill-lock/v0"
	ManifestSchemaVersion = "distill-lock/manifest/v0"

	ToolName        = "distill"
	ToolIdentity    = "github.com/Siddhant-K-code/distill/distill-lock-v0"
	RuntimeIdentity = "go1.24-go1.26"

	CanonicalizationIdentity = "utf8-nfc-lf-v1"
	ChunkingIdentity         = "utf8-fixed-bytes-v1"
	TokenEstimatorIdentity   = "utf8-bytes-div4-ceil-v1"
	DeduplicationIdentity    = "sha256-normalized-exact-v1"
	SelectionIdentity        = "lexical-budget-v1"
	BundleIdentity           = "markdown-bundle-v1"
	CanonicalJSONIdentity    = "go-struct-json-indent-v1"

	BundleFileName    = "context.bundle.md"
	LockFileName      = "context.lock.json"
	ManifestFileName  = "context.manifest.json"
	ChecksumsFileName = "SHA256SUMS"

	maxChunkBytes = 1 << 20
)

var outputFileNames = []string{
	ChecksumsFileName,
	BundleFileName,
	LockFileName,
	ManifestFileName,
}

// Config is the strict, canonical input configuration.
type Config struct {
	SchemaVersion   string   `json:"schema_version"`
	SourceRoot      string   `json:"source_root"`
	Sources         []string `json:"sources"`
	Exclude         []string `json:"exclude"`
	ChunkBytes      int      `json:"chunk_bytes"`
	TokenBudget     int      `json:"token_budget"`
	MetadataRemoval string   `json:"metadata_removal"`
}

// IdentitySet freezes every implementation identity that can affect output.
type IdentitySet struct {
	Tool             string `json:"tool"`
	ToolIdentity     string `json:"tool_identity"`
	Runtime          string `json:"runtime"`
	Canonicalization string `json:"canonicalization"`
	Chunking         string `json:"chunking"`
	TokenEstimator   string `json:"token_estimator"`
	Deduplication    string `json:"deduplication"`
	Selection        string `json:"selection"`
	Bundle           string `json:"bundle"`
	CanonicalJSON    string `json:"canonical_json"`
}

// Source records original and normalized identities for every file under the root.
type Source struct {
	Path             string `json:"path"`
	OriginalSHA256   string `json:"original_sha256"`
	OriginalBytes    int64  `json:"original_bytes"`
	NormalizedSHA256 string `json:"normalized_sha256"`
	NormalizedBytes  int64  `json:"normalized_bytes"`
	Included         bool   `json:"included"`
	Reason           string `json:"reason"`
}

// Chunk records a normalized byte range and its deterministic selection result.
type Chunk struct {
	Path            string `json:"path"`
	StartByte       int64  `json:"start_byte"`
	EndByte         int64  `json:"end_byte"`
	SHA256          string `json:"sha256"`
	EstimatedTokens int    `json:"estimated_tokens"`
	Selected        bool   `json:"selected"`
	SelectionOrder  int    `json:"selection_order"`
	Reason          string `json:"reason"`
}

// Stats is a compact deterministic summary.
type Stats struct {
	SourceCount    int `json:"source_count"`
	IncludedCount  int `json:"included_count"`
	ExcludedCount  int `json:"excluded_count"`
	ChunkCount     int `json:"chunk_count"`
	DuplicateCount int `json:"duplicate_count"`
	SelectedCount  int `json:"selected_count"`
	SelectedTokens int `json:"selected_tokens"`
}

// LockFile freezes the effective configuration, inventory, chunks, and selection.
type LockFile struct {
	SchemaVersion       string      `json:"schema_version"`
	Identities          IdentitySet `json:"identities"`
	Configuration       Config      `json:"configuration"`
	ConfigurationSHA256 string      `json:"configuration_sha256"`
	Sources             []Source    `json:"sources"`
	Chunks              []Chunk     `json:"chunks"`
	Stats               Stats       `json:"stats"`
}

// Output records one artifact's identity.
type Output struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// Manifest binds a lockfile to its rendered bundle.
type Manifest struct {
	SchemaVersion       string      `json:"schema_version"`
	Identities          IdentitySet `json:"identities"`
	ConfigurationSHA256 string      `json:"configuration_sha256"`
	LockSHA256          string      `json:"lock_sha256"`
	Sources             []Source    `json:"sources"`
	Chunks              []Chunk     `json:"chunks"`
	Stats               Stats       `json:"stats"`
	Bundle              Output      `json:"bundle"`
}

// Summary is printed by lock, build, and verify commands.
type Summary struct {
	SourceCount    int
	DuplicateCount int
	SelectedCount  int
	SelectedTokens int
	BundleSHA256   string
	LockSHA256     string
	ManifestSHA256 string
}

func currentIdentities() IdentitySet {
	return IdentitySet{
		Tool:             ToolName,
		ToolIdentity:     ToolIdentity,
		Runtime:          RuntimeIdentity,
		Canonicalization: CanonicalizationIdentity,
		Chunking:         ChunkingIdentity,
		TokenEstimator:   TokenEstimatorIdentity,
		Deduplication:    DeduplicationIdentity,
		Selection:        SelectionIdentity,
		Bundle:           BundleIdentity,
		CanonicalJSON:    CanonicalJSONIdentity,
	}
}
