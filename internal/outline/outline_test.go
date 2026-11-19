package outline

import (
	"testing"

	"github.com/pinionlang/pinion/internal/ast"
	"github.com/pinionlang/pinion/internal/conformance"
	"github.com/pinionlang/pinion/internal/parser"
)

func TestOutline(t *testing.T) {
	src := `import std/fs

/// An order.
pub data Order : Priced {
  /// in cents
  pub subtotal: Int
  init(subtotal: Int) -> Result<Self, E> {
    Ok(Self { subtotal: subtotal })
  }
  pub fn total() -> Int { 0 }
}

class Counter {
  mut n: Int = 0
  pub mut fn hit() { self.n += 1 }
}

enum PayState {
  /// not yet
  Pending
  Paid(approval: Str)
}

interface Priced {
  fn price(m: Map<Str, Int?>) -> fn(Int) -> Int !io
}

fn main() !io !fs { }
`
	f, d := parser.ParseFile("src/shop/order.pi", []byte(src))
	if d.HasErrors() {
		t.Fatal(d.Human())
	}
	got := Outline([]*ast.File{f})
	want := `module shop/order
  import std/fs
  /// An order.
  pub data Order : Priced
    /// in cents
    pub subtotal: Int
    init(subtotal: Int) -> Result<Self, E>
    pub fn total() -> Int
  class Counter
    mut n: Int = …
    pub mut fn hit()
  enum PayState
    /// not yet
    Pending
    Paid(approval: Str)
  interface Priced
    fn price(m: Map<Str, Int?>) -> fn(Int) -> Int !io
  fn main() !io !fs
    effect upper bound: !io !fs
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestOutlineConformance(t *testing.T) {
	cases, err := conformance.LoadDir("../../spec/conformance")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		var fs []*ast.File
		for _, cf := range c.Files {
			if conformance.IsSource(cf.Path) {
				f, _ := parser.ParseFile(cf.Path, []byte(cf.Content))
				fs = append(fs, f)
			}
		}
		if Outline(fs) == "" {
			t.Errorf("%s: empty outline", c.ID())
		}
	}
}
