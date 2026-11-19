package driver

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
)

const snapSrc = `class Cart {
  pub items: List<Str>

  fn remove(index: Int) -> Str {
    self.items[index]
  }
}

fn main() {
  let c = Cart(items: ["a", "b", "c"])
  let _ = c.remove(3)
}

test "hermetic" {
  let c = Cart(items: ["a", "b", "c"])
  let _ = c.remove(3)
}

test "real" !fs {
  let c = Cart(items: ["a", "b", "c"])
  let _ = c.remove(3)
}
`

type snapDiag struct {
	Code     string `json:"code"`
	At       []int  `json:"at"`
	Snapshot *struct {
		Self map[string]any `json:"self"`
		Args map[string]any `json:"args"`
	} `json:"snapshot"`
}

func parseDiag(t *testing.T, raw []byte) snapDiag {
	t.Helper()
	_, line := runtimeDiag(string(raw))
	if line == nil {
		t.Fatalf("no pinion.runtime/1 line in %q", raw)
	}
	var d snapDiag
	if err := json.Unmarshal(line, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func checkSnap(t *testing.T, what string, d snapDiag, want bool) {
	t.Helper()
	if d.Code != "R0102" {
		t.Errorf("%s: code %q, want R0102", what, d.Code)
	}
	if len(d.At) != 2 || d.At[0] != 5 || d.At[1] != 5 {
		t.Errorf("%s: at %v, want [5 5]", what, d.At)
	}
	if !want {
		if d.Snapshot != nil {
			t.Errorf("%s: unexpected snapshot", what)
		}
		return
	}
	if d.Snapshot == nil {
		t.Fatalf("%s: snapshot missing", what)
	}
	if d.Snapshot.Self["class"] != "Cart" {
		t.Errorf("%s: self = %v", what, d.Snapshot.Self)
	}
	if items, _ := d.Snapshot.Self["items"].([]any); len(items) != 3 || items[0] != "a" {
		t.Errorf("%s: self.items = %v", what, d.Snapshot.Self["items"])
	}
	if d.Snapshot.Args["index"] != float64(3) {
		t.Errorf("%s: args = %v", what, d.Snapshot.Args)
	}
}

func TestSnapshots(t *testing.T) {
	p, err := FromSources(map[string][]byte{"main.pi": []byte(snapSrc)})
	if err != nil {
		t.Fatal(err)
	}
	bin, err := p.Build()
	if err != nil {
		t.Fatal(err)
	}
	path := "PATH=" + os.Getenv("PATH")
	run := func(env ...string) []byte {
		cmd := exec.Command(bin)
		cmd.Dir = t.TempDir()
		cmd.Env = append([]string{path}, env...)
		var se bytes.Buffer
		cmd.Stderr = &se
		if code, _ := exitCode(cmd.Run()); code != 70 {
			t.Fatalf("exit %d, want 70; stderr %s", code, se.String())
		}
		return se.Bytes()
	}
	checkSnap(t, "run", parseDiag(t, run()), false)
	checkSnap(t, "run --snapshot", parseDiag(t, run("PINION_SNAPSHOT=1")), true)

	res, err := RunTests(bin, t.TempDir(), []string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("got %d tests", len(res))
	}
	checkSnap(t, "hermetic test", parseDiag(t, res[0].Diag), true)
	checkSnap(t, "!fs test", parseDiag(t, res[1].Diag), false)

	res, err = RunTests(bin, t.TempDir(), []string{path, "PINION_SNAPSHOT=1"})
	if err != nil {
		t.Fatal(err)
	}
	checkSnap(t, "!fs test --snapshot", parseDiag(t, res[1].Diag), true)
}
