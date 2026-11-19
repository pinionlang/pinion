package pinionrt

import (
	"strings"
)

// Kind classifies a static type descriptor (used by json.decode and debug).
type Kind int

const (
	KAny Kind = iota
	KInt
	KFloat
	KBool
	KStr
	KUnit
	KDec
	KBytes
	KInstant
	KDuration
	KList
	KMutList
	KMap
	KMutMap
	KSet
	KMutSet
	KOpt
	KTuple
	KResult
	KObj
	KEnum
	KParam
	KRange
	KFn
)

// Type is a runtime type descriptor.
type Type struct {
	K     Kind
	Args  []*Type
	Obj   *TypeInfo
	Enum  *EnumType
	Param int
}

var (
	TInt      = &Type{K: KInt}
	TFloat    = &Type{K: KFloat}
	TBool     = &Type{K: KBool}
	TStr      = &Type{K: KStr}
	TUnit     = &Type{K: KUnit}
	TDec      = &Type{K: KDec}
	TBytes    = &Type{K: KBytes}
	TInstant  = &Type{K: KInstant}
	TDuration = &Type{K: KDuration}
	TRange    = &Type{K: KRange}
	TAnyT     = &Type{K: KAny}
	TFn       = &Type{K: KFn}
)

// TK builds a composite type descriptor.
func TK(k Kind, args ...*Type) *Type { return &Type{K: k, Args: args} }

// TObj refers to a user data/class type with type arguments.
func TObj(t *TypeInfo, args ...*Type) *Type { return &Type{K: KObj, Obj: t, Args: args} }

// TEnum refers to an enum type with type arguments.
func TEnum(e *EnumType, args ...*Type) *Type { return &Type{K: KEnum, Enum: e, Args: args} }

// TParam refers to the i-th type parameter of the enclosing generic type.
func TParam(i int) *Type { return &Type{K: KParam, Param: i} }

func subst(t *Type, args []*Type) *Type {
	if t == nil {
		return TAnyT
	}
	switch t.K {
	case KParam:
		if t.Param < len(args) && args[t.Param] != nil {
			return args[t.Param]
		}
		return TAnyT
	case KInt, KFloat, KBool, KStr, KUnit, KDec, KBytes, KInstant, KDuration, KRange, KAny, KFn:
		return t
	}
	if len(t.Args) == 0 {
		return t
	}
	n := *t
	n.Args = make([]*Type, len(t.Args))
	for i, a := range t.Args {
		n.Args[i] = subst(a, args)
	}
	return &n
}

// FieldInfo describes a data/class field.
type FieldInfo struct {
	Name    string
	Type    *Type
	Pub     bool
	Mut     bool
	Default func() Value // nil: no default
}

// Method is a user method.
type Method struct {
	Name   string
	Params []string
	Fn     func(self Value, args []Value) Value
}

// TypeInfo describes a data or class type.
type TypeInfo struct {
	Name       string // display name
	Module     string
	Class      bool
	TypeParams []string
	Fields     []FieldInfo
	InitParams []string                 // nil if no init
	Init       func(args []Value) Value // receives InitParams in order
	Methods    map[string]*Method
	fieldIdx   map[string]int
}

// FieldIndex returns the index of a field or -1.
func (t *TypeInfo) FieldIndex(name string) int {
	if t.fieldIdx == nil || len(t.fieldIdx) != len(t.Fields) {
		t.fieldIdx = make(map[string]int, len(t.Fields))
		for i, f := range t.Fields {
			t.fieldIdx[f.Name] = i
		}
	}
	if i, ok := t.fieldIdx[name]; ok {
		return i
	}
	return -1
}

// AddMethod registers a method.
func (t *TypeInfo) AddMethod(name string, params []string, fn func(self Value, args []Value) Value) {
	if t.Methods == nil {
		t.Methods = map[string]*Method{}
	}
	t.Methods[name] = &Method{Name: name, Params: params, Fn: fn}
}

// VariantInfo is one enum variant.
type VariantInfo struct {
	T      *EnumType
	Name   string
	Index  int
	Fields []string // "" for positional payload
	Types  []*Type
	unit   *Enum
}

// Named reports whether the payload is named.
func (v *VariantInfo) Named() bool { return len(v.Fields) > 0 && v.Fields[0] != "" }

// EnumType describes an enum.
type EnumType struct {
	Name       string
	Module     string
	TypeParams []string
	Variants   []*VariantInfo
	byName     map[string]*VariantInfo
}

// NewEnum creates an enum type (variants added with AddVariant).
func NewEnum(name, module string) *EnumType {
	return &EnumType{Name: name, Module: module, byName: map[string]*VariantInfo{}}
}

