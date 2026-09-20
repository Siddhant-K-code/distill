package lock

import (
	"bytes"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestRandomizedEnumerationOrderIsDeterministic(t *testing.T) {
	config := testConfig([]string{"a.txt", "b.txt", "c.txt"}, nil)
	inputs := []sourceInput{
		testInput(t, "c.txt", "charlie\n"),
		testInput(t, "a.txt", "alpha\n"),
		testInput(t, "b.txt", "alpha\n"),
	}
	baseline, _, err := buildLock(config, inputs)
	if err != nil {
		t.Fatal(err)
	}
	baselineBytes, err := canonicalJSON(baseline)
	if err != nil {
		t.Fatal(err)
	}

	random := rand.New(rand.NewSource(20260920))
	for iteration := 0; iteration < 100; iteration++ {
		shuffled := append([]sourceInput(nil), inputs...)
		random.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		actual, _, err := buildLock(config, shuffled)
		if err != nil {
			t.Fatalf("iteration %d: %v", iteration, err)
		}
		actualBytes, err := canonicalJSON(actual)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(actualBytes, baselineBytes) {
			t.Fatalf("iteration %d produced different lock bytes", iteration)
		}
	}
}

func TestDifferentWorkingPathsProduceIdenticalArtifacts(t *testing.T) {
	first := copyFixture(t, filepath.Join(t.TempDir(), "short"))
	second := copyFixture(t, filepath.Join(t.TempDir(), "a", "much", "longer", "workspace", "path"))

	firstFiles := buildFixture(t, first, "out")
	secondFiles := buildFixture(t, second, "out")
	if !reflect.DeepEqual(firstFiles, secondFiles) {
		t.Fatal("portable artifacts differ across working-directory paths")
	}
	for name, data := range firstFiles {
		if bytes.Contains(data, []byte(first)) || bytes.Contains(data, []byte(second)) {
			t.Fatalf("%s contains an absolute workspace path", name)
		}
	}
}

func TestLineEndingsNormalizeButOriginalHashesRemainDistinct(t *testing.T) {
	root := copyFixture(t, filepath.Join(t.TempDir(), "fixture"))
	lockPath := filepath.Join(root, LockFileName)
	if _, err := Create(filepath.Join(root, "config.json"), lockPath); err != nil {
		t.Fatal(err)
	}
	_, lockFile, err := readLockFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}

	lf := findSource(t, lockFile, "line-endings/lf.txt")
	crlf := findSource(t, lockFile, "line-endings/crlf.txt")
	if lf.OriginalSHA256 == crlf.OriginalSHA256 {
		t.Fatal("LF and CRLF original hashes unexpectedly match")
	}
	if lf.NormalizedSHA256 != crlf.NormalizedSHA256 || lf.NormalizedBytes != crlf.NormalizedBytes {
		t.Fatal("LF and CRLF normalized identities differ")
	}

	crlfChunk := findChunk(t, lockFile, crlf.Path)
	if !crlfChunk.Selected {
		t.Fatal("lexically first CRLF chunk was not the exact-duplicate representative")
	}
	if !strings.HasPrefix(findChunk(t, lockFile, lf.Path).Reason, "exact_duplicate_of:") {
		t.Fatal("LF-equivalent chunk was not marked as an exact duplicate")
	}
}

func TestUnicodeNormalizesToNFC(t *testing.T) {
	decomposed := []byte("Cafe\u0301\n")
	composed := []byte("Caf\u00e9\n")
	first, err := normalizeText(decomposed)
	if err != nil {
		t.Fatal(err)
	}
	second, err := normalizeText(composed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("NFC forms differ: %q != %q", first, second)
	}
	if digestBytes(decomposed) == digestBytes(composed) {
		t.Fatal("original Unicode hashes unexpectedly match")
	}
}

