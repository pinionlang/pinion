package check

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// The standard library and prelude are virtual: their signatures live in
// these tables (spec/STDLIB.md), written in a tiny signature syntax.

type stdModule struct {
	name  string
	fns   map[string]*FnInfo
	types map[string]*TypeInfo
}

type stdEnv struct {
	types      map[string]*TypeInfo // prelude named types (FsError, Round, ...)
	methods    map[Kind]map[string]*FnInfo
	recvParams map[Kind][]*TypeParamInfo
	statics    map[string]map[string]*FnInfo
	prelude    map[string]*FnInfo
	modules    map[string]*stdModule
	jsonT      *TypeInfo // std/json Json; its T?-returning methods also accept a Json? receiver
	httpReq    *TypeInfo // std/http Request
	httpResp   *TypeInfo // std/http Response
}

var (
	stdOnce sync.Once
	theStd  *stdEnv
)

func getStd() *stdEnv {
	stdOnce.Do(func() { theStd = buildStd() })
	return theStd
}

// ---------- signature mini-parser ----------

type sigParser struct {
	toks   []string
	i      int
	env    *stdEnv
	params map[string]*TypeParamInfo
}

func tokenizeSig(s string) []string {
	var out []string
	rs := []rune(s)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case unicode.IsLetter(r) || r == '_' || unicode.IsDigit(r) || r == '"' || r == '{' || r == '[':
			j := i
			if r == '"' {
				j++
				for j < len(rs) && rs[j] != '"' {
					j++
				}
				j++
			} else if r == '{' || r == '[' {
				j += 2
			} else {
				for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_') {
					j++
				}
			}
			out = append(out, string(rs[i:j]))
			i = j
		case r == '-' && i+1 < len(rs) && rs[i+1] == '>':
			out = append(out, "->")
			i += 2
		default:
			out = append(out, string(r))
			i++
		}
	}
	return out
}

func (p *sigParser) peek() string {
	if p.i < len(p.toks) {
		return p.toks[p.i]
	}
	return ""
}

func (p *sigParser) next() string {
	t := p.peek()
	p.i++
	return t
}

func (p *sigParser) expect(s string) {
	if t := p.next(); t != s {
		panic(fmt.Sprintf("std sig: want %q got %q in %v", s, t, p.toks))
	}
}

func (p *sigParser) typ() *Type {
	t := p.typ0()
	for p.peek() == "?" {
		p.next()
		t = optOf(t)
	}
	return t
}

func (p *sigParser) typ0() *Type {
	tok := p.next()
	switch tok {
	case "(":
		var elems []*Type
		for p.peek() != ")" {
			elems = append(elems, p.typ())
			if p.peek() == "," {
				p.next()
			}
		}
		p.expect(")")
		if len(elems) == 0 {
			return unitT
		}
		return tupleOf(elems...)
	case "fn":
		p.expect("(")
		var ps []*Type
		for p.peek() != ")" {
			ps = append(ps, p.typ())
			if p.peek() == "," {
				p.next()
			}
		}
		p.expect(")")
		ret := unitT
		if p.peek() == "->" {
			p.next()
			ret = p.typ()
		}
		return &Type{K: KFn, Args: ps, Ret: ret, Poly: true}
	}
	var args []*Type
	if p.peek() == "<" {
		p.next()
		for p.peek() != ">" {
			args = append(args, p.typ())
			if p.peek() == "," {
				p.next()
			}
		}
		p.expect(">")
	}
	if tp, ok := p.params[tok]; ok {
		return &Type{K: KParam, Param: tp}
	}
	if t := builtinType(tok, args); t != nil {
		return t
	}
	if ti, ok := p.env.types[tok]; ok {
		return named(ti, args...)
	}
	panic("std sig: unknown type " + tok)
}

