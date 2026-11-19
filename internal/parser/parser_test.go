package parser

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/pinionlang/pinion/internal/ast"
	"github.com/pinionlang/pinion/internal/conformance"
)

var parserCode = regexp.MustCompile(`^(E0\d{3}|W0201)$`)

func TestConformance(t *testing.T) {
	cases, err := conformance.LoadDir("../../spec/conformance")
	if err != nil {
		t.Fatal(err)
	}
	nfiles := 0
	for _, c := range cases {
		var want []string
		for _, m := range c.Markers {
			if parserCode.MatchString(m.Code) {
				want = append(want, fmt.Sprintf("%s:%d:%s", m.File, m.Line, m.Code))
			}
		}
		var got []string
		for _, f := range c.Files {
			if !conformance.IsSource(f.Path) {
				continue
			}
			nfiles++
			_, d := ParseFile(f.Path, []byte(f.Content))
			for _, it := range d.Items {
				got = append(got, fmt.Sprintf("%s:%d:%s", f.Path, it.Pos.Line, it.Code))
			}
			if len(want) == 0 && d.HasErrors() {
				t.Errorf("%s: unexpected errors:\n%s", c.ID(), d.Human())
			}
		}
		sort.Strings(want)
		sort.Strings(got)
		if strings.Join(want, ",") != strings.Join(got, ",") {
			t.Errorf("%s: want %v, got %v", c.ID(), want, got)
		}
	}
	t.Logf("%d cases, %d source files", len(cases), nfiles)
}

func TestModulePath(t *testing.T) {
	for in, want := range map[string]string{"src/todo/store.pi": "todo/store", "main.pi": "", "/x/proj/src/main.pi": "main"} {
		if got := ModulePath(in); got != want {
			t.Errorf("%s: got %q want %q", in, got, want)
		}
	}
}

func TestDocAttach(t *testing.T) {
	src := "/// a\n/// b\npub fn f() -> Int { 1 }\n\nenum E {\n  /// v\n  A\n}\n"
	f, d := ParseFile("x.pi", []byte(src))
	if len(d.Items) != 0 {
		t.Fatal(d.Human())
	}
	if f.Decls[0].(*ast.FnDecl).Doc != "a\nb" {
		t.Errorf("doc %q", f.Decls[0].(*ast.FnDecl).Doc)
	}
	if f.Decls[1].(*ast.EnumDecl).Variants[0].Doc != "v" {
		t.Error("variant doc")
	}
	if len(f.Comments) != 3 {
		t.Error("comments")
	}
}

func TestShapes(t *testing.T) {
	src := `fn main() !io {
  let x = json.decode<Map<Str, Int?>>(s).ok() else return
  let y = (a ?? 0) + 1
  xs.each(x => sum = sum + x)
  match (s, e) {
    (Idle, Start) => State.Running
    Err(A(_) | B(_, _)) => 1
  }
  print("{name ?? "-"} \{x\}")
  for (k, _) in m { }
}
`
	f, d := ParseFile("x.pi", []byte(src))
	if len(d.Items) != 0 {
		t.Fatal(d.Human())
	}
	body := f.Decls[0].(*ast.FnDecl).Body
	if len(body.Stmts) != 6 {
		t.Fatalf("stmts %d", len(body.Stmts))
	}
	ls := body.Stmts[0].(*ast.LetStmt)
	if ls.Else == nil {
		t.Error("let else")
	}
	sl := body.Stmts[4].(*ast.ExprStmt).X.(*ast.CallExpr).Args[0].Value.(*ast.StrLit)
	if len(sl.Parts) != 2 || sl.Parts[1].Text != " {x}" {
		t.Errorf("parts %#v", sl.Parts)
	}
}
