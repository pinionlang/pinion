package pinionrt

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Defect is a program defect (abort).
type Defect struct {
	Code string
	Msg  string
}

type assertFail struct{ msg string }

// Abort raises a defect.
func Abort(code, msg string) { panic(&Defect{Code: code, Msg: msg}) }

func panicInternal(format string, args ...any) {
	panic(&Defect{Code: "R0100", Msg: "internal runtime error: " + fmt.Sprintf(format, args...)})
}

var inTest bool

// Assert implements `assert cond`.
func Assert(cond Value, src string) {
	if Truthy(cond) {
		return
	}
	if inTest {
		panic(&assertFail{msg: "assertion failed: " + src})
	}
	Abort("T0001", "assertion failed: "+src)
}

// FuncNames maps generated Go function names to Pinion names (set by codegen).
var FuncNames = map[string]string{}

// Test is a compiled test block.
type Test struct {
	Name    string
	Effects []string
	Fn      func() Value
}

func pinionName(goName string) string {
	base := goName
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimPrefix(base, "main.")
	suffix := ""
	if i := strings.Index(base, ".func"); i >= 0 {
		base, suffix = base[:i], " <closure>"
	}
	if n, ok := FuncNames[base]; ok {
		return n + suffix
	}
	return base + suffix
}

func piStack() (at string, line, col int, stack []string) {
	pcs := make([]uintptr, 128)
	n := runtime.Callers(3, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if strings.HasSuffix(f.File, ".pi") && isPinionFrame(f.Function) {
			file := strings.TrimPrefix(f.File, "pinionprog/")
			loc := fmt.Sprintf("%s:%d", file, f.Line)
			if at == "" {
				at, line = file, f.Line
			}
			stack = append(stack, loc+" "+pinionName(f.Function))
		}
		if !more {
			break
		}
	}
	return at, line, 1, stack
}

func isPinionFrame(fn string) bool {
	base := fn
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimPrefix(base, "main.")
	if i := strings.Index(base, "."); i >= 0 {
		base = base[:i]
	}
	_, ok := FuncNames[base]
	return ok
}

func report(d *Defect) {
	flushStdout()
	file, line, col, stack := piStack()
	if stack == nil {
		stack = []string{}
	}
	out := map[string]any{"schema": "pinion.runtime/1", "code": d.Code, "file": file,
		"at": []int{line, col}, "msg": d.Msg, "stack": stack}
	writeJSONLine(out)
}

// Main is the entry point of generated programs.
func Main(mainFn func() Value, tests []Test) {
	if idx := os.Getenv("PINION_TEST_INDEX"); idx != "" {
		os.Unsetenv("PINION_TEST_INDEX")
		i, _ := strconv.Atoi(idx)
		runTest(tests[i])
		return
	}
	if os.Getenv("PINION_TEST_LIST") != "" {
		for _, t := range tests {
			fmt.Println(t.Name)
		}
		return
	}
	if mainFn == nil {
		fmt.Fprintln(os.Stderr, "error: no main function")
		os.Exit(1)
	}
	var res Value
	func() {
		defer func() {
			if r := recover(); r != nil {
				d, ok := r.(*Defect)
				if !ok {
					d = &Defect{Code: "R0100", Msg: fmt.Sprint(r)}
				}
				report(d)
				os.Exit(70)
			}
		}()
		res = mainFn()
	}()
	flushStdout()
	if r, ok := res.(*Result); ok && !r.IsOk {
		msg := Debug(r.V)
		if s, ok := r.V.(string); ok {
			msg = s
		}
		fmt.Fprintln(os.Stderr, "error: "+msg)
		os.Exit(1)
	}
	os.Exit(0)
}

func runTest(t Test) {
	inTest = true
	captured := setupTest(t.Effects)
	code := 0
	func() {
		defer func() {
			if r := recover(); r != nil {
				switch x := r.(type) {
				case *assertFail:
					flushStdout()
					file, line, col, stack := piStack()
					if stack == nil {
						stack = []string{}
					}
					writeJSONLine(map[string]any{"schema": "pinion.runtime/1", "code": "T0001", "file": file,
						"at": []int{line, col}, "msg": x.msg, "stack": stack})
					code = 3
				case *Defect:
					report(x)
					code = 70
				default:
					report(&Defect{Code: "R0100", Msg: fmt.Sprint(r)})
					code = 70
				}
			}
		}()
		t.Fn()
	}()
	if captured != nil {
		realStdout.WriteString(captured.String())
	}
	flushStdout()
	os.Exit(code)
}

// Global implements a lazily initialised top-level `let`.
type Global struct {
	done bool
	busy bool
	v    Value
	Init func() Value
}

// Get returns the value, initialising it on first use.
func (g *Global) Get() Value {
	if !g.done {
		if g.busy {
			panicInternal("cyclic top-level let initialisation")
		}
		g.busy = true
		g.v = g.Init()
		g.done = true
	}
	return g.v
}

func writeJSONLine(v any) {
	enc := json.NewEncoder(os.Stderr)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
