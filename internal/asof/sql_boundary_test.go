package asof_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestEveryKlineLatestQueryIsDateBounded is the static half of this layer's
// contract. Reading kline_latest without a `date <=` bound is how lookahead
// leaks back in, and a runtime test only covers the queries it happens to
// execute. This gate scans every string literal in the production files of
// this package and fails on any `FROM kline_latest` without the bound.
func TestEveryKlineLatestQueryIsDateBounded(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	checked := 0
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			lit, ok := node.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("%s: unquote %s: %v", name, lit.Value, err)
			}
			normalized := strings.Join(strings.Fields(strings.ToLower(value)), " ")
			// Only FROM clauses are queries. Error messages and comments may
			// mention the view by name and must not trip the gate.
			if !strings.Contains(normalized, "from kline_latest") {
				return true
			}
			checked++
			if !strings.Contains(normalized, "date <=") {
				t.Errorf("%s: kline_latest query without a `date <=` bound: %s",
					fset.Position(lit.Pos()), value)
			}
			return true
		})
	}
	if checked == 0 {
		t.Fatal("found no kline_latest queries: the static check would pass vacuously")
	}
}
