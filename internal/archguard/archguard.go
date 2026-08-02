// Package archguard implements static architecture quality gates. See
// ADR 0002 and tongstock-ai.1 (architecture first) for the constraints.
package archguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// GateResult is the outcome of a single architecture check.
type GateResult struct {
	Name    string
	Passed  bool
	Issues  []string
	Allowed []string
}

// Report aggregates all gate results. Passed is false if any gate failed.
type Report struct {
	Passed bool
	Root   string
	Gates  []GateResult
}

// Option configures Run.
type Option func(*runner)

// WithGoRoot overrides the repo root (defaults to ".")
func WithGoRoot(root string) Option { return func(r *runner) { r.root = root } }

type runner struct {
	root        string
	imports     map[string][]string // package dir -> direct imports
	fset        *token.FileSet
	nonTestOnly map[string]bool // dir has production Go files
}

// Run executes every gate and returns a deterministic report.
func Run(opts ...Option) *Report {
	r := &runner{
		root:        ".",
		imports:     map[string][]string{},
		fset:        token.NewFileSet(),
		nonTestOnly: map[string]bool{},
	}
	for _, o := range opts {
		o(r)
	}
	_ = r.scan()
	rep := &Report{Root: r.root, Passed: true}
	rep.Gates = append(rep.Gates, r.dependencyDirection())
	rep.Gates = append(rep.Gates, r.cycleDetection())
	rep.Gates = append(rep.Gates, r.noMockNoRandomNoHardcoded())
	rep.Gates = append(rep.Gates, r.noDeadProductionCallers())
	rep.Gates = append(rep.Gates, r.noDuplicateDomainType())
	for _, g := range rep.Gates {
		if !g.Passed {
			rep.Passed = false
		}
	}
	return rep
}

func (r *runner) scan() error {
	return filepath.Walk(r.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || base == "node_modules" || base == "web" || (strings.HasPrefix(base, ".") && base != ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		return r.parseFile(path)
	})
}

func (r *runner) parseFile(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	f, err := parser.ParseFile(r.fset, path, src, parser.ImportsOnly)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	r.nonTestOnly[dir] = true
	pkgPath := filepath.ToSlash(dir)
	for _, imp := range f.Imports {
		raw := strings.Trim(imp.Path.Value, "\"")
		r.imports[pkgPath] = append(r.imports[pkgPath], raw)
	}
	return nil
}

var forbiddenDomainImports = map[string][]string{
	"internal/paradigms": {
		"github.com/gin-gonic/gin",
		"github.com/sjzsdu/tongstock/pkg/storage",
		"github.com/sjzsdu/tongstock/pkg/tdx",
	},
	"internal/monitoring": {
		"github.com/gin-gonic/gin",
		"github.com/sjzsdu/tongstock/pkg/storage",
		"github.com/sjzsdu/tongstock/pkg/tdx",
	},
	"internal/ai_critic": {
		"github.com/gin-gonic/gin",
		"github.com/sjzsdu/tongstock/pkg/storage",
		"github.com/sjzsdu/tongstock/pkg/tdx",
	},
	"internal/backtest": {
		"github.com/gin-gonic/gin",
		"github.com/sjzsdu/tongstock/pkg/storage",
	},
}

// dependencyDirection: domain packages must NOT import gin/storage/tdx directly.
func (r *runner) dependencyDirection() GateResult {
	res := GateResult{
		Name:   "dependency-direction",
		Passed: true,
		Allowed: []string{
			"internal/adapter/*: adapters may depend on storage/tdx",
			"pkg/server: transport may depend on gin",
		},
	}
	for pkg, forbidden := range forbiddenDomainImports {
		absPkg := filepath.Clean(filepath.Join(r.root, pkg))
		imports, ok := r.imports[filepath.ToSlash(absPkg)]
		if !ok {
			continue
		}
		for _, imp := range imports {
			for _, bad := range forbidden {
				if imp == bad {
					res.Passed = false
					res.Issues = append(res.Issues,
						fmt.Sprintf("%s: imports %s (ADR 0002: domain must not depend on transport/database)", pkg, bad))
				}
			}
		}
	}
	return res
}

