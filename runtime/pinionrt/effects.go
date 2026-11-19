package pinionrt

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// ---------- effect implementations ----------

// FSImpl is the std/fs implementation.
type FSImpl interface {
	Read(path string) ([]byte, Value) // Value: FsError or nil
	Write(path string, data []byte) Value
	Exists(path string) bool
	Remove(path string) Value
}

type realFS struct{}

func osFsErr(path string, err error) Value {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fsErr("NotFound", path)
	case errors.Is(err, fs.ErrPermission):
		return fsErr("PermissionDenied", path)
	}
	if st, e := os.Stat(path); e == nil && st.IsDir() {
		return fsErr("IsDirectory", path)
	}
	return fsErr("Other", path, err.Error())
}

func (realFS) Read(p string) ([]byte, Value) {
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return nil, fsErr("IsDirectory", p)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, osFsErr(p, err)
	}
	return b, nil
}

func (realFS) Write(p string, data []byte) Value {
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return fsErr("IsDirectory", p)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return osFsErr(p, err)
	}
	return nil
}

func (realFS) Exists(p string) bool { _, err := os.Stat(p); return err == nil }

func (realFS) Remove(p string) Value {
	if err := os.Remove(p); err != nil {
		return osFsErr(p, err)
	}
	return nil
}

type fakeFS struct {
	files map[string][]byte
}

func (f *fakeFS) Read(p string) ([]byte, Value) {
	b, ok := f.files[p]
	if !ok {
		return nil, fsErr("NotFound", p)
	}
	return b, nil
}

func (f *fakeFS) Write(p string, data []byte) Value {
	f.files[p] = append([]byte(nil), data...)
	return nil
}

func (f *fakeFS) Exists(p string) bool { _, ok := f.files[p]; return ok }

func (f *fakeFS) Remove(p string) Value {
	if _, ok := f.files[p]; !ok {
		return fsErr("NotFound", p)
	}
	delete(f.files, p)
	return nil
}

// EnvImpl is std/env.
type EnvImpl struct {
	Args []string
	Vars map[string]string
	Real bool
}

// IOImpl is std/io.
type IOImpl struct {
	Out    io.Writer
	Err    io.Writer
	In     *bufio.Reader
	buf    *strings.Builder // captured output (test mode)
	stdout *bufio.Writer
}

// TimeImpl is std/time.
type TimeImpl struct {
	Real  bool
	NowNS int64
}

// RandImpl is std/rand.
type RandImpl struct{ R *rand.Rand }

var (
	fsStack   []FSImpl
	envStack  []*EnvImpl
	ioStack   []*IOImpl
	timeStack []*TimeImpl
	randStack []*RandImpl
)

var realStdout = bufio.NewWriterSize(os.Stdout, 32<<10)

func flushStdout() { _ = realStdout.Flush() }

func realIO() *IOImpl {
	return &IOImpl{Out: realStdout, Err: os.Stderr, In: bufio.NewReader(os.Stdin), stdout: realStdout}
}

func newRand(seed int64) *RandImpl {
	return &RandImpl{R: rand.New(rand.NewPCG(uint64(seed), 0x9e3779b97f4a7c15))}
}

func setupReal() {
	fsStack = []FSImpl{realFS{}}
	envStack = []*EnvImpl{{Args: os.Args[1:], Real: true}}
	ioStack = []*IOImpl{realIO()}
	timeStack = []*TimeImpl{{Real: true}}
	randStack = []*RandImpl{newRand(time.Now().UnixNano())}
}

// setupTest installs the hermetic test defaults; real lists effects that use
// the real implementation.
func setupTest(real []string) *strings.Builder {
	setupReal()
	isReal := map[string]bool{}
	for _, r := range real {
		isReal[r] = true
	}
	var captured *strings.Builder
	if !isReal["fs"] {
		fsStack = []FSImpl{&fakeFS{files: map[string][]byte{}}}
	}
	if !isReal["env"] {
		envStack = []*EnvImpl{{Vars: map[string]string{}}}
	}
	if !isReal["io"] {
		captured = &strings.Builder{}
		ioStack = []*IOImpl{{Out: captured, Err: captured, In: bufio.NewReader(strings.NewReader("")), buf: captured}}
	}
	if !isReal["time"] {
		timeStack = []*TimeImpl{{NowNS: 946684800 * 1e9}}
	}
	if !isReal["rand"] {
		randStack = []*RandImpl{newRand(0)}
	}
	netReal = isReal["net"]
	return captured
}

