package conformance

import "testing"

func TestLoadAll(t *testing.T) {
	cases, err := LoadDir("../../spec/conformance")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 150 {
		t.Fatalf("only %d cases", len(cases))
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if seen[c.ID()] {
			t.Errorf("duplicate %s", c.ID())
		}
		seen[c.ID()] = true
		if len(c.Files) == 0 {
			t.Errorf("%s: no files", c.ID())
		}
		if (c.Expect == "error" || c.Expect == "warning") && len(c.Markers) == 0 {
			t.Errorf("%s: expect %s but no markers", c.ID(), c.Expect)
		}
	}
	t.Logf("%d cases", len(cases))
}