func TestRelevantMutationChangesSourceChunkAndBundle(t *testing.T) {
	config := testConfig([]string{"code.go"}, nil)
	beforeInput := testInput(t, "code.go", "package p\nconst answer = 42\n")
	afterInput := testInput(t, "code.go", "package p\nconst answer = 43\n")

	before, beforeContents, err := buildLock(config, []sourceInput{beforeInput})
	if err != nil {
		t.Fatal(err)
	}
	after, afterContents, err := buildLock(config, []sourceInput{afterInput})
	if err != nil {
		t.Fatal(err)
	}
	beforeBundle, err := renderBundle(before.Chunks, beforeContents)
	if err != nil {
		t.Fatal(err)
	}
	afterBundle, err := renderBundle(after.Chunks, afterContents)
	if err != nil {
		t.Fatal(err)
	}

	if before.Sources[0].NormalizedSHA256 == after.Sources[0].NormalizedSHA256 {
		t.Fatal("source normalized hash did not change")
	}
	if before.Chunks[0].SHA256 == after.Chunks[0].SHA256 {
		t.Fatal("chunk hash did not change")
	}
	if digestBytes(beforeBundle) == digestBytes(afterBundle) {
		t.Fatal("bundle hash did not change")
	}
}

func TestExcludedMutationDoesNotChangeBundle(t *testing.T) {
	config := testConfig([]string{"selected.txt"}, []string{"ignored.txt"})
	beforeInputs := []sourceInput{
		testInput(t, "ignored.txt", "ignored before\n"),
		testInput(t, "selected.txt", "selected\n"),
	}
	afterInputs := []sourceInput{
		testInput(t, "ignored.txt", "ignored after\n"),
		testInput(t, "selected.txt", "selected\n"),
	}
	before, beforeContents, err := buildLock(config, beforeInputs)
	if err != nil {
		t.Fatal(err)
	}
	after, afterContents, err := buildLock(config, afterInputs)
	if err != nil {
		t.Fatal(err)
	}
	beforeBundle, _ := renderBundle(before.Chunks, beforeContents)
	afterBundle, _ := renderBundle(after.Chunks, afterContents)

	if !bytes.Equal(beforeBundle, afterBundle) {
		t.Fatal("excluded source mutation changed the bundle")
	}
	if before.Sources[0].OriginalSHA256 == after.Sources[0].OriginalSHA256 {
		t.Fatal("excluded source mutation was not recorded in inventory")
	}
	if before.Sources[0].Reason != "configured_exclusion" || after.Sources[0].Reason != "configured_exclusion" {
		t.Fatal("excluded source reason changed")
	}
}

func TestBuildRejectsLockedInputDrift(t *testing.T) {
	tests := map[string]func(t *testing.T, root string){
		"missing": func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "sources", "code", "example.go")); err != nil {
				t.Fatal(err)
			}
		},
		"changed": func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, "sources", "code", "example.go"), []byte("package example\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"unexpected": func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, "sources", "new.txt"), []byte("new\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root := copyFixture(t, filepath.Join(t.TempDir(), "fixture"))
			lockPath := filepath.Join(root, LockFileName)
			if _, err := Create(filepath.Join(root, "config.json"), lockPath); err != nil {
				t.Fatal(err)
			}
			mutate(t, root)
			output := filepath.Join(root, "output")
			if _, err := Build(lockPath, output); err == nil {
				t.Fatal("build succeeded after locked input drift")
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("failed build left a success-shaped output: %v", err)
			}
		})
	}
}

