package archguard_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sjzsdu/tongstock/internal/archguard"
)

// TestArchguardHasExpectedGates runs the guard against the current repo and
// asserts the fixed set of gates are present. This catches silent regression
// if a gate is accidentally removed from the Run() pipeline.
func TestArchguardHasExpectedGates(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	rep := archguard.Run(archguard.WithGoRoot(root))
	want := map[string]bool{
		"dependency-direction":       false,
		"cycle-detection":            false,
		"no-mock-random-hardcoded":   false,
		"no-dead-production-callers": false,
		"no-duplicate-domain-type":   false,
	}
	for _, g := range rep.Gates {
		if _, ok := want[g.Name]; ok {
			want[g.Name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Fatalf("expected gate %q missing from report", name)
		}
	}
}

// TestDependencyDirectionCatchesForbiddenImport ensures the gate would flag
// a domain package that directly imports gin. It constructs a synthetic tree,
// so it doesn't rely on the repo currently having violations.
func TestDependencyDirectionCatchesForbiddenImport(t *testing.T) {
	tmp := t.TempDir()
	pkg := filepath.Join(tmp, "internal", "paradigms")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	// Synthetic bad file: paradigms importing gin.
	badGo := `package paradigms
import _ "github.com/gin-gonic/gin"
func F() {}
`
	if err := os.WriteFile(filepath.Join(pkg, "bad.go"), []byte(badGo), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := archguard.Run(archguard.WithGoRoot(tmp))
	for _, g := range rep.Gates {
		if g.Name != "dependency-direction" {
			continue
		}
		if g.Passed {
			t.Fatalf("expected dependency-direction to fail for paradigms→gin import, got PASS issues=%v", g.Issues)
		}
	}
	if rep.Passed {
		t.Fatalf("expected synthetic tree to fail, got overall PASS")
	}
}

// TestCleanTreePasses runs the guard against the real repo root and asserts
// that the current production baseline passes every gate. Failing this means
// a regression has been introduced and must be fixed before releasing.
func TestCleanTreePasses(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	rep := archguard.Run(archguard.WithGoRoot(root))
	if !rep.Passed {
		t.Fatalf("repo does not pass architecture gates: %s", rep.Text())
	}
}

func repoRoot() (string, error) {
	// archguard package lives at internal/archguard -> up two levels.
	d, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(d, "..", ".."))
}