func builtinType(name string, args []*Type) *Type {
	one := func(k Kind) *Type {
		if len(args) != 1 {
			return nil
		}
		return &Type{K: k, Args: args}
	}
	two := func(k Kind) *Type {
		if len(args) != 2 {
			return nil
		}
		return &Type{K: k, Args: args}
	}
	switch name {
	case "Int":
		return intT
	case "Float":
		return floatT
	case "Dec":
		return decT
	case "Bool":
		return boolT
	case "Str":
		return strT
	case "Bytes":
		return bytesT
	case "Range":
		return rangeT
	case "Instant":
		return instantT
	case "Duration":
		return durationT
	case "Never":
		return neverT
	case "Handler":
		return handlerT
	case "List":
		return one(KList)
	case "MutList":
		return one(KMutList)
	case "Set":
		return one(KSet)
	case "MutSet":
		return one(KMutSet)
	case "Map":
		return two(KMap)
	case "MutMap":
		return two(KMutMap)
	case "Result":
		return two(KResult)
	}
	return nil
}

// builtinArity returns the number of type arguments of a builtin type name, or -1.
func builtinArity(name string) int {
	switch name {
	case "Int", "Float", "Dec", "Bool", "Str", "Bytes", "Range", "Instant", "Duration", "Never":
		return 0
	case "List", "MutList", "Set", "MutSet":
		return 1
	case "Map", "MutMap", "Result":
		return 2
	}
	return -1
}

// parseSig parses `name<U>(a: T, b: T = default) -> R !eff`.
func (e *stdEnv) parseSig(src string, recv []*TypeParamInfo) *FnInfo {
	p := &sigParser{toks: tokenizeSig(src), env: e, params: map[string]*TypeParamInfo{}}
	for _, r := range recv {
		p.params[r.Name] = r
	}
	f := &FnInfo{Name: p.next(), Pub: true}
	if p.peek() == "<" {
		p.next()
		for p.peek() != ">" {
			tp := &TypeParamInfo{Name: p.next()}
			p.params[tp.Name] = tp
			f.TParams = append(f.TParams, tp)
			if p.peek() == "," {
				p.next()
			}
		}
		p.expect(">")
	}
	p.expect("(")
	for p.peek() != ")" {
		pi := &ParamInfo{Name: p.next()}
		p.expect(":")
		pi.Type = p.typ()
		if p.peek() == "=" {
			p.next()
			p.next()
			pi.HasDefault = true
		}
		f.Params = append(f.Params, pi)
		if p.peek() == "," {
			p.next()
		}
	}
	p.expect(")")
	f.Ret = unitT
	if p.peek() == "->" {
		p.next()
		f.Ret = p.typ()
	}
	for p.peek() == "!" {
		p.next()
		f.Eff |= effectBit(p.next())
	}
	return f
}

func (e *stdEnv) table(recv []*TypeParamInfo, sigs ...string) map[string]*FnInfo {
	m := map[string]*FnInfo{}
	for _, s := range sigs {
		f := e.parseSig(s, recv)
		m[f.Name] = f
	}
	return m
}

func merge(dst, src map[string]*FnInfo) map[string]*FnInfo {
	for k, v := range src {
		if _, ok := dst[k]; !ok {
			dst[k] = v
		}
	}
	return dst
}