func TestUnsafeInputsFailClosed(t *testing.T) {
	t.Run("traversal", func(t *testing.T) {
		config := testConfig([]string{"safe.txt"}, nil)
		config.SourceRoot = "../outside"
		if err := validateConfig(config); err == nil {
			t.Fatal("traversing source root was accepted")
		}
	})

	t.Run("duplicate config path", func(t *testing.T) {
		config := testConfig([]string{"same.txt", "same.txt"}, nil)
		if err := validateConfig(config); err == nil {
			t.Fatal("duplicate config path was accepted")
		}
	})

	t.Run("duplicate normalized source path", func(t *testing.T) {
		config := testConfig([]string{"same.txt"}, nil)
		inputs := []sourceInput{testInput(t, "same.txt", "a"), testInput(t, "same.txt", "b")}
		if _, _, err := buildLock(config, inputs); err == nil {
			t.Fatal("duplicate normalized source path was accepted")
		}
	})

	t.Run("escaping symlink", func(t *testing.T) {
		root := t.TempDir()
		outside := filepath.Join(t.TempDir(), "outside.txt")
		if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, "escape.txt")); err != nil {
			t.Fatal(err)
		}
		if _, err := scanSourceRoot(root); err == nil {
			t.Fatal("escaping symlink was accepted")
		}
	})

	t.Run("in-root symlink", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "target.txt"), []byte("target"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("target.txt", filepath.Join(root, "link.txt")); err != nil {
			t.Fatal(err)
		}
		if _, err := scanSourceRoot(root); err == nil {
			t.Fatal("in-root symlink was accepted")
		}
	})

	t.Run("invalid UTF-8", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "bad.txt"), []byte{0xff, 0xfe}, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := scanSourceRoot(root); err == nil {
			t.Fatal("invalid UTF-8 was accepted")
		}
	})

	t.Run("binary NUL", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "bad.txt"), []byte{'a', 0, 'b'}, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := scanSourceRoot(root); err == nil {
			t.Fatal("binary NUL was accepted")
		}
	})

	t.Run("unsupported type", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "image.png"), []byte("not really an image"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := scanSourceRoot(root); err == nil {
			t.Fatal("unsupported file type was accepted")
		}
	})

	t.Run("output inside source", func(t *testing.T) {
		root := copyFixture(t, filepath.Join(t.TempDir(), "fixture"))
		lockPath := filepath.Join(root, LockFileName)
		if _, err := Create(filepath.Join(root, "config.json"), lockPath); err != nil {
			t.Fatal(err)
		}
		output := filepath.Join(root, "sources", "output")
		if _, err := Build(lockPath, output); err == nil {
			t.Fatal("output inside source root was accepted")
		}
	})
}

func TestVerifyRejectsTampering(t *testing.T) {
	tests := map[string]func(t *testing.T, output string){
		"hash": func(t *testing.T, output string) {
			path := filepath.Join(output, BundleFileName)
			data := mustRead(t, path)
			data[len(data)-1] ^= 1
			mustWrite(t, path, data)
		},
		"length": func(t *testing.T, output string) {
			path := filepath.Join(output, ManifestFileName)
			var manifest Manifest
			if err := decodeCanonicalJSON(mustRead(t, path), &manifest); err != nil {
				t.Fatal(err)
			}
			manifest.Bundle.Bytes++
			data, _ := canonicalJSON(manifest)
			mustWrite(t, path, data)
		},
		"schema": func(t *testing.T, output string) {
			path := filepath.Join(output, LockFileName)
			var lockFile LockFile
			if err := decodeCanonicalJSON(mustRead(t, path), &lockFile); err != nil {
				t.Fatal(err)
			}
			lockFile.SchemaVersion = "distill-lock/v999"
			data, _ := canonicalJSON(lockFile)
			mustWrite(t, path, data)
		},
		"canonical JSON": func(t *testing.T, output string) {
			path := filepath.Join(output, ManifestFileName)
			data := append(mustRead(t, path), ' ')
			mustWrite(t, path, data)
		},
		"file set": func(t *testing.T, output string) {
			mustWrite(t, filepath.Join(output, "unexpected.txt"), []byte("unexpected"))
		},
		"manifest": func(t *testing.T, output string) {
			path := filepath.Join(output, ManifestFileName)
			var manifest Manifest
			if err := decodeCanonicalJSON(mustRead(t, path), &manifest); err != nil {
				t.Fatal(err)
			}
			manifest.Sources[0].Reason = "tampered"
			data, _ := canonicalJSON(manifest)
			mustWrite(t, path, data)
		},
		"checksums": func(t *testing.T, output string) {
			path := filepath.Join(output, ChecksumsFileName)
			data := mustRead(t, path)
			data[0] = '0'
			mustWrite(t, path, data)
		},
	}

	for name, tamper := range tests {
		t.Run(name, func(t *testing.T) {
			root := copyFixture(t, filepath.Join(t.TempDir(), "fixture"))
			output := filepath.Join(root, "output")
			buildFixture(t, root, "output")
			tamper(t, output)
			if _, err := Verify(output); err == nil {
				t.Fatal("verification accepted tampered output")
			}
		})
	}
}

