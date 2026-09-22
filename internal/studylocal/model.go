package studylocal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	ModelManifestSchema                  = SchemaVersion + "/model-manifest"
	ModelFileCount                       = 8
	ModelTotalBytes                int64 = 2_274_515_269
	ConversionManifestSHA256             = "2f796802ff529150c0b5b7325c3abebd2dcd510df3b5e84ab2581513c7358f05"
	ConverterPackage                     = "mlx-lm"
	ConverterVersion                     = "0.31.3"
	ConverterRevision                    = "ed1fca4cef15a824c5f1702c80f70b4cffc8e4dd"
	RuntimeIdentitySHA256                = "391e14ce1b09b5de11b96ab24f98dd73c4c8334625599e1cda5dd5f4906c16a5"
	RuntimeClosureSHA256                 = "119a039f08656f87e0a847a2ec9d9ba0633c12418fd5e08c469e67a262e46997"
	RuntimeRequirementsSHA256            = "779c01eeabc50358a94c00d204dbe02526c19165505fd0904c1ed37583a8fee1"
	RuntimeWheelVerificationSHA256       = "b628d0b295cb4ce1cac3c51489930281581b9de4b7ad8e168d2e154a6cc98d3a"
	RuntimeVenvManifestSHA256            = "0941d4489820fffe045875d9e192378af135c8d0ca7d1d6c4f4b240d74b0f222"
	BasePythonBinarySHA256               = "4e1dfb03f82c5f7f253bbc3c04a79bb9f09a5cd0528829c32d6984ef309ebb2f"
	BasePythonManifestSHA256             = "295be0913c3d0390eccd7ffbc456874677a8e7b417de8bd90cae13102cc5a9d5"
	ModelFileManifestSHA256              = "922cf680fd50d20b0797493cbc32d230419de37567638f503256658c57257b1f"
)

type ModelFile struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

type ModelManifest struct {
	SchemaVersion            string      `json:"schema_version"`
	OfficialModelID          string      `json:"official_model_id"`
	OfficialRevision         string      `json:"official_revision"`
	Format                   string      `json:"format"`
	QuantizationMode         string      `json:"quantization_mode"`
	QuantizationBits         int         `json:"quantization_bits"`
	QuantizationGroupSize    int         `json:"quantization_group_size"`
	ConverterPackage         string      `json:"converter_package"`
	ConverterVersion         string      `json:"converter_version"`
	ConverterRevision        string      `json:"converter_revision"`
	ConversionManifestSHA256 string      `json:"conversion_manifest_sha256"`
	FileManifestSHA256       string      `json:"file_manifest_sha256"`
	Files                    []ModelFile `json:"files"`
	FileCount                int         `json:"file_count"`
	TotalBytes               int64       `json:"total_bytes"`
	ArtifactSHA256           string      `json:"artifact_sha256"`
	ManifestSHA256           string      `json:"manifest_sha256"`
}

var expectedModelFiles = []ModelFile{
	{Path: "README.md", SizeBytes: 81, SHA256: "1a127c49a60fb70816260c037e7b54b3b5831e05ba4e4427dd016bf579247943"},
	{Path: "chat_template.jinja", SizeBytes: 4168, SHA256: "a55ee1b1660128b7098723e0abcd92caa0788061051c62d51cbe87d9cf1974d8"},
	{Path: "config.json", SizeBytes: 1021, SHA256: "cd08d43fc6f59e643b58b114d308a29bbd4ca147d30df6f419fcbe3115dcc2dd"},
	{Path: "generation_config.json", SizeBytes: 239, SHA256: "2325da0f15bb848e018c5ae071b7943332e9f871d6b60e2ed22ca97d4cb993d2"},
	{Path: "model.safetensors", SizeBytes: 2_263_022_417, SHA256: "365a6fff57490ca4e89e354c3295b7f2f60ac3fcb0a8a3fa06e21b80bb4d19ca"},
	{Path: "model.safetensors.index.json", SizeBytes: 63_964, SHA256: "388d811b8b7c2608dd04cce1bcb04a8bf715d19b42790894e6d3427ff429a777"},
	{Path: "tokenizer.json", SizeBytes: 11_422_650, SHA256: "be75606093db2094d7cd20f3c2f385c212750648bd6ea4fb2bf507a6a4c55506"},
	{Path: "tokenizer_config.json", SizeBytes: 729, SHA256: "ee8f6d44bf2353e6d3686c3adaf70e1ccfe9e6ed6822d0ab2f28cafdd7754792"},
}

