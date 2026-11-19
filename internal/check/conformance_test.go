package check

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/pinionlang/pinion/internal/ast"
	"github.com/pinionlang/pinion/internal/conformance"
	"github.com/pinionlang/pinion/internal/parser"
)

func checkerCode(code string) bool {
	return strings.HasPrefix(code, "E1") || strings.HasPrefix(code, "E2") || strings.HasPrefix(code, "E3") ||
		strings.HasPrefix(code, "W3")
}

func TestConformance(t *testing.T) {
	cases, err := conformance.LoadDir("../../spec/conformance")
	if err != nil {
		t.Fatal(err)
	}
	pass, total := 0, 0
	var failures []string
	for _, cs := range cases {
		var files []*ast.File
		parseErr := false
		for _, f := range cs.Files {
			if !conformance.IsSource(f.Path) {
				continue
			}
			af, d := parser.ParseFile(f.Path, []byte(f.Content))
			for _, x := range d.Items {
				if strings.HasPrefix(x.Code, "E0") {
					parseErr = true
				}
			}
			if af != nil {
				files = append(files, af)
			}
		}
		if parseErr || len(files) == 0 {
			continue
		}
		total++
		var got, msgs []string
		func() {
			defer func() {
				if r := recover(); r != nil {
					got = append(got, fmt.Sprintf("PANIC %v", r))
				}
			}()
			for _, d := range Check(files).Items {
				if checkerCode(d.Code) {
					got = append(got, fmt.Sprintf("%s:%d %s", d.Pos.File, d.Pos.Line, d.Code))
					msgs = append(msgs, d.String())
				}
			}
		}()
		var want []string
		for _, m := range cs.Markers {
			if checkerCode(m.Code) {
				want = append(want, fmt.Sprintf("%s:%d %s", m.File, m.Line, m.Code))
			}
		}
		sort.Strings(got)
		sort.Strings(want)
		if strings.Join(got, ",") == strings.Join(want, ",") {
			pass++
			continue
		}
		failures = append(failures, fmt.Sprintf("%s (%s)\n    want %v\n    got  %v\n      %s", cs.ID(), cs.Expect, want, got,
			strings.Join(msgs, "\n      ")))
	}
	t.Logf("checker conformance: %d/%d passed", pass, total)
	for _, f := range failures {
		t.Errorf("FAIL %s", f)
	}
}