func TestFailedBuildPreservesPriorVerifiedOutput(t *testing.T) {
	root := copyFixture(t, filepath.Join(t.TempDir(), "fixture"))
	output := filepath.Join(root, "output")
	buildFixture(t, root, "output")
	before := readOutputFiles(t, output)

	mustWrite(t, filepath.Join(root, "sources", "code", "example.go"), []byte("package changed\n"))
	if _, err := Build(filepath.Join(root, LockFileName), output); err == nil {
		t.Fatal("build unexpectedly succeeded")
	}
	if _, err := Verify(output); err != nil {
		t.Fatalf("prior output no longer verifies: %v", err)
	}
	after := readOutputFiles(t, output)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed build altered prior output")
	}
}

func TestLockPackageHasNoNetworkOrProviderImports(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data := string(mustRead(t, entry.Name()))
		for _, forbidden := range []string{`"net"`, `"net/http"`, `"net/url"`, `pkg/embedding`, `pkg/retriever`} {
			if strings.Contains(data, forbidden) {
				t.Fatalf("%s imports forbidden network/provider dependency %s", entry.Name(), forbidden)
			}
		}
	}
}

func TestGoldenFixture(t *testing.T) {
	root := copyFixture(t, filepath.Join(t.TempDir(), "fixture"))
	files := buildFixture(t, root, "output")
	actual := map[string]string{}
	for name, data := range files {
		actual[name] = digestBytes(data)
	}
	expected := map[string]string{
		BundleFileName:    "3096d7b4492124df4e893d27b15a9d59ffe8a9fad80d24cd8265262c2251866a",
		LockFileName:      "c27f70c858b10b02efd94727339f003311ef0e62feb6550a1f2481fed6eb00eb",
		ManifestFileName:  "e95b24ed00659d3eb13fb84ef8027fd425f1fa8ea16f991c3b1d60325f3d9eb7",
		ChecksumsFileName: "d1411a6046d01299dcf46fc3e46050bf5e64a685aea5f041965ba96aaa57a439",
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("golden hashes differ\nactual: %#v", actual)
	}
}

