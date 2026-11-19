package driver

import (
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pinionlang/pinion/internal/check"
)

const httpProgram = `import std/http
import std/env

data Item {
  pub name: Str
}

let items = MutList<Item>()
let app = http.App()
app.get("/items", req => http.json(items.freeze()))
app.get("/items/:i", req => {
  let i = Int.parse(req.params.get_or("i", "")).ok() else { return http.error(400, "bad index") }
  let it = items.get(i) else { return http.error(404, "no item") }
  http.json(it)
})
app.post("/items", req => {
  let it = req.json<Item>().ok() else { return http.error(400, "bad json") }
  items.push(it)
  http.json(it, status: 201)
})
app.get("/echo", req => http.text("{req.query.get_or("q", "-")} {req.headers.get_or("x-test", "-")}"))
app.listen(Int.parse(env.get("PORT") ?? "").ok() ?? 8080)
`

func TestHTTPListen(t *testing.T) {
	Checker = check.Check
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	p, err := FromSources(map[string][]byte{"server.pi": []byte(httpProgram)})
	if err != nil {
		t.Fatal(err)
	}
	if d := p.Check(); d.HasErrors() {
		t.Fatal(d.Human())
	}
	bin, err := p.Build()
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	cmd := exec.Command(bin)
	cmd.Env = []string{"PORT=" + strconv.Itoa(port)}
	var stdout strings.Builder
	cmd.Stdout = &stdout
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	for i := 0; ; i++ {
		if c, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port)); err == nil {
			c.Close()
			break
		}
		if i > 100 {
			t.Fatal("server did not start")
		}
		time.Sleep(50 * time.Millisecond)
	}
	do := func(method, path, body string) (int, string, http.Header) {
		req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
		req.Header.Set("X-Test", "yes")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b), resp.Header
	}
	for _, c := range []struct {
		method, path, body string
		status             int
		want               string
	}{
		{"GET", "/items", "", 200, "[]"},
		{"POST", "/items", `{"name":"a"}`, 201, `{"name":"a"}`},
		{"POST", "/items", `{"nam":"a"}`, 400, `{"error":"bad json"}`},
		{"GET", "/items/0", "", 200, `{"name":"a"}`},
		{"GET", "/items/5", "", 404, `{"error":"no item"}`},
		{"GET", "/items", "", 200, `[{"name":"a"}]`},
		{"GET", "/echo?q=hi%21", "", 200, "hi! yes"},
		{"GET", "/missing", "", 404, `{"error":"not found"}`},
		{"PATCH", "/items", "", 405, `{"error":"method not allowed"}`},
	} {
		st, body, hdr := do(c.method, c.path, c.body)
		if st != c.status || body != c.want {
			t.Errorf("%s %s = %d %q, want %d %q", c.method, c.path, st, body, c.status, c.want)
		}
		if strings.HasPrefix(c.want, "{") && hdr.Get("Content-Type") != "application/json" {
			t.Errorf("%s %s: content-type %q", c.method, c.path, hdr.Get("Content-Type"))
		}
	}
	if stdout.Len() != 0 {
		t.Errorf("server printed %q", stdout.String())
	}
}
