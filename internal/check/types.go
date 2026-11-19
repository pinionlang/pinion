package check

import (
	"sort"
	"strings"

	"github.com/pinionlang/pinion/internal/ast"
	"github.com/pinionlang/pinion/internal/token"
)

// Kind classifies a checker type.
type Kind int

const (
	KError Kind = iota // unknown / already reported; compatible with everything
	KVoid              // expectation only: the value is discarded
	KUnit
	KNever
	KInt
	KFloat
	KDec
	KBool
	KStr
	KBytes
	KRange
	KInstant
	KDuration
	KHandler // effect replacement handler (fs.fake(...))
	KList
	KMutList
	KMap
	KMutMap
	KSet
	KMutSet
	KOption
	KResult
	KTuple
	KFn
	KNamed // user or prelude data/class/enum/interface
	KParam // type parameter
	KVar   // inference variable
)

// Type is a checker type. Types are immutable except for inference variables.
type Type struct {
	K     Kind
	Args  []*Type // elements, type arguments, fn params
	Ret   *Type   // fn result
	Eff   Effects // fn effects
	Poly  bool    // fn param of a std higher-order function: callback effects pass through
	Info  *TypeInfo
	Param *TypeParamInfo
	V     *TVar
}

// TVar is an inference variable.
type TVar struct {
	id    int
	bound *Type
}

var (
	errT      = &Type{K: KError}
	voidT     = &Type{K: KVoid}
	unitT     = &Type{K: KUnit}
	neverT    = &Type{K: KNever}
	intT      = &Type{K: KInt}
	floatT    = &Type{K: KFloat}
	decT      = &Type{K: KDec}
	boolT     = &Type{K: KBool}
	strT      = &Type{K: KStr}
	bytesT    = &Type{K: KBytes}
	rangeT    = &Type{K: KRange}
	instantT  = &Type{K: KInstant}
	durationT = &Type{K: KDuration}
	handlerT  = &Type{K: KHandler}
)

func listOf(t *Type) *Type      { return &Type{K: KList, Args: []*Type{t}} }
func optOf(t *Type) *Type       { return &Type{K: KOption, Args: []*Type{t}} }
func resultOf(t, e *Type) *Type { return &Type{K: KResult, Args: []*Type{t, e}} }
func tupleOf(ts ...*Type) *Type { return &Type{K: KTuple, Args: ts} }
func named(i *TypeInfo, args ...*Type) *Type {
	return &Type{K: KNamed, Info: i, Args: args}
}

// Effects is a set of effect names.
type Effects uint32

var effectNames = []string{"io", "fs", "env", "time", "rand", "db", "net"}

func effectBit(name string) Effects {
	for i, n := range effectNames {
		if n == name {
			return 1 << uint(i)
		}
	}
	return 0
}

func (e Effects) has(o Effects) bool { return e&o == o }

func (e Effects) names() []string {
	var out []string
	for i, n := range effectNames {
		if e&(1<<uint(i)) != 0 {
			out = append(out, n)
		}
	}
	return out
}

func (e Effects) String() string {
	var b strings.Builder
	for i, n := range e.names() {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString("!" + n)
	}
	return b.String()
}

func effectsOf(es []ast.Effect) Effects {
	var e Effects
	for _, x := range es {
		e |= effectBit(x.Name)
	}
	return e
}

// ---------- declarations ----------

type DeclKind int

const (
	DData DeclKind = iota
	DClass
	DEnum
	DInterface
	DAlias
)

type TypeParamInfo struct {
	Name   string
	Cons   []string
	Frozen bool // data type parameter: Frozen is required
}

func (p *TypeParamInfo) hasCons(c string) bool {
	for _, x := range p.Cons {
		if x == c {
			return true
		}
	}
	return false
}

type FieldInfo struct {
	Name       string
	Type       *Type
	Pub        bool
	Mut        bool
	HasDefault bool
	Pos        token.Pos
	Decl       *ast.Field
}

type VariantInfo struct {
	Name   string
	Fields []*FieldInfo // Name "" for positional
	Owner  *TypeInfo
	Pos    token.Pos
}

type TypeInfo struct {
	Name     string
	File     string // defining file path ("" for prelude)
	Module   *module
	Kind     DeclKind
	Pub      bool
	Params   []*TypeParamInfo
	Fields   []*FieldInfo
	Methods  map[string]*FnInfo
	Variants []*VariantInfo
	Init     *FnInfo
	Impls    []*TypeInfo
	Alias    *Type
	Pos      token.Pos

	TypeDecl  *ast.TypeDecl
	EnumDecl  *ast.EnumDecl
	IfaceDecl *ast.InterfaceDecl
	AliasDecl *ast.AliasDecl

	resolving bool
}