func TestDistillLockDemo(t *testing.T) {
	root := copyFixture(t, filepath.Join(t.TempDir(), "distill-lock-demo"))
	lockPath := filepath.Join(root, LockFileName)
	locked, err := Create(filepath.Join(root, "config.json"), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Build(lockPath, filepath.Join(root, "output-a"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(lockPath, filepath.Join(root, "output-b"))
	if err != nil {
		t.Fatal(err)
	}
	if first.BundleSHA256 != second.BundleSHA256 || first.ManifestSHA256 != second.ManifestSHA256 {
		t.Fatal("repeated build summaries differ")
	}
	if _, err := Verify(filepath.Join(root, "output-a")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(readOutputFiles(t, filepath.Join(root, "output-a")), readOutputFiles(t, filepath.Join(root, "output-b"))) {
		t.Fatal("repeated builds are not byte-identical")
	}

	_, originalLock, err := readLockFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	codePath := filepath.Join(root, "sources", "code", "example.go")
	code := mustRead(t, codePath)
	mustWrite(t, codePath, bytes.Replace(code, []byte("42"), []byte("43"), 1))
	mutatedLockPath := filepath.Join(root, "context.mutated.lock.json")
	if _, err := Create(filepath.Join(root, "config.json"), mutatedLockPath); err != nil {
		t.Fatal(err)
	}
	mutated, mutatedLock, err := readLockFile(mutatedLockPath)
	if err != nil {
		t.Fatal(err)
	}
	mutatedSummary, err := Build(mutatedLockPath, filepath.Join(root, "output-mutated"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(filepath.Join(root, "output-mutated")); err != nil {
		t.Fatal(err)
	}

	originalSource := findSource(t, originalLock, "code/example.go")
	changedSource := findSource(t, mutatedLock, "code/example.go")
	originalChunk := findChunk(t, originalLock, "code/example.go")
	changedChunk := findChunk(t, mutatedLock, "code/example.go")
	if originalSource.NormalizedSHA256 == changedSource.NormalizedSHA256 ||
		originalChunk.SHA256 == changedChunk.SHA256 ||
		first.BundleSHA256 == mutatedSummary.BundleSHA256 {
		t.Fatal("relevant mutation did not change source, chunk, and bundle identities")
	}

	t.Logf(
		"sources=%d duplicates=%d selected=%d tokens=%d bundle=%s lock=%s manifest=%s",
		first.SourceCount,
		first.DuplicateCount,
		first.SelectedCount,
		first.SelectedTokens,
		first.BundleSHA256,
		locked.LockSHA256,
		first.ManifestSHA256,
	)
	t.Logf(
		"repeat=byte-identical mutation_source=%s->%s mutation_chunk=%s->%s mutation_bundle=%s->%s mutated_lock_bytes=%d",
		originalSource.NormalizedSHA256,
		changedSource.NormalizedSHA256,
		originalChunk.SHA256,
		changedChunk.SHA256,
		first.BundleSHA256,
		mutatedSummary.BundleSHA256,
		len(mutated),
	)
}

func testConfig(sources, exclude []string) Config {
	sources = append([]string(nil), sources...)
	exclude = append([]string(nil), exclude...)
	sort.Strings(sources)
	sort.Strings(exclude)
	return Config{
		SchemaVersion:   ConfigSchemaVersion,
		SourceRoot:      "sources",
		Sources:         sources,
		Exclude:         exclude,
		ChunkBytes:      4096,
		TokenBudget:     4096,
		MetadataRemoval: "none",
	}
}

func testInput(t *testing.T, path, content string) sourceInput {
	t.Helper()
	normalized, err := normalizeText([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	return sourceInput{path: path, original: []byte(content), normalized: normalized}
}

func fixturePath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "testdata", "distill-lock-v0"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func copyFixture(t *testing.T, destination string) string {
	t.Helper()
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	source := fixturePath(t)
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return destination
}

func buildFixture(t *testing.T, root, outputName string) map[string][]byte {
	t.Helper()
	lockPath := filepath.Join(root, LockFileName)
	if _, err := Create(filepath.Join(root, "config.json"), lockPath); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, outputName)
	if _, err := Build(lockPath, output); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(output); err != nil {
		t.Fatal(err)
	}
	return readOutputFiles(t, output)
}

func readOutputFiles(t *testing.T, output string) map[string][]byte {
	t.Helper()
	result := make(map[string][]byte, len(outputFileNames))
	for _, name := range outputFileNames {
		result[name] = mustRead(t, filepath.Join(output, name))
	}
	return result
}

func findSource(t *testing.T, lockFile LockFile, path string) Source {
	t.Helper()
	for _, source := range lockFile.Sources {
		if source.Path == path {
			return source
		}
	}
	t.Fatalf("source %q not found", path)
	return Source{}
}

func findChunk(t *testing.T, lockFile LockFile, path string) Chunk {
	t.Helper()
	for _, chunk := range lockFile.Chunks {
		if chunk.Path == path {
			return chunk
		}
	}
	t.Fatalf("chunk for %q not found", path)
	return Chunk{}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
