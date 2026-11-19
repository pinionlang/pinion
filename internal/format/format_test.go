package format

import (
	"testing"

	"github.com/pinionlang/pinion/internal/conformance"
)

func TestIdempotentOnConformance(t *testing.T) {
	cases, err := conformance.LoadDir("../../spec/conformance")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range cases {
		for _, f := range c.Files {
			if !conformance.IsSource(f.Path) {
				continue
			}
			n++
			once, d := Format(f.Path, []byte(f.Content))
			if d.HasErrors() {
				if string(once) != f.Content {
					t.Errorf("%s: erroneous input must be returned unchanged", c.ID())
				}
				continue
			}
			twice, d2 := Format(f.Path, once)
			if d2.HasErrors() {
				t.Errorf("%s: formatted output does not parse:\n%s\n%s", c.ID(), d2.Human(), once)
				continue
			}
			if string(once) != string(twice) {
				t.Errorf("%s: not idempotent:\n--- once\n%s--- twice\n%s", c.ID(), once, twice)
			}
		}
	}
	t.Logf("%d files", n)
}

func TestCanonical(t *testing.T) {
	src := `/// doc
pub   fn f<T: Ord>( x:List< Int >,y : Int )->Int !io{
    let m = {"a":1}
      let e = {}
  xs.each( x=>{
print( "{x}" )   // hi
  })
  if !ok { return -1 }


  a.with(name: "b")[0]
  let r = 0..3
}
data Flat:Discount { pub off: Int }
fn g() -> Result<(), E> !fs !io { }
`
	want := `/// doc
pub fn f<T: Ord>(x: List<Int>, y: Int) -> Int !io {
  let m = {"a": 1}
  let e = {}
  xs.each(x => {
    print("{x}") // hi
  })
  if !ok { return -1 }

  a.with(name: "b")[0]
  let r = 0..3
}
data Flat : Discount { pub off: Int }
fn g() -> Result<(), E> !fs !io { }
`
	got, d := Format("x.pi", []byte(src))
	if d.HasErrors() {
		t.Fatal(d.Human())
	}
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
