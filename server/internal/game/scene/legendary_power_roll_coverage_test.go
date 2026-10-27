package scene

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Static half of the coverage contract. It catches misspelled keys and wrong
// units at literal callsites, including calls nested in registration callbacks
// and power-ID switch arms. Shared factories with runtime parameters also need
// their concrete behavioral tests; this deliberately does not pretend that a
// source scan proves every runtime branch is reachable.
func TestLegendaryPowerRollAuthoredCallsites(t *testing.T) {
	type coefficient struct {
		key  string
		base int32
	}
	byID := map[string]map[coefficient]bool{}
	all := map[coefficient]bool{}
	for _, spec := range domain.LegendaryPowerRollSpecs() {
		byID[spec.ID] = map[coefficient]bool{}
		for _, p := range spec.Parameters {
			v := coefficient{p.Key, p.Base}
			byID[spec.ID][v] = true
			all[v] = true
		}
	}
	seen := map[coefficient]bool{}
	dynamic := map[string]bool{}
	files, err := filepath.Glob("legendary_*.go")
	files = append(files, "trap.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	// Typed baseline tuning fields are simple constants, not arbitrary
	// expression evaluation of catalog prose.
	tuning := map[string]int32{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		root, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(root, func(n ast.Node) bool {
			v, ok := n.(*ast.ValueSpec)
			if !ok || len(v.Names) != 1 || v.Names[0].Name != "universalTuning" || len(v.Values) != 1 {
				return true
			}
			lit, ok := v.Values[0].(*ast.CompositeLit)
			if !ok {
				return true
			}
			for _, e := range lit.Elts {
				kv, ok := e.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				value, ok := kv.Value.(*ast.BasicLit)
				if !ok {
					continue
				}
				n, err := strconv.ParseInt(value.Value, 0, 32)
				if err == nil {
					tuning[key.Name] = int32(n)
				}
			}
			return true
		})
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		root, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		var stack []ast.Node
		ast.Inspect(root, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			stack = append(stack, n)
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "RollValue" {
				return true
			}
			keyLit, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				t.Errorf("%s: nonliteral roll key", fset.Position(n.Pos()))
				return true
			}
			key, err := strconv.Unquote(keyLit.Value)
			if err != nil {
				t.Fatal(err)
			}
			var base int32
			known := false
			switch v := call.Args[1].(type) {
			case *ast.BasicLit:
				parsed, err := strconv.ParseInt(v.Value, 0, 32)
				if err == nil {
					base = int32(parsed)
					known = true
				}
			case *ast.SelectorExpr:
				if x, ok := v.X.(*ast.Ident); ok && x.Name == "universalTuning" {
					base, known = tuning[v.Sel.Name]
				}
			}
			if !known {
				dynamic[key] = true
				return true
			}
			value := coefficient{key, base}
			seen[value] = true
			if !all[value] {
				t.Errorf("%s: runtime coefficient %s=%d has no authored range", fset.Position(n.Pos()), key, base)
			}
			// Other active powers deliberately read their own roll, not the
			// surrounding switch arm (e.g. poison crit + critical bonus).
			owner, own := sel.X.(*ast.Ident)
			if !own || owner.Name != "c" {
				return true
			}
			var ids []string
			for i := len(stack) - 2; i >= 0 && len(ids) == 0; i-- {
				switch p := stack[i].(type) {
				case *ast.CaseClause:
					for _, expr := range p.List {
						if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
							id, _ := strconv.Unquote(lit.Value)
							if _, ok := domain.LegendaryPower(id); ok {
								ids = append(ids, id)
							}
						}
					}
				case *ast.CallExpr:
					fn, ok := p.Fun.(*ast.Ident)
					if !ok || !strings.HasPrefix(fn.Name, "register") || len(p.Args) < 1 {
						continue
					}
					if lit, ok := p.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						id, _ := strconv.Unquote(lit.Value)
						if _, ok := domain.LegendaryPower(id); ok {
							ids = append(ids, id)
						}
					}
				}
			}
			for _, id := range ids {
				if !byID[id][value] {
					t.Errorf("%s: %s runtime %s=%d disagrees with authored metadata", fset.Position(n.Pos()), id, key, base)
				}
			}
			return true
		})
	}
	for id, coefficients := range byID {
		for p := range coefficients {
			if !seen[p] && !dynamic[p.key] {
				t.Errorf("%s authored %s=%d has no runtime callsite", id, p.key, p.base)
			}
		}
	}
}
