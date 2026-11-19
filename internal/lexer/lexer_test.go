package lexer

import (
	"testing"

	"github.com/pinionlang/pinion/internal/diag"
	"github.com/pinionlang/pinion/internal/token"
)

func kinds(src string) ([]token.Kind, *diag.List) {
	d := &diag.List{}
	r := Lex("x.pi", []byte(src), d)
	var ks []token.Kind
	for _, t := range r.Tokens {
		ks = append(ks, t.Kind)
	}
	return ks, d
}

func TestNewlines(t *testing.T) {
	ks, _ := kinds("a\n  .b(\n1,\n2)\nc +\nd")
	want := []token.Kind{token.Ident, token.Dot, token.Ident, token.LParen, token.Int, token.Comma, token.Int, token.RParen,
		token.Newline, token.Ident, token.Plus, token.Ident, token.EOF}
	if len(ks) != len(want) {
		t.Fatalf("got %v", ks)
	}
	for i := range ks {
		if ks[i] != want[i] {
			t.Fatalf("got %v", ks)
		}
	}
}

func TestLiterals(t *testing.T) {
	d := &diag.List{}
	r := Lex("x.pi", []byte(`12.30d 0..3 1.5 "a{x ?? "-"}\{\}" Int??`), d)
	k := []token.Kind{token.Dec, token.Int, token.DotDot, token.Int, token.Float, token.String, token.Ident, token.QQ, token.EOF}
	for i, want := range k {
		if r.Tokens[i].Kind != want {
			t.Fatalf("tok %d: %v", i, r.Tokens[i])
		}
	}
	parts := r.Tokens[5].Parts
	if len(parts) != 3 || parts[1].Expr != `x ?? "-"` || parts[2].Text != "{}" {
		t.Errorf("parts %#v", parts)
	}
	if len(d.Items) != 0 {
		t.Error(d.Human())
	}
}

func TestComments(t *testing.T) {
	d := &diag.List{}
	r := Lex("x.pi", []byte("/// doc\nfn f() // c\n/* no */"), d)
	if len(r.Comments) != 2 || !r.Comments[0].Doc || !r.Comments[1].Trailing {
		t.Errorf("%#v", r.Comments)
	}
	if len(d.Items) != 1 || d.Items[0].Code != "E0001" || d.Items[0].Pos.Line != 3 {
		t.Error(d.Human())
	}
}
