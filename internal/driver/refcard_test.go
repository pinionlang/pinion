package driver

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/pinionlang/pinion/internal/check"
)

// TestReferenceCard checks that every pinion snippet in spec/REFERENCE-CARD.md
// compiles without diagnostics, and runs the tests of snippets that have any.
func TestReferenceCard(t *testing.T) {
	Checker = check.Check
	src, err := os.ReadFile("../../spec/REFERENCE-CARD.md")
	if err != nil {
		t.Fatal(err)
	}
	var snippets []string
	for _, part := range strings.Split(string(src), "```pinion\n")[1:] {
		code, _, ok := strings.Cut(part, "```")
		if !ok {
			t.Fatal("unterminated pinion block")
		}
		snippets = append(snippets, code)
	}
	if len(snippets) < 2 {
		t.Fatalf("found %d snippets", len(snippets))
	}
	_, goErr := exec.LookPath("go")
	for i, code := range snippets {
		files := map[string][]byte{"main.pi": []byte(code)}
		p, err := FromSources(files)
		if err != nil {
			t.Fatalf("snippet %d: %v", i, err)
		}
		if d := p.Check(); len(d.Items) > 0 {
			t.Errorf("snippet %d:\n%s", i, d.Human())
			continue
		}
		if goErr != nil || !strings.Contains(code, "\ntest \"") {
			continue
		}
		_, stderr, _, tests, err := RunCase(files, "tests", t.TempDir())
		if err != nil {
			t.Fatalf("snippet %d: %v", i, err)
		}
		if len(tests) == 0 {
			t.Errorf("snippet %d: no tests ran\n%s", i, stderr)
		}
		for _, r := range tests {
			if r.Result != "passed" {
				t.Errorf("snippet %d: test %s -> %s %s %s", i, r.Name, r.Result, r.Code, r.Diag)
			}
		}
	}
}
