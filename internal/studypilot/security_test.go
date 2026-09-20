package studypilot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeContainsNoCredentialOrNetworkAccess(t *testing.T) {
	roots := []string{".", filepath.Join("..", "..", "cmd", "distill-jev-pilot")}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(root, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			prohibited := []string{
				"os.Get" + "env", "os.Lookup" + "Env", `"net/http"`, `"net"`,
				"http.New" + "Request", "http.Get", "net.Dial",
				"TYPE" + "SAFE_API_KEY",
			}
			for _, needle := range prohibited {
				if strings.Contains(source, needle) {
					t.Fatalf("%s contains prohibited runtime access %q", filepath.Join(root, entry.Name()), needle)
				}
			}
		}
	}
}

func TestJSONDepthLimitPrecedesDecode(t *testing.T) {
	value := strings.Repeat("[", maxJSONDepth+1) + "0" + strings.Repeat("]", maxJSONDepth+1)
	if _, err := decodeCanonicalValue([]byte(value)); err == nil ||
		!strings.Contains(err.Error(), "nesting exceeds") {
		t.Fatalf("deep JSON was not rejected by structural limit: %v", err)
	}
}