// cycleDetection: direct import cycles among project packages (DFS on adj).
func (r *runner) cycleDetection() GateResult {
	res := GateResult{Name: "cycle-detection", Passed: true}
	const prefix = "github.com/sjzsdu/tongstock/"
	adj := map[string][]string{}
	for dir, imps := range r.imports {
		rel := strings.TrimPrefix(strings.ReplaceAll(filepath.Clean(dir), filepath.Clean(r.root)+string(filepath.Separator), ""), string(filepath.Separator))
		key := prefix + rel
		for _, i := range imps {
			if strings.HasPrefix(i, prefix) {
				adj[key] = append(adj[key], i)
			}
		}
	}
	visited := map[string]bool{}
	onStack := map[string]bool{}
	var dfs func(n string, stack []string)
	dfs = func(n string, stack []string) {
		if onStack[n] {
			idx := -1
			for i, s := range stack {
				if s == n {
					idx = i
					break
				}
			}
			if idx < 0 {
				idx = 0
			}
			cycle := append([]string{}, stack[idx:]...)
			cycle = append(cycle, n)
			res.Passed = false
			res.Issues = append(res.Issues, "cycle: "+strings.Join(cycle, " → "))
			return
		}
		if visited[n] {
			return
		}
		visited[n] = true
		onStack[n] = true
		stack = append(stack, n)
		for _, nxt := range adj[n] {
			dfs(nxt, stack)
		}
		stack = stack[:len(stack)-1]
		onStack[n] = false
	}
	for n := range adj {
		if !visited[n] {
			dfs(n, nil)
		}
	}
	return res
}

// noMockNoRandomNoHardcoded enforces the architecture-first rules:
//
//  1. Production files may not import math/rand (real sources of truth only).
//  2. Production files may not import mock frameworks.
//  3. Text markers for synthetic/forged backtest results are forbidden in production.
//
// Imports are derived from the AST-backed runner.imports map to avoid false
// positives on comment or string content mentioning forbidden packages.
func (r *runner) noMockNoRandomNoHardcoded() GateResult {
	res := GateResult{
		Name:   "no-mock-random-hardcoded",
		Passed: true,
		Allowed: []string{
			"_test.go files: tests may use any mocking",
			"crypto/rand is allowed for true hashing/nonces",
			"golden baselines with hardcoded results are allowed in *_test.go",
		},
	}
	const prefix = "github.com/sjzsdu/tongstock/"
	for dir, imports := range r.imports {
		rel := strings.TrimPrefix(strings.ReplaceAll(filepath.Clean(dir), filepath.Clean(r.root)+string(filepath.Separator), ""), string(filepath.Separator))
		pkg := prefix + rel
		_ = pkg
		for _, imp := range imports {
			switch imp {
			case "math/rand":
				res.Passed = false
				res.Issues = append(res.Issues, rel+": imports math/rand (production stock data must be real)")
			case "github.com/stretchr/testify/mock", "go.uber.org/mock/gomock":
				res.Passed = false
				res.Issues = append(res.Issues, rel+": imports mock framework in production code")
			}
		}
	}
	// Second pass: text markers that cannot be caught from import specs alone
	// (synthetic-backtest markers are literal string signatures). Skip files
	// that contain these in comments only; but since the string literals above
	// (in archguard itself) would trigger, skip archguard package files.
	_ = filepath.Walk(r.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || base == "node_modules" || base == "web" || (strings.HasPrefix(base, ".") && base != ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(r.root, path)
		if strings.HasPrefix(rel, "internal/archguard/") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		low := strings.ToLower(string(src))
		if strings.Contains(low, "synthetic backtest") ||
			strings.Contains(low, "synthetic_result") {
			res.Passed = false
			res.Issues = append(res.Issues, rel+": contains synthetic backtest/result marker")
		}
		return nil
	})
	// The browser is also a production path. Recommendations must not be made
	// from random values or literal fake-result markers in React/TypeScript.
	webRoot := filepath.Join(r.root, "web", "src")
	_ = filepath.Walk(webRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.Contains(path, ".test.") || strings.Contains(path, ".spec.") {
			return nil
		}
		if !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		low := strings.ToLower(string(src))
		rel, _ := filepath.Rel(r.root, path)
		if strings.Contains(string(src), "Math.random(") {
			res.Passed = false
			res.Issues = append(res.Issues, rel+": uses Math.random in production UI")
		}
		if strings.Contains(low, "synthetic_result") || strings.Contains(low, "mock returns") {
			res.Passed = false
			res.Issues = append(res.Issues, rel+": contains fabricated-result marker")
		}
		return nil
	})
	return res
}