func buildStd() *stdEnv {
	e := &stdEnv{
		types:      map[string]*TypeInfo{},
		methods:    map[Kind]map[string]*FnInfo{},
		recvParams: map[Kind][]*TypeParamInfo{},
		statics:    map[string]map[string]*FnInfo{},
		prelude:    map[string]*FnInfo{},
		modules:    map[string]*stdModule{},
	}
	enum := func(name string, variants ...string) *TypeInfo {
		ti := &TypeInfo{Name: name, Kind: DEnum, Pub: true, Methods: map[string]*FnInfo{}}
		e.types[name] = ti
		return ti
	}
	addVariants := func(ti *TypeInfo, specs ...string) {
		for _, s := range specs {
			// "Name" or "Name(a: T, b: T)"
			name, rest, _ := strings.Cut(s, "(")
			v := &VariantInfo{Name: name, Owner: ti}
			if rest != "" {
				fake := e.parseSig("v("+rest, nil)
				for _, p := range fake.Params {
					v.Fields = append(v.Fields, &FieldInfo{Name: p.Name, Type: p.Type, Pub: true})
				}
			}
			ti.Variants = append(ti.Variants, v)
		}
	}
	dataT := func(name string, fields ...string) *TypeInfo {
		ti := &TypeInfo{Name: name, Kind: DData, Pub: true, Methods: map[string]*FnInfo{}}
		e.types[name] = ti
		return ti
	}
	addFields := func(ti *TypeInfo, spec string) {
		fake := e.parseSig("v("+spec+")", nil)
		for _, p := range fake.Params {
			ti.Fields = append(ti.Fields, &FieldInfo{Name: p.Name, Type: p.Type, Pub: true})
		}
	}

	fsErr := enum("FsError")
	jsonKind := enum("JsonErrorKind")
	parseErr := enum("ParseError")
	round := enum("Round")
	jsonErr := dataT("JsonError")
	utf8 := dataT("Utf8Error")
	addVariants(fsErr, "NotFound(path: Str)", "PermissionDenied(path: Str)", "IsDirectory(path: Str)",
		"InvalidUtf8(path: Str)", "Other(path: Str, msg: Str)")
	addVariants(jsonKind, "Syntax", "UnknownField", "MissingField", "DuplicateKey", "DuplicateElement",
		"TypeMismatch", "OutOfRange")
	addVariants(parseErr, "InvalidFormat(input: Str)", "OutOfRange(input: Str)")
	addVariants(round, "HalfEven", "HalfUp", "Down", "Floor", "Ceil")
	addFields(jsonErr, "kind: JsonErrorKind, path: Str, msg: Str")
	jsonErr.Methods["code"] = e.parseSig("code() -> Str", nil)
	addFields(utf8, "byte_offset: Int")

	T := &TypeParamInfo{Name: "T"}
	K := &TypeParamInfo{Name: "K"}
	V := &TypeParamInfo{Name: "V"}
	E := &TypeParamInfo{Name: "E"}
	tT := []*TypeParamInfo{T}
	tKV := []*TypeParamInfo{K, V}
	tTE := []*TypeParamInfo{T, E}

	e.methods[KInt] = e.table(nil, "to_float() -> Float", "to_dec() -> Dec", "abs() -> Int",
		"max(o: Int) -> Int", "min(o: Int) -> Int", "pow(e: Int) -> Int")
	e.methods[KFloat] = e.table(nil, "trunc_int() -> Int", "round_int() -> Int", "abs() -> Float",
		"max(o: Float) -> Float", "min(o: Float) -> Float", "sqrt() -> Float")
	e.methods[KDec] = e.table(nil, "div(b: Dec, scale: Int, round: Round) -> Dec",
		"round(scale: Int, round: Round) -> Dec", "scale() -> Int", "to_float() -> Float",
		"trunc_int() -> Int", "abs() -> Dec")
	e.methods[KStr] = e.table(nil, "len() -> Int", "byte_len() -> Int", "is_empty() -> Bool",
		"slice(start: Int, end: Int) -> Str", "chars() -> List<Str>", "trim() -> Str", "lower() -> Str",
		"upper() -> Str", "words() -> List<Str>", "lines() -> List<Str>", "split(sep: Str) -> List<Str>",
		"contains(sub: Str) -> Bool", "starts_with(p: Str) -> Bool", "ends_with(p: Str) -> Bool",
		"index_of(sub: Str) -> Int?", "count(sub: Str) -> Int", "replace(old: Str, new: Str) -> Str",
		"repeat(n: Int) -> Str", "to_bytes() -> Bytes", "before(sep: Str) -> Str?", "after(sep: Str) -> Str?",
		`pad_left(n: Int, ch: Str = " ") -> Str`, `pad_right(n: Int, ch: Str = " ") -> Str`)
	listCommon := []string{"len() -> Int", "is_empty() -> Bool", "get(i: Int) -> T?", "first() -> T?",
		"last() -> T?", "map<U>(f: fn(T) -> U) -> List<U>", "filter(f: fn(T) -> Bool) -> List<T>",
		"each(f: fn(T))", "fold<U>(init: U, f: fn(U, T) -> U) -> U", "any(f: fn(T) -> Bool) -> Bool",
		"all(f: fn(T) -> Bool) -> Bool", "find(f: fn(T) -> Bool) -> T?", "sort() -> List<T>",
		"sort_by<K>(f: fn(T) -> K) -> List<T>", "sum() -> T", "join(sep: Str) -> Str",
		"contains(x: T) -> Bool", "append(x: T) -> List<T>", "concat(o: List<T>) -> List<T>",
		"reverse() -> List<T>", "slice(start: Int, end: Int) -> List<T>", "to_set() -> Set<T>",
		"enumerate() -> List<(Int, T)>", "count_by<K>(f: fn(T) -> K) -> Map<K, Int>",
		"group_by<K>(f: fn(T) -> K) -> Map<K, List<T>>", "take(n: Int) -> List<T>", "drop(n: Int) -> List<T>",
		"max_by<K>(f: fn(T) -> K) -> T?", "min_by<K>(f: fn(T) -> K) -> T?", "sum_by<U>(f: fn(T) -> U) -> U",
		"index_of(x: T) -> Int?", "distinct() -> List<T>"}
	e.methods[KList] = e.table(tT, listCommon...)
	e.recvParams[KList] = tT
	e.methods[KMutList] = merge(e.table(tT, "push(x: T)", "pop() -> T?", "set(i: Int, x: T)", "clear()",
		"freeze() -> List<T>", "remove_at(i: Int) -> T", "insert(i: Int, x: T)", "retain(f: fn(T) -> Bool)"), e.table(tT, listCommon...))
	e.recvParams[KMutList] = tT
	mapCommon := []string{"len() -> Int", "is_empty() -> Bool", "get(k: K) -> V?", "contains_key(k: K) -> Bool",
		"keys() -> List<K>", "values() -> List<V>", "entries() -> List<(K, V)>", "get_or(k: K, d: V) -> V",
		"map_values<U>(f: fn(V) -> U) -> Map<K, U>", "filter(f: fn(K, V) -> Bool) -> Map<K, V>"}
	e.methods[KMap] = e.table(tKV, mapCommon...)
	e.recvParams[KMap] = tKV
	e.methods[KMutMap] = merge(e.table(tKV, "set(k: K, v: V)", "remove(k: K) -> V?", "clear()",
		"freeze() -> Map<K, V>", "inc(k: K)", "update(k: K, default: V, f: fn(V) -> V)"), e.table(tKV, mapCommon...))
	e.recvParams[KMutMap] = tKV
	setCommon := []string{"len() -> Int", "is_empty() -> Bool", "contains(x: T) -> Bool",
		"to_list() -> List<T>", "map<U>(f: fn(T) -> U) -> List<U>", "each(f: fn(T))",
		"any(f: fn(T) -> Bool) -> Bool", "all(f: fn(T) -> Bool) -> Bool",
		"fold<U>(init: U, f: fn(U, T) -> U) -> U", "find(f: fn(T) -> Bool) -> T?"}
	e.methods[KSet] = merge(e.table(tT, "union(o: Set<T>) -> Set<T>", "intersect(o: Set<T>) -> Set<T>",
		"diff(o: Set<T>) -> Set<T>", "sym_diff(o: Set<T>) -> Set<T>", "is_subset(o: Set<T>) -> Bool",
		"is_disjoint(o: Set<T>) -> Bool", "filter(f: fn(T) -> Bool) -> Set<T>"), e.table(tT, setCommon...))
	e.recvParams[KSet] = tT
	e.methods[KMutSet] = merge(e.table(tT, "add(x: T) -> Bool", "remove(x: T) -> Bool", "clear()",
		"freeze() -> Set<T>", "filter(f: fn(T) -> Bool) -> List<T>"), e.table(tT, setCommon...))
	e.recvParams[KMutSet] = tT
	// Range: the List operations with T = Int, producing Lists directly.
	var rangeSigs []string
	tWord := regexp.MustCompile(`\bT\b`)
	for _, sig := range listCommon {
		if !strings.HasPrefix(sig, "join(") {
			rangeSigs = append(rangeSigs, tWord.ReplaceAllString(sig, "Int"))
		}
	}
	e.methods[KRange] = merge(e.table(nil, "to_list() -> List<Int>", "rev() -> Range", "step(n: Int) -> Range",
		"contains(x: Int) -> Bool", "len() -> Int"), e.table(nil, rangeSigs...))
	e.methods[KBytes] = e.table(nil, "len() -> Int", "slice(start: Int, end: Int) -> Bytes",
		"to_str() -> Result<Str, Utf8Error>")
	e.methods[KOption] = e.table(tT, "is_some() -> Bool", "is_none() -> Bool", "map<U>(f: fn(T) -> U) -> U?")
	e.recvParams[KOption] = tT
	e.methods[KResult] = e.table(tTE, "ok() -> T?", "map<U>(f: fn(T) -> U) -> Result<U, E>",
		"map_err<F>(f: fn(E) -> F) -> Result<T, F>", "is_ok() -> Bool", "is_err() -> Bool",
		"unwrap_or(v: T) -> T")
	e.recvParams[KResult] = tTE
	e.methods[KInstant] = e.table(nil, "add(d: Duration) -> Instant", "sub(d: Duration) -> Instant",
		"since(o: Instant) -> Duration", "unix() -> Int")
	e.methods[KDuration] = e.table(nil, "plus(o: Duration) -> Duration", "minus(o: Duration) -> Duration",
		"times(n: Int) -> Duration", "ms() -> Int", "ns() -> Int")

	e.statics["Int"] = e.table(nil, "parse(s: Str) -> Result<Int, ParseError>")
	e.statics["Float"] = e.table(nil, "parse(s: Str) -> Result<Float, ParseError>")
	e.statics["Dec"] = e.table(nil, "parse(s: Str) -> Result<Dec, ParseError>")
	e.statics["Bytes"] = e.table(nil, "from_list(xs: List<Int>) -> Bytes")

	e.prelude = e.table(nil, "print(s: Str) !io", "debug<T>(x: T) -> Str", `todo(msg: Str = "") -> Never`,
		`unreachable(msg: Str = "") -> Never`, "Set<T>(xs: List<T>) -> Set<T>", "MutList<T>() -> MutList<T>",
		"MutMap<K, V>() -> MutMap<K, V>", "MutSet<T>() -> MutSet<T>",
		"Duration(ns: Int = 0, us: Int = 0, ms: Int = 0, s: Int = 0, m: Int = 0, h: Int = 0) -> Duration")

	mod := func(name string, sigs ...string) *stdModule {
		m := &stdModule{name: name, fns: e.table(nil, sigs...), types: map[string]*TypeInfo{}}
		for k, v := range e.types {
			m.types[k] = v
		}
		e.modules[name] = m
		return m
	}
	mod("fs", "read(path: Str) -> Result<Str, FsError> !fs", "read_bytes(path: Str) -> Result<Bytes, FsError> !fs",
		"write(path: Str, text: Str) -> Result<(), FsError> !fs", "exists(path: Str) -> Bool !fs",
		"remove(path: Str) -> Result<(), FsError> !fs", "fake(files: Map<Str, Str> = {}) -> Handler")
	mod("env", "args() -> List<Str> !env", "get(name: Str) -> Str? !env",
		"fake(args: List<Str> = [], vars: Map<Str, Str> = {}) -> Handler")
	mod("io", "read_line() -> Str? !io", "read_all() -> Str !io", "eprint(s: Str) !io",
		`fake(input: Str = "") -> Handler`)
	mod("time", "now() -> Instant !time", "now_unix() -> Int !time", "sleep(d: Duration) !time",
		"fake(at: Int = 0) -> Handler")
	mod("rand", "int(lo: Int, hi: Int) -> Int !rand", "float() -> Float !rand", "fake(seed: Int = 0) -> Handler")
	e.buildJSONValue()
	j := mod("json", "encode<T>(v: T) -> Str", "decode<T>(s: Str) -> Result<T, JsonError>",
		"parse(s: Str) -> Result<Json, JsonError>")
	j.fns["encode"].Special = "encode"
	j.fns["decode"].Special = "decode"
	e.buildHTTP(dataT, addFields)
	// Json is visible only as json.Json (it would shadow user types named Json).
	delete(e.types, "Json")
	delete(e.modules["http"].types, "Json")
	return e
}