func (i *TypeInfo) field(name string) *FieldInfo {
	for _, f := range i.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func (i *TypeInfo) variant(name string) *VariantInfo {
	for _, v := range i.Variants {
		if v.Name == name {
			return v
		}
	}
	return nil
}

func (i *TypeInfo) hasPrivateFields() bool {
	for _, f := range i.Fields {
		if !f.Pub {
			return true
		}
	}
	return false
}

// selfType is the type as seen inside its own declaration.
func (i *TypeInfo) selfType() *Type {
	args := make([]*Type, len(i.Params))
	for k, p := range i.Params {
		args[k] = &Type{K: KParam, Param: p}
	}
	return named(i, args...)
}

func (i *TypeInfo) implements(iface *TypeInfo) bool {
	for _, x := range i.Impls {
		if x == iface {
			return true
		}
	}
	return false
}

type ParamInfo struct {
	Name       string
	Type       *Type
	HasDefault bool
}

type FnInfo struct {
	Name    string
	TParams []*TypeParamInfo
	Params  []*ParamInfo
	Ret     *Type
	Eff     Effects
	Mut     bool
	Pub     bool
	Owner   *TypeInfo
	File    string
	Decl    *ast.FnDecl
	Special string // "encode", "decode" for json
	Pos     token.Pos
	ImplOf  *FnInfo // interface method implemented
}

func (f *FnInfo) fnType() *Type {
	ps := make([]*Type, len(f.Params))
	for i, p := range f.Params {
		ps[i] = p.Type
	}
	return &Type{K: KFn, Args: ps, Ret: f.Ret, Eff: f.Eff}
}

// ---------- type operations ----------

func prune(t *Type) *Type {
	for t != nil && t.K == KVar && t.V.bound != nil {
		t = t.V.bound
	}
	return t
}

func subst(t *Type, m map[*TypeParamInfo]*Type) *Type {
	if t == nil || len(m) == 0 {
		return t
	}
	t = prune(t)
	switch t.K {
	case KParam:
		if r, ok := m[t.Param]; ok {
			return r
		}
		return t
	case KVar:
		return t
	}
	if len(t.Args) == 0 && t.Ret == nil {
		return t
	}
	n := *t
	n.Args = make([]*Type, len(t.Args))
	for i, a := range t.Args {
		n.Args[i] = subst(a, m)
	}
	n.Ret = subst(t.Ret, m)
	return &n
}

func hasVars(t *Type) bool {
	t = prune(t)
	if t == nil {
		return false
	}
	if t.K == KVar {
		return true
	}
	for _, a := range t.Args {
		if hasVars(a) {
			return true
		}
	}
	return hasVars(t.Ret)
}

func isErr(t *Type) bool {
	t = prune(t)
	return t == nil || t.K == KError
}

// containsErr reports whether t mentions an error type anywhere.
func containsErr(t *Type) bool {
	t = prune(t)
	if t == nil {
		return false
	}
	if t.K == KError {
		return true
	}
	for _, a := range t.Args {
		if containsErr(a) {
			return true
		}
	}
	return containsErr(t.Ret)
}

func typeString(t *Type) string {
	t = prune(t)
	if t == nil {
		return "?"
	}
	switch t.K {
	case KError:
		return "<error>"
	case KVoid, KUnit:
		return "()"
	case KNever:
		return "Never"
	case KInt:
		return "Int"
	case KFloat:
		return "Float"
	case KDec:
		return "Dec"
	case KBool:
		return "Bool"
	case KStr:
		return "Str"
	case KBytes:
		return "Bytes"
	case KRange:
		return "Range"
	case KInstant:
		return "Instant"
	case KDuration:
		return "Duration"
	case KHandler:
		return "Handler"
	case KList:
		return "List<" + typeString(t.Args[0]) + ">"
	case KMutList:
		return "MutList<" + typeString(t.Args[0]) + ">"
	case KSet:
		return "Set<" + typeString(t.Args[0]) + ">"
	case KMutSet:
		return "MutSet<" + typeString(t.Args[0]) + ">"
	case KMap:
		return "Map<" + typeString(t.Args[0]) + ", " + typeString(t.Args[1]) + ">"
	case KMutMap:
		return "MutMap<" + typeString(t.Args[0]) + ", " + typeString(t.Args[1]) + ">"
	case KOption:
		return typeString(t.Args[0]) + "?"
	case KResult:
		return "Result<" + typeString(t.Args[0]) + ", " + typeString(t.Args[1]) + ">"
	case KTuple:
		parts := make([]string, len(t.Args))
		for i, a := range t.Args {
			parts[i] = typeString(a)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case KFn:
		parts := make([]string, len(t.Args))
		for i, a := range t.Args {
			parts[i] = typeString(a)
		}
		s := "fn(" + strings.Join(parts, ", ") + ")"
		if r := prune(t.Ret); r != nil && r.K != KUnit && r.K != KVoid {
			s += " -> " + typeString(r)
		}
		if t.Eff != 0 {
			s += " " + t.Eff.String()
		}
		return s
	case KNamed:
		s := t.Info.Name
		if len(t.Args) > 0 {
			parts := make([]string, len(t.Args))
			for i, a := range t.Args {
				parts[i] = typeString(a)
			}
			s += "<" + strings.Join(parts, ", ") + ">"
		}
		return s
	case KParam:
		return t.Param.Name
	case KVar:
		return "_"
	}
	return "?"
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
