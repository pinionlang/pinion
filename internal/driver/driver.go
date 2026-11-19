// Package driver orchestrates parsing, code generation, Go builds (with a
// content-hash cache), running programs and running tests.
package driver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pinionlang/pinion/internal/ast"
	"github.com/pinionlang/pinion/internal/codegen"
	"github.com/pinionlang/pinion/internal/diag"
	"github.com/pinionlang/pinion/internal/parser"
	"github.com/pinionlang/pinion/runtime/pinionrt"
)

// Checker is the static checker hook (set by cmd/pinion to check.Check).
// When nil, programs are built without static checking.
var Checker func(files []*ast.File) *diag.List

// TestTimeout is the per-test time limit.
var TestTimeout = 10 * time.Second

// Program is a parsed Pinion program.
type Program struct {
	Root        string // directory paths are relative to
	Files       []*ast.File
	Sources     map[string][]byte // rel path -> content
	EntryModule string
}

// ErrDiagnostics is returned when parsing or checking fails.
type ErrDiagnostics struct{ List *diag.List }

func (e *ErrDiagnostics) Error() string { return strings.TrimRight(e.List.Human(), "\n") }

// Load parses a single .pi file (module "") or a project directory (src/**/*.pi).
func Load(path string) (*Program, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	srcs := map[string][]byte{}
	var root string
	if !st.IsDir() {
		root = filepath.Dir(path)
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		srcs[filepath.Base(path)] = b
		return parseProgram(root, srcs, true)
	}
	root = path
	srcDir := filepath.Join(path, "src")
	if _, err := os.Stat(srcDir); err != nil {
		return nil, fmt.Errorf("%s: no src/ directory", path)
	}
	err = filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".pi") {
			rel, _ := filepath.Rel(root, p)
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			srcs[filepath.ToSlash(rel)] = b
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return parseProgram(root, srcs, false)
}

// FromSources parses in-memory sources (rel path -> content). Files outside
// src/ are treated as a single-file program when there is exactly one.
func FromSources(srcs map[string][]byte) (*Program, error) {
	pi := map[string][]byte{}
	for k, v := range srcs {
		if strings.HasSuffix(k, ".pi") {
			pi[k] = v
		}
	}
	single := len(pi) == 1
	for k := range pi {
		if strings.HasPrefix(k, "src/") {
			single = false
		}
	}
	return parseProgram("", pi, single)
}

func parseProgram(root string, srcs map[string][]byte, single bool) (*Program, error) {
	p := &Program{Root: root, Sources: srcs}
	paths := make([]string, 0, len(srcs))
	for k := range srcs {
		paths = append(paths, k)
	}
	sort.Strings(paths)
	all := &diag.List{}
	for _, path := range paths {
		f, d := parser.ParseFile(path, srcs[path])
		if d != nil {
			all.Items = append(all.Items, d.Items...)
		}
		if f == nil {
			continue
		}
		if single {
			f.Module = ""
		} else {
			f.Module = parser.ModulePath(path)
		}
		p.Files = append(p.Files, f)
	}
	if all.HasErrors() {
		all.Sort()
		return nil, &ErrDiagnostics{all}
	}
	p.EntryModule = "main"
	if single {
		p.EntryModule = ""
	}
	return p, nil
}

// Check runs the static checker (if available) and returns its diagnostics.
func (p *Program) Check() *diag.List {
	if Checker == nil {
		return &diag.List{}
	}
	d := Checker(p.Files)
	if d == nil {
		d = &diag.List{}
	}
	d.Sort()
	return d
}

// GoSource generates the Go program.
func (p *Program) GoSource() ([]byte, error) {
	// The checker also annotates the AST (e.g. MemberExpr.OnOption) for codegen,
	// so it runs even when the caller asked to skip reporting diagnostics.
	if Checker != nil {
		Checker(p.Files)
	}
	return codegen.Generate(p.Files, codegen.Options{EntryModule: p.EntryModule})
}

func cacheDir() string {
	if d := os.Getenv("PINION_CACHE"); d != "" {
		return d
	}
	d, err := os.UserCacheDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "pinion")
}

func runtimeFiles() (map[string][]byte, error) {
	out := map[string][]byte{}
	ents, err := fs.ReadDir(pinionrt.Source, ".")
	if err != nil {
		return nil, err
	}
	for _, e := range ents {
		n := e.Name()
		if n == "embed.go" || strings.HasSuffix(n, "_test.go") {
			continue
		}
		b, err := fs.ReadFile(pinionrt.Source, n)
		if err != nil {
			return nil, err
		}
		out[n] = b
	}
	return out, nil
}

// Build compiles the program and returns the path of the (cached) binary.
func (p *Program) Build() (string, error) {
	src, err := p.GoSource()
	if err != nil {
		return "", err
	}
	return BuildGo(src)
}

