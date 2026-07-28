package stockdata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTransportLayersDoNotReachIntoRawTDXClient(t *testing.T) {
	for _, directory := range []string{
		filepath.Join("..", "..", "..", "cmd", "cli"),
		filepath.Join("..", "..", "..", "pkg", "server"),
	} {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
				strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(directory, entry.Name())
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(source)
			for _, forbidden := range []string{"*tdx.Client", ".Client.", "ExecDo("} {
				if strings.Contains(text, forbidden) {
					t.Errorf("%s reaches through the typed service boundary using %q", path, forbidden)
				}
			}
		}
	}
}
