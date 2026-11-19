package check

import (
	"os"
	"testing"

	"github.com/pinionlang/pinion/internal/ast"
	"github.com/pinionlang/pinion/internal/parser"
)

func TestAdhoc(t *testing.T) {
	p := os.Getenv("PINION_ADHOC")
	if p == "" {
		t.Skip()
	}
	src, _ := os.ReadFile(p)
	f, d := parser.ParseFile("main.pi", src)
	for _, x := range d.Items {
		t.Log(x.String())
	}
	for _, x := range Check([]*ast.File{f}).Items {
		t.Log(x.String())
	}
}