// AddVariant adds a variant; fields are names ("" positional); types may be nil.
func (e *EnumType) AddVariant(name string, fields []string, types []*Type) *VariantInfo {
	v := &VariantInfo{T: e, Name: name, Index: len(e.Variants), Fields: fields, Types: types}
	if len(fields) == 0 {
		v.unit = &Enum{V: v}
	}
	e.Variants = append(e.Variants, v)
	if e.byName == nil {
		e.byName = map[string]*VariantInfo{}
	}
	e.byName[name] = v
	return v
}

// Variant looks a variant up by name.
func (e *EnumType) Variant(name string) *VariantInfo {
	v := e.byName[name]
	if v == nil {
		panicInternal("enum %s has no variant %s", e.Name, name)
	}
	return v
}

// Unit returns the value of a payload-less variant.
func (v *VariantInfo) Unit() Value {
	if v.unit == nil {
		v.unit = &Enum{V: v}
	}
	return v.unit
}

// MkEnum builds a variant value from (possibly named) args.
func MkEnum(v *VariantInfo, names []string, args []Value) Value {
	if len(v.Fields) == 0 {
		return v.Unit()
	}
	return &Enum{V: v, F: Reorder("variant "+v.T.Name+"."+v.Name, v.Fields, names, args, nil)}
}

// VariantFunc returns a variant constructor as a function value.
func VariantFunc(v *VariantInfo) Value {
	if len(v.Fields) == 0 {
		return v.Unit()
	}
	return MkFunc(len(v.Fields), v.T.Name+"."+v.Name, func(a []Value) Value {
		return &Enum{V: v, F: append([]Value(nil), a...)}
	})
}

// IsVariant reports whether x is the named enum variant (by name).
func IsVariant(x Value, name string) bool {
	e, ok := x.(*Enum)
	return ok && e.V.Name == name
}

// Payload returns the i-th payload value of an enum value.
func Payload(x Value, i int) Value {
	e := x.(*Enum)
	if i >= len(e.F) {
		panicInternal("variant %s has no payload %d", e.V.Name, i)
	}
	return e.F[i]
}

// Reorder maps call arguments (positional, then named in any order) onto
// params. missing(i) supplies a value for an omitted parameter (nil -> error).
func Reorder(what string, params []string, names []string, args []Value, missing func(i int) (Value, bool)) []Value {
	out := make([]Value, len(params))
	set := make([]bool, len(params))
	for i, a := range args {
		name := ""
		if i < len(names) {
			name = names[i]
		}
		idx := -1
		if name == "" {
			idx = i
		} else {
			for j, p := range params {
				if p == name {
					idx = j
					break
				}
			}
		}
		if idx < 0 || idx >= len(params) {
			if name == "" {
				panicInternal("%s: too many arguments", what)
			}
			panicInternal("%s: unknown argument %q", what, name)
		}
		out[idx] = a
		set[idx] = true
	}
	for i := range params {
		if !set[i] {
			if missing != nil {
				if v, ok := missing(i); ok {
					out[i] = v
					continue
				}
			}
			panicInternal("%s: missing argument %q", what, params[i])
		}
	}
	return out
}

func (t *TypeInfo) fieldNames() []string {
	ns := make([]string, len(t.Fields))
	for i, f := range t.Fields {
		ns[i] = f.Name
	}
	return ns
}

func (t *TypeInfo) defaultFor(name string) (Value, bool) {
	i := t.FieldIndex(name)
	if i >= 0 && t.Fields[i].Default != nil {
		return t.Fields[i].Default(), true
	}
	return nil, false
}

// Construct implements the automatic constructor T(...): args are already
// evaluated in written order; then omitted fields get defaults in
// declaration order; then init runs.
func Construct(t *TypeInfo, names []string, args []Value) Value {
	if t.Init != nil {
		vals := Reorder(t.Name, t.InitParams, names, args, func(i int) (Value, bool) {
			return t.defaultFor(t.InitParams[i])
		})
		return t.Init(vals)
	}
	vals := Reorder(t.Name, t.fieldNames(), names, args, func(i int) (Value, bool) {
		if d := t.Fields[i].Default; d != nil {
			return d(), true
		}
		return nil, false
	})
	return &Object{T: t, F: vals}
}

// SelfLit implements `Self { f: v, ... }` inside init.
func SelfLit(t *TypeInfo, names []string, args []Value) Value {
	vals := Reorder("Self", t.fieldNames(), names, args, func(i int) (Value, bool) {
		if d := t.Fields[i].Default; d != nil {
			return d(), true
		}
		return nil, false
	})
	return &Object{T: t, F: vals}
}

// With implements data `v.with(f: x, ...)`.
func With(v Value, names []string, args []Value) Value {
	o, ok := v.(*Object)
	if !ok || o.T.Class {
		panicInternal("with on non-data value %s", typeName(v))
	}
	vals := append([]Value(nil), o.F...)
	for i, n := range names {
		j := o.T.FieldIndex(n)
		if j < 0 {
			panicInternal("%s has no field %s", o.T.Name, n)
		}
		vals[j] = args[i]
	}
	if o.T.Init != nil {
		iv := make([]Value, len(o.T.InitParams))
		for i, p := range o.T.InitParams {
			j := o.T.FieldIndex(p)
			if j < 0 {
				panicInternal("init parameter %s is not a field", p)
			}
			iv[i] = vals[j]
		}
		return o.T.Init(iv)
	}
	return &Object{T: o.T, F: vals}
}