// buildJSONValue adds std/json's dynamic value enum (DESIGN 3.7.1).
func (e *stdEnv) buildJSONValue() {
	ti := &TypeInfo{Name: "Json", Kind: DEnum, Pub: true, Methods: map[string]*FnInfo{}}
	e.types["Json"] = ti
	e.jsonT = ti
	self := named(ti)
	ti.Variants = append(ti.Variants, &VariantInfo{Name: "Null", Owner: ti})
	for _, v := range []struct {
		name string
		t    *Type
	}{{"Bool", boolT}, {"Int", intT}, {"Float", floatT}, {"Str", strT},
		{"Arr", &Type{K: KList, Args: []*Type{self}}}, {"Obj", &Type{K: KMap, Args: []*Type{strT, self}}}} {
		ti.Variants = append(ti.Variants, &VariantInfo{Name: v.name, Owner: ti,
			Fields: []*FieldInfo{{Name: "", Type: v.t, Pub: true}}})
	}
	for _, s := range []string{"get(key: Str) -> Json?", "at(i: Int) -> Json?", "as_str() -> Str?",
		"as_int() -> Int?", "as_float() -> Float?", "as_bool() -> Bool?", "as_list() -> List<Json>?",
		"as_obj() -> Map<Str, Json>?", "is_null() -> Bool", "has(key: Str) -> Bool", "keys() -> List<Str>",
		"kind() -> Str",
		// validation helpers (DESIGN 3.7.1): Err is a readable message "field: problem"
		"only(keys: List<Str>) -> Result<(), Str>",
		"req_str(k: Str, min: Int = _, max: Int = _) -> Result<Str, Str>",
		"opt_str(k: Str, min: Int = _, max: Int = _) -> Result<Str?, Str>",
		"req_int(k: Str, min: Int = _, max: Int = _) -> Result<Int, Str>",
		"opt_int(k: Str, min: Int = _, max: Int = _) -> Result<Int?, Str>",
		"req_float(k: Str, min: Float = _, max: Float = _) -> Result<Float, Str>",
		"opt_float(k: Str, min: Float = _, max: Float = _) -> Result<Float?, Str>",
		"req_bool(k: Str) -> Result<Bool, Str>", "opt_bool(k: Str) -> Result<Bool?, Str>",
		"str_list(k: Str) -> Result<List<Str>?, Str>"} {
		f := e.parseSig(s, nil)
		ti.Methods[f.Name] = f
	}
}

