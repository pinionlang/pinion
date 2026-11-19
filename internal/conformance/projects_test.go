package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinionlang/pinion/internal/ast"
	"github.com/pinionlang/pinion/internal/check"
	"github.com/pinionlang/pinion/internal/parser"
)

// TestProjectsCheckClean parses and checks every example and benchmark project:
// they are valid programs and must produce no diagnostics.
func TestProjectsCheckClean(t *testing.T) {
	var roots []string
	for _, pat := range []string{"../../examples/*/src", "../../bench/tasks/*/pinion/src"} {
		m, _ := filepath.Glob(pat)
		roots = append(roots, m...)
	}
	if len(roots) == 0 {
		t.Skip("no projects")
	}
	for _, src := range roots {
		root := filepath.Dir(src)
		t.Run(strings.TrimPrefix(root, "../../"), func(t *testing.T) {
			var files []*ast.File
			filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() || !strings.HasSuffix(p, ".pi") {
					return err
				}
				rel, _ := filepath.Rel(root, p)
				b, _ := os.ReadFile(p)
				f, dl := parser.ParseFile(rel, b)
				for _, d := range dl.Items {
					t.Errorf("parse: %s", d)
				}
				files = append(files, f)
				return nil
			})
			dl := check.Check(files)
			for _, d := range dl.Items {
				t.Errorf("check: %s", d)
			}
		})
	}
}
