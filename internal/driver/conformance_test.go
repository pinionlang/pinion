package driver

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinionlang/pinion/internal/check"
	"github.com/pinionlang/pinion/internal/conformance"
)

func TestRuntimeConformance(t *testing.T) {
	Checker = check.Check
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	cases, err := conformance.LoadDir("../../spec/conformance")
	if err != nil {
		t.Fatal(err)
	}
	pass, total := 0, 0
	var failed []string
	for _, c := range cases {
		if c.Expect != "run" && c.Expect != "abort" && c.Expect != "tests" {
			continue
		}
		total++
		c := c
		ok := t.Run(c.ID(), func(t *testing.T) {
			dir := t.TempDir()
			files := map[string][]byte{}
			for _, f := range c.Files {
				p := filepath.Join(dir, filepath.FromSlash(f.Path))
				os.MkdirAll(filepath.Dir(p), 0o755)
				if err := os.WriteFile(p, []byte(f.Content), 0o644); err != nil {
					t.Fatal(err)
				}
				files[f.Path] = []byte(f.Content)
			}
			mode := "run"
			if c.Expect == "tests" {
				mode = "tests"
			}
			stdout, stderr, exit, tests, err := RunCase(files, mode, dir)
			if err != nil {
				t.Fatalf("RunCase: %v", err)
			}
			if exit != c.WantExit() {
				t.Errorf("exit = %d, want %d\nstderr: %s", exit, c.WantExit(), stderr)
			}
			if c.Expect == "abort" {
				code, _ := runtimeDiag(stderr)
				if code != c.AbortCode {
					t.Errorf("abort code = %q, want %q\nstderr: %s", code, c.AbortCode, stderr)
				}
			}
			if c.Expect != "tests" {
				got := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
				if stdout == "" {
					got = nil
				}
				if strings.Join(got, "\n") != strings.Join(c.Stdout, "\n") {
					t.Errorf("stdout = %q, want %q\nstderr: %s", got, c.Stdout, stderr)
				}
			} else {
				if len(tests) != len(c.Tests) {
					t.Fatalf("got %d tests %+v, want %d", len(tests), tests, len(c.Tests))
				}
				for i, w := range c.Tests {
					g := tests[i]
					if g.Name != w.Name || g.Result != w.Result || g.Code != w.Code {
						t.Errorf("test %d = %s -> %s %s, want %s -> %s %s (%s)", i, g.Name, g.Result, g.Code, w.Name, w.Result, w.Code, g.Diag)
					}
				}
			}
		})
		if ok {
			pass++
		} else {
			failed = append(failed, c.ID())
		}
	}
	t.Logf("runtime conformance: %d/%d passed; failing: %v", pass, total, failed)
}