func init() { setupReal() }

func curFS() FSImpl      { return fsStack[len(fsStack)-1] }
func curEnv() *EnvImpl   { return envStack[len(envStack)-1] }
func curIO() *IOImpl     { return ioStack[len(ioStack)-1] }
func curTime() *TimeImpl { return timeStack[len(timeStack)-1] }
func curRand() *RandImpl { return randStack[len(randStack)-1] }

// PushHandler installs an effect replacement (`with eff: h { ... }`).
func PushHandler(effect string, h Value) {
	hd, ok := h.(*Handler)
	if !ok || hd.Effect != effect {
		panicInternal("with %s: handler is not a %s fake", effect, effect)
	}
	switch effect {
	case "fs":
		fsStack = append(fsStack, hd.Impl.(FSImpl))
	case "env":
		envStack = append(envStack, hd.Impl.(*EnvImpl))
	case "io":
		ioStack = append(ioStack, hd.Impl.(*IOImpl))
	case "time":
		timeStack = append(timeStack, hd.Impl.(*TimeImpl))
	case "rand":
		randStack = append(randStack, hd.Impl.(*RandImpl))
	default:
		panicInternal("unknown effect %s", effect)
	}
}

// PopHandler restores the outer implementation.
func PopHandler(effect string) {
	switch effect {
	case "fs":
		fsStack = fsStack[:len(fsStack)-1]
	case "env":
		envStack = envStack[:len(envStack)-1]
	case "io":
		ioStack = ioStack[:len(ioStack)-1]
	case "time":
		timeStack = timeStack[:len(timeStack)-1]
	case "rand":
		randStack = randStack[:len(randStack)-1]
	}
}

// WithEffect runs body with handler h installed, restoring it afterwards
// (also on defects). body returns (value, control) where control is passed through.
func WithEffect(effect string, h Value, body func()) {
	PushHandler(effect, h)
	defer PopHandler(effect)
	body()
}

// ---------- prelude io ----------

// Print is print(s).
func Print(s Value) Value {
	o := curIO()
	io.WriteString(o.Out, Display(s))
	io.WriteString(o.Out, "\n")
	return UnitV
}

// ---------- std modules ----------

type stdFn struct {
	params []string
	fn     func(a []Value) Value
}

func unitOrErr(e Value) Value {
	if e != nil {
		return Err(e)
	}
	return Ok(UnitV)
}

func strMap(v Value) map[string]string {
	out := map[string]string{}
	if m, ok := v.(*Map); ok {
		for i, k := range m.keys.items {
			out[k.(string)] = Display(m.vals[i])
		}
	}
	return out
}

func strs(v Value) []string {
	var out []string
	for _, x := range elems(v) {
		out = append(out, x.(string))
	}
	return out
}

