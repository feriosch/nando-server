package internal_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestTestsDeclareTheirOwnJSONShapes checks the rule in the README's "JSON
// shapes are the API": tests read and write the API with their own copies of
// its JSON shapes, never the model ones, so a changed json tag fails a test.
// It fails on any use of a model type named …JSON in a tests/ folder.
func TestTestsDeclareTheirOwnJSONShapes(t *testing.T) {
	files := 0
	err := filepath.WalkDir(".", func(file string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(file, "_test.go") ||
			path.Base(path.Dir(filepath.ToSlash(file))) != "tests" {
			return err
		}
		files++
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			return err
		}
		models := map[string]bool{} // the names this file imports a model package under
		for _, spec := range f.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			if !strings.HasPrefix(imported, internalPath) || path.Base(imported) != "model" {
				continue
			}
			name := "model"
			if spec.Name != nil {
				name = spec.Name.Name
			}
			models[name] = true
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && models[pkg.Name] && strings.HasSuffix(sel.Sel.Name, "JSON") {
				t.Errorf("%s: uses %s.%s; declare the test's own copy of the JSON shape instead",
					fset.Position(sel.Pos()), pkg.Name, sel.Sel.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("found no tests/ folders under internal/; the test is looking in the wrong place")
	}
}