// GetField reads x.name for objects, enum payloads and tuples.
func GetField(x Value, name string) Value {
	switch o := x.(type) {
	case *Object:
		i := o.T.FieldIndex(name)
		if i < 0 {
			panicInternal("%s has no field %s", o.T.Name, name)
		}
		return o.F[i]
	case *Enum:
		for i, f := range o.V.Fields {
			if f == name {
				return o.F[i]
			}
		}
	case *Tuple:
		var idx int
		if _, err := parseIndex(name, &idx); err == nil && idx < len(o.Elems) {
			return o.Elems[idx]
		}
	}
	panicInternal("value of type %s has no field %s", typeName(x), name)
	return nil
}

func parseIndex(s string, out *int) (int, error) {
	n := 0
	if s == "" {
		return 0, errBad
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errBad
		}
		n = n*10 + int(c-'0')
	}
	*out = n
	return n, nil
}

type badErr struct{}

func (badErr) Error() string { return "bad" }

var errBad = badErr{}

// SetField assigns self.name = v.
func SetField(x Value, name string, v Value) {
	o, ok := x.(*Object)
	if !ok {
		panicInternal("cannot assign field %s on %s", name, typeName(x))
	}
	i := o.T.FieldIndex(name)
	if i < 0 {
		panicInternal("%s has no field %s", o.T.Name, name)
	}
	o.F[i] = v
}

// ---------- standard types ----------

var (
	RoundEnum         = NewEnum("Round", "")
	FsErrorEnum       = NewEnum("FsError", "")
	JsonErrorKindEnum = NewEnum("JsonErrorKind", "")
	ParseErrorEnum    = NewEnum("ParseError", "")
	JsonErrorType     = &TypeInfo{Name: "JsonError"}
	Utf8ErrorType     = &TypeInfo{Name: "Utf8Error"}
)

// BuiltinEnums are the prelude enums, by name.
var BuiltinEnums = map[string]*EnumType{}

// BuiltinTypes are the prelude data types, by name.
var BuiltinTypes = map[string]*TypeInfo{}

func init() {
	for _, n := range []string{"HalfEven", "HalfUp", "Down", "Floor", "Ceil"} {
		RoundEnum.AddVariant(n, nil, nil)
	}
	for _, n := range []string{"NotFound", "PermissionDenied", "IsDirectory", "InvalidUtf8"} {
		FsErrorEnum.AddVariant(n, []string{"path"}, []*Type{TStr})
	}
	FsErrorEnum.AddVariant("Other", []string{"path", "msg"}, []*Type{TStr, TStr})
	for _, n := range []string{"Syntax", "UnknownField", "MissingField", "DuplicateKey", "DuplicateElement", "TypeMismatch", "OutOfRange"} {
		JsonErrorKindEnum.AddVariant(n, nil, nil)
	}
	ParseErrorEnum.AddVariant("InvalidFormat", []string{"input"}, []*Type{TStr})
	ParseErrorEnum.AddVariant("OutOfRange", []string{"input"}, []*Type{TStr})
	JsonErrorType.Fields = []FieldInfo{
		{Name: "kind", Type: TEnum(JsonErrorKindEnum), Pub: true},
		{Name: "path", Type: TStr, Pub: true},
		{Name: "msg", Type: TStr, Pub: true},
	}
	JsonErrorType.AddMethod("code", nil, func(self Value, _ []Value) Value {
		k := GetField(self, "kind").(*Enum)
		return snake(k.V.Name)
	})
	Utf8ErrorType.Fields = []FieldInfo{{Name: "byte_offset", Type: TInt, Pub: true}}
	for _, e := range []*EnumType{RoundEnum, FsErrorEnum, JsonErrorKindEnum, ParseErrorEnum} {
		BuiltinEnums[e.Name] = e
	}
	BuiltinTypes["JsonError"] = JsonErrorType
	BuiltinTypes["Utf8Error"] = Utf8ErrorType
}

func snake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r + ('a' - 'A'))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// BuiltinVariant returns a prelude enum variant (e.g. "Round", "HalfEven").
func BuiltinVariant(enum, name string) *VariantInfo { return BuiltinEnums[enum].Variant(name) }

func fsErr(variant, path string, msg ...string) Value {
	v := FsErrorEnum.Variant(variant)
	f := []Value{path}
	if variant == "Other" {
		m := ""
		if len(msg) > 0 {
			m = msg[0]
		}
		f = append(f, m)
	}
	return &Enum{V: v, F: f}
}

func parseErr(variant, input string) Value {
	return &Enum{V: ParseErrorEnum.Variant(variant), F: []Value{input}}
}