func BuildModelManifest(modelDirectory string) (ModelManifest, error) {
	root, err := safeExactDirectory(modelDirectory)
	if err != nil {
		return ModelManifest{}, fmt.Errorf("model directory: %w", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return ModelManifest{}, err
	}
	if len(entries) != ModelFileCount {
		return ModelManifest{}, fmt.Errorf("model directory has %d entries, want %d", len(entries), ModelFileCount)
	}
	files := make([]ModelFile, 0, len(entries))
	var total int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return ModelManifest{}, err
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || info.Mode().Perm()&0o022 != 0 {
			return ModelManifest{}, fmt.Errorf("model entry %q is not a trusted regular file", entry.Name())
		}
		if err := requireCurrentOwner(info); err != nil {
			return ModelManifest{}, err
		}
		path := filepath.Join(root, entry.Name())
		before := fileIdentityFromInfo(info)
		digest, err := digestRegularFile(path, info)
		if err != nil {
			return ModelManifest{}, err
		}
		after, err := os.Lstat(path)
		if err != nil || !os.SameFile(info, after) || before != fileIdentityFromInfo(after) {
			return ModelManifest{}, fmt.Errorf("model entry %q changed while hashing", entry.Name())
		}
		files = append(files, ModelFile{Path: entry.Name(), SizeBytes: info.Size(), SHA256: digest})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	if fmt.Sprint(files) != fmt.Sprint(expectedModelFiles) || total != ModelTotalBytes {
		return ModelManifest{}, fmt.Errorf("model artifact differs from the frozen eight-file conversion receipt")
	}
	fileContract, err := frozenModelContractJSON(files)
	if err != nil || DigestBytes(fileContract) != TargetModelArtifactSHA256 {
		return ModelManifest{}, fmt.Errorf("model artifact aggregate digest mismatch")
	}
	var fileManifest strings.Builder
	for _, file := range files {
		fmt.Fprintf(&fileManifest, "%s\t%d\t%s\n", file.Path, file.SizeBytes, file.SHA256)
	}
	if DigestBytes([]byte(fileManifest.String())) != ModelFileManifestSHA256 {
		return ModelManifest{}, fmt.Errorf("model per-file manifest digest mismatch")
	}
	if err := validateModelConfiguration(root); err != nil {
		return ModelManifest{}, err
	}
	manifest := ModelManifest{
		SchemaVersion: ModelManifestSchema, OfficialModelID: TargetModelID,
		OfficialRevision: TargetModelRevision, Format: TargetModelFormat,
		QuantizationMode: "affine", QuantizationBits: 4, QuantizationGroupSize: 64,
		ConverterPackage: ConverterPackage, ConverterVersion: ConverterVersion,
		ConverterRevision: ConverterRevision, ConversionManifestSHA256: ConversionManifestSHA256,
		FileManifestSHA256: ModelFileManifestSHA256,
		Files:              files, FileCount: len(files), TotalBytes: total,
		ArtifactSHA256: TargetModelArtifactSHA256,
	}
	manifest.ManifestSHA256, _ = DigestDomain("model-manifest", modelManifestProjection(manifest))
	if err := validateModelManifestRecord(manifest); err != nil {
		return ModelManifest{}, err
	}
	return manifest, nil
}

// frozenModelContractJSON reproduces the byte format bound by
// TargetModelArtifactSHA256: sorted object keys, two-space indentation, and one
// trailing LF. It is intentionally separate from this study's RFC 8785 identities.
func frozenModelContractJSON(files []ModelFile) ([]byte, error) {
	contract := make([]map[string]any, len(files))
	for i, file := range files {
		contract[i] = map[string]any{
			"path":       file.Path,
			"sha256":     file.SHA256,
			"size_bytes": file.SizeBytes,
		}
	}
	data, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func ValidateModelManifest(modelDirectory string, manifest ModelManifest) error {
	actual, err := BuildModelManifest(modelDirectory)
	if err != nil {
		return err
	}
	expected, _ := canonicalJSON(actual)
	received, err := canonicalJSON(manifest)
	if err != nil || !bytesEqual(expected, received) {
		return fmt.Errorf("model manifest does not match the verified local artifact")
	}
	return nil
}

func WriteModelManifest(modelDirectory, outputPath string) (ModelManifest, error) {
	manifest, err := BuildModelManifest(modelDirectory)
	if err != nil {
		return ModelManifest{}, err
	}
	data, err := canonicalJSONFile(manifest)
	if err != nil {
		return ModelManifest{}, err
	}
	if outputPath == "" || filepath.Clean(outputPath) != outputPath {
		return ModelManifest{}, fmt.Errorf("unsafe model-manifest output")
	}
	if err := writeExclusive(outputPath, data, 0o600); err != nil {
		return ModelManifest{}, err
	}
	return manifest, nil
}

func ReadModelManifest(path string) (ModelManifest, []byte, error) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
		info.Mode().Perm()&0o077 != 0 {
		return ModelManifest{}, nil, fmt.Errorf("model manifest must be an owner-only regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ModelManifest{}, nil, err
	}
	var manifest ModelManifest
	if err := strictDecode(data, &manifest); err != nil {
		return ModelManifest{}, nil, err
	}
	return manifest, data, nil
}

func modelManifestProjection(manifest ModelManifest) ModelManifest {
	manifest.ManifestSHA256 = ""
	return manifest
}

func validateModelManifestRecord(manifest ModelManifest) error {
	if manifest.SchemaVersion != ModelManifestSchema ||
		manifest.OfficialModelID != TargetModelID ||
		manifest.OfficialRevision != TargetModelRevision ||
		manifest.Format != TargetModelFormat ||
		manifest.QuantizationMode != "affine" || manifest.QuantizationBits != 4 ||
		manifest.QuantizationGroupSize != 64 ||
		manifest.ConverterPackage != ConverterPackage ||
		manifest.ConverterVersion != ConverterVersion ||
		manifest.ConverterRevision != ConverterRevision ||
		manifest.ConversionManifestSHA256 != ConversionManifestSHA256 ||
		manifest.FileManifestSHA256 != ModelFileManifestSHA256 ||
		manifest.FileCount != ModelFileCount || manifest.TotalBytes != ModelTotalBytes ||
		manifest.ArtifactSHA256 != TargetModelArtifactSHA256 ||
		fmt.Sprint(manifest.Files) != fmt.Sprint(expectedModelFiles) {
		return fmt.Errorf("model manifest constants drifted")
	}
	digest, _ := DigestDomain("model-manifest", modelManifestProjection(manifest))
	if digest != manifest.ManifestSHA256 {
		return fmt.Errorf("model manifest digest mismatch")
	}
	return nil
}

func validateModelConfiguration(root string) error {
	data, err := os.ReadFile(filepath.Join(root, "config.json"))
	if err != nil {
		return err
	}
	var config struct {
		ModelType    string `json:"model_type"`
		Quantization struct {
			GroupSize int    `json:"group_size"`
			Bits      int    `json:"bits"`
			Mode      string `json:"mode"`
		} `json:"quantization"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("model config: %w", err)
	}
	if config.ModelType != "qwen3" || config.Quantization.GroupSize != 64 ||
		config.Quantization.Bits != 4 || config.Quantization.Mode != "affine" {
		return fmt.Errorf("model configuration does not match qwen3 affine int4 group64")
	}
	return nil
}

type fileIdentity struct {
	mode    os.FileMode
	size    int64
	modTime int64
}

func fileIdentityFromInfo(info os.FileInfo) fileIdentity {
	return fileIdentity{mode: info.Mode(), size: info.Size(), modTime: info.ModTime().UnixNano()}
}

func digestRegularFile(path string, expected os.FileInfo) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	if info, err := file.Stat(); err != nil || fileIdentityFromInfo(info) != fileIdentityFromInfo(expected) {
		return "", fmt.Errorf("file changed while hashing")
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func safeExactDirectory(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute path required")
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
		return "", fmt.Errorf("directory is missing, symlinked, or group/world writable")
	}
	if err := requireCurrentOwner(info); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return "", fmt.Errorf("directory path contains a symbolic link")
	}
	return path, nil
}

func bytesEqual(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var different byte
	for i := range left {
		different |= left[i] ^ right[i]
	}
	return different == 0
}