// buildHTTP adds std/http (DESIGN 3.21). Its types are visible only through
// the module (http.Request), not in the prelude.
func (e *stdEnv) buildHTTP(dataT func(string, ...string) *TypeInfo, addFields func(*TypeInfo, string)) {
	req := dataT("Request")
	resp := dataT("Response")
	app := &TypeInfo{Name: "App", Kind: DClass, Pub: true, Methods: map[string]*FnInfo{}}
	e.types["App"] = app
	defer func() {
		delete(e.types, "Request")
		delete(e.types, "Response")
		delete(e.types, "App")
	}()
	addFields(req, "method: Str, path: Str, params: Map<Str, Str>, query: Map<Str, Str>, headers: Map<Str, Str>, body: Str")
	addFields(resp, "status: Int, body: Str, headers: Map<Str, Str>")
	for _, ti := range []*TypeInfo{req, resp} {
		f := e.parseSig("json<T>() -> Result<T, JsonError>", nil)
		f.Special = "decode"
		ti.Methods["json"] = f
	}
	for _, ti := range []*TypeInfo{req, resp} {
		ti.Methods["json_value"] = e.parseSig("json_value() -> Result<Json, JsonError>", nil)
	}
	e.httpReq, e.httpResp = req, resp
	for _, ti := range []*TypeInfo{req} {
		ti.Methods["json_obj"] = e.parseSig("json_obj() -> Result<Json, Str>", nil)
	}
	for _, m := range []string{"get", "post", "put", "patch", "delete"} {
		// the handler may also return Result<Response, Response | Str> (see callFn "route")
		f := e.parseSig(m+"(path: Str, h: fn(Request) -> Response)", nil)
		f.Special = "route"
		app.Methods[m] = f
	}
	app.Methods["handle"] = e.parseSig("handle(req: Request) -> Response", nil)
	app.Methods["listen"] = e.parseSig(`listen(port: Int, host: Str = "127.0.0.1") !net`, nil)
	m := &stdModule{name: "http", types: map[string]*TypeInfo{}, fns: e.table(nil, "App() -> App",
		`request(method: Str, path: Str, body: Str = "", headers: Map<Str, Str> = {}) -> Request`,
		"json<T>(v: T, status: Int = 200) -> Response", "text(s: Str, status: Int = 200) -> Response",
		"empty(status: Int) -> Response", "error(status: Int, msg: Str) -> Response")}
	for k, v := range e.types {
		m.types[k] = v
	}
	m.fns["json"].Special = "encode"
	e.modules["http"] = m
}
