// Package internal_test checks the import rules in the README's Layout: a
// Module may import what Core exports, but Core never imports a Module, a
// Module never imports another Module, and no Module imports Core's
// repositories or services.
package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const internalPath = "github.com/feriosch/nando-server/internal/"

// notModules are the folders under internal/ that aren't Modules. Every other
// folder is one, so a new Module is covered without changing this test.
var notModules = map[string]bool{"core": true, "server": true}

func isModule(area string) bool {
	return area != "" && !notModules[area] && !strings.HasPrefix(area, "test")
}

// area is the folder under internal/ that an import path or file path is in,
// such as "core" or "calories".
func area(path string) string {
	first, _, _ := strings.Cut(path, "/")
	return first
}

func TestCoreAndModulesImportOnlyWhatTheyMay(t *testing.T) {
	files := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		files++
		from := area(filepath.ToSlash(path))
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range f.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			rest, ok := strings.CutPrefix(imported, internalPath)
			if !ok {
				continue
			}
			to := area(rest)
			switch {
			case from == "core" && isModule(to):
				t.Errorf("%s imports %s: Core never imports a Module", path, imported)
			case isModule(from) && isModule(to) && to != from:
				t.Errorf("%s imports %s: a Module never imports another Module", path, imported)
			case isModule(from) && (strings.HasPrefix(rest, "core/repository") || strings.HasPrefix(rest, "core/service")):
				t.Errorf("%s imports %s: a Module reaches Core's data only through model.CoreService", path, imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("found no Go files under internal/; the test is looking in the wrong place")
	}
}