var stdFns = map[string]stdFn{
	"fs.read": {[]string{"path"}, func(a []Value) Value {
		p := a[0].(string)
		b, e := curFS().Read(p)
		if e != nil {
			return Err(e)
		}
		if !utf8.Valid(b) {
			return Err(fsErr("InvalidUtf8", p))
		}
		return Ok(string(b))
	}},
	"fs.read_bytes": {[]string{"path"}, func(a []Value) Value {
		b, e := curFS().Read(a[0].(string))
		if e != nil {
			return Err(e)
		}
		return Ok(Bytes(b))
	}},
	"fs.write": {[]string{"path", "text"}, func(a []Value) Value {
		var data []byte
		switch x := a[1].(type) {
		case string:
			data = []byte(x)
		case Bytes:
			data = []byte(x)
		}
		return unitOrErr(curFS().Write(a[0].(string), data))
	}},
	"fs.exists": {[]string{"path"}, func(a []Value) Value { return curFS().Exists(a[0].(string)) }},
	"fs.remove": {[]string{"path"}, func(a []Value) Value { return unitOrErr(curFS().Remove(a[0].(string))) }},
	"fs.fake": {[]string{"files"}, func(a []Value) Value {
		files := map[string][]byte{}
		for k, v := range strMap(a[0]) {
			files[k] = []byte(v)
		}
		return &Handler{Effect: "fs", Impl: &fakeFS{files: files}}
	}},
	"env.args": {nil, func(a []Value) Value { return strList(curEnv().Args) }},
	"env.get": {[]string{"name"}, func(a []Value) Value {
		e := curEnv()
		n := a[0].(string)
		if e.Real {
			v, ok := os.LookupEnv(n)
			if !ok {
				return None
			}
			return v
		}
		v, ok := e.Vars[n]
		if !ok {
			return None
		}
		return v
	}},
	"env.fake": {[]string{"args", "vars"}, func(a []Value) Value {
		e := &EnvImpl{Vars: map[string]string{}}
		if a[0] != nil {
			e.Args = strs(a[0])
		}
		if a[1] != nil {
			e.Vars = strMap(a[1])
		}
		return &Handler{Effect: "env", Impl: e}
	}},
	"io.read_line": {nil, func(a []Value) Value {
		in := curIO().In
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			return None
		}
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		return line
	}},
	"io.read_all": {nil, func(a []Value) Value {
		b, _ := io.ReadAll(curIO().In)
		return strings.ToValidUTF8(string(b), "�")
	}},
	"io.eprint": {[]string{"s"}, func(a []Value) Value {
		o := curIO()
		if o.stdout != nil {
			_ = o.stdout.Flush()
		}
		io.WriteString(o.Err, Display(a[0])+"\n")
		return UnitV
	}},
	"io.print": {[]string{"s"}, func(a []Value) Value { return Print(a[0]) }},
	"io.fake": {[]string{"input"}, func(a []Value) Value {
		in := ""
		if a[0] != nil {
			in = a[0].(string)
		}
		cur := curIO()
		return &Handler{Effect: "io", Impl: &IOImpl{Out: cur.Out, Err: cur.Err, In: bufio.NewReader(strings.NewReader(in)), buf: cur.buf, stdout: cur.stdout}}
	}},
	"time.now": {nil, func(a []Value) Value { return Instant{NS: nowNS()} }},
	"time.now_unix": {nil, func(a []Value) Value {
		return instantMethod(Instant{NS: nowNS()}, "unix", nil)
	}},
	"time.sleep": {[]string{"d"}, func(a []Value) Value {
		d := int64(a[0].(Duration))
		t := curTime()
		if t.Real {
			if d > 0 {
				time.Sleep(time.Duration(d))
			}
		} else if d > 0 {
			t.NowNS = addDur(t.NowNS, d)
		}
		return UnitV
	}},
	"time.fake": {[]string{"at"}, func(a []Value) Value {
		return &Handler{Effect: "time", Impl: &TimeImpl{NowNS: mulInt(a[0].(int64), 1e9)}}
	}},
	"rand.int": {[]string{"lo", "hi"}, func(a []Value) Value {
		lo, hi := a[0].(int64), a[1].(int64)
		if hi <= lo {
			Abort("R0102", "rand.int: empty range")
		}
		span := uint64(hi - lo)
		return lo + int64(curRand().R.Uint64N(span))
	}},
	"rand.float": {nil, func(a []Value) Value { return curRand().R.Float64() }},
	"rand.fake": {[]string{"seed"}, func(a []Value) Value {
		return &Handler{Effect: "rand", Impl: newRand(a[0].(int64))}
	}},
	"json.encode": {[]string{"v"}, func(a []Value) Value { return JSONEncode(a[0]) }},
}

func nowNS() int64 {
	t := curTime()
	if t.Real {
		return time.Now().UnixNano()
	}
	return t.NowNS
}

// stdDefaults holds default values of optional std parameters, by index.
var stdDefaults = map[string]map[int]Value{}

// Std calls a std module function: Std("fs", "read", names, args...).
func Std(mod, name string, names []string, args ...Value) Value {
	f, ok := stdFns[mod+"."+name]
	if !ok {
		panicInternal("std/%s has no function %s", mod, name)
	}
	optional := map[string]bool{"env.fake": true, "io.fake": true}[mod+"."+name]
	defs := stdDefaults[mod+"."+name]
	vals := Reorder(mod+"."+name, f.params, names, args, func(i int) (Value, bool) {
		if d, ok := defs[i]; ok {
			return d, true
		}
		if optional {
			return nil, true
		}
		return nil, false
	})
	return f.fn(vals)
}

// StdFunc returns a std function as a value (e.g. passing fs.read).
func StdFunc(mod, name string) Value {
	f, ok := stdFns[mod+"."+name]
	if !ok {
		panicInternal("std/%s has no function %s", mod, name)
	}
	return MkFunc(len(f.params), mod+"."+name, f.fn)
}