// BuildGo builds generated Go source into a cached binary.
func BuildGo(src []byte) (string, error) {
	rtf, err := runtimeFiles()
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(runtime.Version() + runtime.GOOS + runtime.GOARCH + "\x00"))
	h.Write(src)
	names := make([]string, 0, len(rtf))
	for n := range rtf {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		h.Write([]byte("\x00" + n + "\x00"))
		h.Write(rtf[n])
	}
	sum := hex.EncodeToString(h.Sum(nil))[:24]
	dir := filepath.Join(cacheDir(), sum)
	bin := filepath.Join(dir, "prog")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if _, err := os.Stat(bin); err == nil {
		return bin, nil
	}
	tmp, err := os.MkdirTemp("", "pinion-build-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module pinionprog\n\ngo 1.23\n"), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), src, 0o644); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(tmp, "pinionrt"), 0o755); err != nil {
		return "", err
	}
	for n, b := range rtf {
		if err := os.WriteFile(filepath.Join(tmp, "pinionrt", n), b, 0o644); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	out := bin + ".tmp" + strconv.Itoa(os.Getpid())
	cmd := exec.Command("go", "build", "-trimpath", "-o", out, ".")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "CGO_ENABLED=0", "GOTOOLCHAIN=local")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go build failed: %v\n%s", err, stderr.String())
	}
	if err := os.Rename(out, bin); err != nil {
		return "", err
	}
	return bin, nil
}

// ---------- running ----------

// RunOpts controls program execution.
type RunOpts struct {
	Dir            string
	Env            []string // nil: inherit
	Args           []string
	Stdin          *os.File
	Stdout, Stderr *bytes.Buffer // nil: os.Stdout / os.Stderr
}

func exitCode(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return -1, err
}

// TestResult is one test outcome.
type TestResult struct {
	Name   string          `json:"name"`
	Result string          `json:"result"` // passed | failed | aborted | timeout
	Code   string          `json:"code,omitempty"`
	Output string          `json:"output,omitempty"`
	Diag   json.RawMessage `json:"runtime,omitempty"`
}

// ListTests returns the test names compiled into bin.
func ListTests(bin string) ([]string, error) {
	cmd := exec.Command(bin)
	cmd.Env = []string{"PINION_TEST_LIST=1"}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if l != "" {
			names = append(names, l)
		}
	}
	return names, nil
}

// RunTests runs every test in its own process, sequentially.
func RunTests(bin, dir string, env []string) ([]TestResult, error) {
	names, err := ListTests(bin)
	if err != nil {
		return nil, err
	}
	var res []TestResult
	for i, n := range names {
		res = append(res, runOneTest(bin, dir, env, i, n))
	}
	return res, nil
}

func runOneTest(bin, dir string, env []string, idx int, name string) TestResult {
	ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = dir
	if env == nil {
		env = os.Environ()
	}
	cmd.Env = append(append([]string(nil), env...), "PINION_TEST_INDEX="+strconv.Itoa(idx))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	r := TestResult{Name: name, Output: stdout.String()}
	if ctx.Err() == context.DeadlineExceeded {
		r.Result, r.Code = "timeout", "T0003"
		return r
	}
	code, _ := exitCode(err)
	rc, raw := runtimeDiag(stderr.String())
	r.Diag = raw
	switch code {
	case 0:
		r.Result = "passed"
	case 3:
		r.Result, r.Code = "failed", "T0001"
	default:
		r.Result, r.Code = "aborted", rc
		if rc == "" {
			r.Code = "T0002"
		}
	}
	return r
}

func runtimeDiag(stderr string) (string, json.RawMessage) {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(l, "{") {
			continue
		}
		var d struct {
			Schema string `json:"schema"`
			Code   string `json:"code"`
		}
		if json.Unmarshal([]byte(l), &d) == nil && d.Schema == "pinion.runtime/1" {
			return d.Code, json.RawMessage(l)
		}
	}
	return "", nil
}

// RunCase is the conformance-harness entry point: files (rel path ->
// content) are the case files (already written into workdir by the caller
// or not; sources are taken from the map). mode is "run" or "tests".
// The program runs with cwd=workdir and an environment containing only PATH.
func RunCase(files map[string][]byte, mode string, workdir string) (stdout string, stderr string, exit int, tests []TestResult, err error) {
	p, err := FromSources(files)
	if err != nil {
		return "", "", 0, nil, err
	}
	bin, err := p.Build()
	if err != nil {
		return "", "", 0, nil, err
	}
	env := []string{"PATH=" + os.Getenv("PATH")}
	if mode == "tests" {
		tests, err = RunTests(bin, workdir, env)
		if err != nil {
			return "", "", 0, nil, err
		}
		exit = 0
		var out strings.Builder
		for _, t := range tests {
			out.WriteString(t.Output)
			if t.Result != "passed" {
				exit = 1
			}
		}
		return out.String(), "", exit, tests, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = workdir
	cmd.Env = env
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	exit, err = exitCode(cmd.Run())
	return so.String(), se.String(), exit, nil, err
}
