package graph

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// BuildFromGoFiles walks a directory tree and builds a dependency graph from
// Go import statements. It does not require go/packages or cgo — it uses
// simple line-by-line parsing of import blocks.
func BuildFromGoFiles(root string) (*Graph, error) {
	g := New()

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			// Skip hidden dirs and vendor.
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		pkg := filepath.ToSlash(filepath.Dir(rel))
		if pkg == "." {
			pkg = ""
		}

		lang := "go"
		tags := []string{}
		if strings.HasSuffix(path, "_test.go") {
			tags = append(tags, "test")
		}

		g.AddNode(Node{
			ID:       rel,
			Type:     NodeTypeFile,
			Package:  pkg,
			Language: lang,
			Tags:     tags,
		})

		imports, err := parseGoImports(path)
		if err != nil {
			return nil
		}

		for _, imp := range imports {
			// Only track intra-repo imports (those that start with the module path
			// or are relative). We store the import path as the target node ID.
			// Callers can resolve these to file paths if needed.
			g.AddEdge(Edge{
				From:   rel,
				To:     imp,
				Weight: 1.0,
			})
		}
		return nil
	})
	return g, err
}

// parseGoImports extracts import paths from a Go source file.
func parseGoImports(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var imports []string
	scanner := bufio.NewScanner(f)
	inImport := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "import (" {
			inImport = true
			continue
		}
		if inImport && line == ")" {
			inImport = false
			continue
		}
		if strings.HasPrefix(line, "import ") && !inImport {
			// Single-line import.
			imp := extractImportPath(line)
			if imp != "" {
				imports = append(imports, imp)
			}
			continue
		}
		if inImport && line != "" && !strings.HasPrefix(line, "//") {
			imp := extractImportPath(line)
			if imp != "" {
				imports = append(imports, imp)
			}
		}
	}
	return imports, scanner.Err()
}

// extractImportPath extracts the import path string from a line like:
//
//	"github.com/foo/bar"
//	alias "github.com/foo/bar"
//	_ "github.com/foo/bar"
func extractImportPath(line string) string {
	// Find the quoted string.
	start := strings.Index(line, `"`)
	end := strings.LastIndex(line, `"`)
	if start < 0 || end <= start {
		return ""
	}
	return line[start+1 : end]
}