// noDeadProductionCallers reports production packages that are not imported
// by any other production code or cmd entrypoint. White-listed packages with
// explicit reasons are allowed.
func (r *runner) noDeadProductionCallers() GateResult {
	res := GateResult{Name: "no-dead-production-callers", Passed: true}
	whitelist := map[string]string{
		"internal/ai_critic":            "wired via HTTP handlers + experiment tooling",
		"internal/archguard":            "command-only entry, wired in cmd/cli",
		"internal/picoclaw":             "optional runtime wiring",
		"internal/adapter/paradigmrepo": "injected at composition root in serverapp",
		"pkg/tdx/adjust":                "used by cmd/* through tdx.Service, indirectly via adjust.{ApplyForward|Split}",
	}
	const prefix = "github.com/sjzsdu/tongstock/"
	importedBy := map[string]bool{}
	for _, imps := range r.imports {
		for _, i := range imps {
			if strings.HasPrefix(i, prefix) {
				importedBy[i] = true
			}
		}
	}
	for dir := range r.nonTestOnly {
		rel := strings.TrimPrefix(strings.ReplaceAll(filepath.Clean(dir), filepath.Clean(r.root)+string(filepath.Separator), ""), string(filepath.Separator))
		me := prefix + rel
		if strings.HasPrefix(me, prefix+"cmd/") {
			continue
		}
		if importedBy[me] {
			continue
		}
		reason, ok := whitelist[rel]
		if ok {
			res.Allowed = append(res.Allowed, rel+": "+reason)
			continue
		}
		res.Passed = false
		res.Issues = append(res.Issues, "package not imported by production code: "+me)
	}
	return res
}

// noDuplicateDomainType: flag if the same semantic domain type is co-owned in
// two distinct domain-layer package directories (e.g. two Paradigm structs).
func (r *runner) noDuplicateDomainType() GateResult {
	res := GateResult{Name: "no-duplicate-domain-type", Passed: true}
	watched := map[string]bool{
		"Paradigm": true, "Experiment": true, "Hypothesis": true,
		"SignalEntry": true, "DatasetSnapshot": true, "EvidenceCard": true,
		"MonitorReport": true, "ReviewOutcome": true,
	}
	seen := map[string]map[string]bool{} // TypeName -> set of package dirs
	err := filepath.Walk(r.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || base == "node_modules" || base == "web" || (strings.HasPrefix(base, ".") && base != ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(r.fset, path, src, 0)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(r.root, path)
		pkgDir := filepath.ToSlash(filepath.Dir(rel))
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				name := ts.Name.Name
				if !watched[name] {
					continue
				}
				if _, ok := seen[name]; !ok {
					seen[name] = map[string]bool{}
				}
				seen[name][pkgDir] = true
			}
		}
		return nil
	})
	_ = err
	for name, pkgs := range seen {
		// Allow owning pkg + transport/DTO mirroring, but flag two true domain owners.
		domainOwners := map[string]bool{}
		for p := range pkgs {
			if strings.HasPrefix(p, "internal/paradigm") {
				domainOwners[p] = true
			}
		}
		if len(domainOwners) > 1 {
			res.Passed = false
			res.Issues = append(res.Issues, fmt.Sprintf("type %s has multiple domain-layer owners: %v", name, sortedKeys(domainOwners)))
		}
	}
	return res
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// Text returns a human-readable report.
func (r *Report) Text() string {
	var sb strings.Builder
	status := "PASS"
	if !r.Passed {
		status = "FAIL"
	}
	fmt.Fprintf(&sb, "archguard: %s (root=%s)\n", status, r.Root)
	for _, g := range r.Gates {
		gst := "PASS"
		if !g.Passed {
			gst = "FAIL"
		}
		fmt.Fprintf(&sb, "  [%s] %s\n", gst, g.Name)
		for _, issue := range g.Issues {
			fmt.Fprintf(&sb, "      ! %s\n", issue)
		}
	}
	return sb.String()
}
