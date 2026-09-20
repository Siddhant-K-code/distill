package lock

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

func loadConfig(configPath string) (Config, []byte, error) {
	info, err := os.Lstat(configPath)
	if err != nil {
		return Config{}, nil, fmt.Errorf("inspect config: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Config{}, nil, fmt.Errorf("config must be a regular file, not a symlink")
	}
	if err := validateTrustedInfo(info, configPath, false); err != nil {
		return Config{}, nil, err
	}
	data, err := readRegularFile(configPath, info)
	if err != nil {
		return Config{}, nil, fmt.Errorf("read config: %w", err)
	}

	var config Config
	if err := decodeCanonicalJSON(data, &config); err != nil {
		return Config{}, nil, fmt.Errorf("config %q: %w", configPath, err)
	}
	if err := validateConfig(config); err != nil {
		return Config{}, nil, fmt.Errorf("config %q: %w", configPath, err)
	}
	return config, data, nil
}

func validateConfig(config Config) error {
	if config.SchemaVersion != ConfigSchemaVersion {
		return fmt.Errorf("schema_version: got %q, want %q", config.SchemaVersion, ConfigSchemaVersion)
	}
	canonicalRoot, err := canonicalPortablePath(config.SourceRoot)
	if err != nil {
		return fmt.Errorf("source_root: %w", err)
	}
	if canonicalRoot != config.SourceRoot {
		return fmt.Errorf("source_root %q is not canonical; use %q", config.SourceRoot, canonicalRoot)
	}
	if config.SourceRoot == "." {
		return fmt.Errorf("source_root: current directory is not allowed")
	}
	if config.ChunkBytes < 1 || config.ChunkBytes > maxChunkBytes {
		return fmt.Errorf("chunk_bytes: must be between 1 and %d", maxChunkBytes)
	}
	if config.TokenBudget < 0 {
		return fmt.Errorf("token_budget: must be non-negative")
	}
	if config.MetadataRemoval != "none" {
		return fmt.Errorf("metadata_removal: v0 supports only %q", "none")
	}
	if len(config.Sources) == 0 {
		return fmt.Errorf("sources: at least one source is required")
	}

	seen := make(map[string]string, len(config.Sources)+len(config.Exclude))
	for _, item := range config.Sources {
		normalized, err := canonicalPortablePath(item)
		if err != nil {
			return fmt.Errorf("sources entry %q: %w", item, err)
		}
		if normalized != item {
			return fmt.Errorf("sources entry %q is not canonical; use %q", item, normalized)
		}
		if previous, ok := seen[item]; ok {
			return fmt.Errorf("duplicate canonical path %q in %s and sources", item, previous)
		}
		seen[item] = "sources"
	}
	for _, item := range config.Exclude {
		normalized, err := canonicalPortablePath(item)
		if err != nil {
			return fmt.Errorf("exclude entry %q: %w", item, err)
		}
		if normalized != item {
			return fmt.Errorf("exclude entry %q is not canonical; use %q", item, normalized)
		}
		if previous, ok := seen[item]; ok {
			return fmt.Errorf("duplicate canonical path %q in %s and exclude", item, previous)
		}
		seen[item] = "exclude"
	}
	if !sort.StringsAreSorted(config.Sources) {
		return fmt.Errorf("sources: entries must be sorted by bytewise lexical order")
	}
	if !sort.StringsAreSorted(config.Exclude) {
		return fmt.Errorf("exclude: entries must be sorted by bytewise lexical order")
	}
	return nil
}

func canonicalPortablePath(value string) (string, error) {
	if err := validatePortablePath(value); err != nil {
		return "", err
	}
	return norm.NFC.String(value), nil
}

func validatePortablePath(value string) error {
	if value == "" {
		return fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(value) || path.IsAbs(value) {
		return fmt.Errorf("absolute paths are not allowed")
	}
	if strings.Contains(value, `\`) {
		return fmt.Errorf("backslashes are not allowed; use POSIX separators")
	}
	if value != path.Clean(value) {
		return fmt.Errorf("path is not lexically normalized")
	}
	if value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("path traversal is not allowed")
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("empty, dot, and dot-dot segments are not allowed")
		}
	}
	for _, r := range value {
		if r == 0 || unicode.IsControl(r) {
			return fmt.Errorf("control characters are not allowed")
		}
	}
	return nil
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
