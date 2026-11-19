// Package pinionrt is the runtime library linked into every compiled Pinion
// program. Generated code manipulates dynamically typed Values.
//
// Value representation:
//
//	Int      int64          Float   float64      Bool  bool
//	Str      string         Bytes   Bytes        Unit  Unit
//	Dec      *Dec           List / MutList       *List
//	Map / MutMap  *Map      Set / MutSet         *Set
//	tuple    *Tuple         Range   Range        Result *Result
//	T?       None, or the value itself (Some(x) == x; T?? is forbidden)
//	data / class  *Object   enum    *Enum        function *Func
//	Instant  Instant        Duration Duration    effect handler *Handler
package pinionrt

import (
	"fmt"
	"math"
	"sync/atomic"
)

// Value is any Pinion runtime value.
type Value = any

// Unit is the value `()`.
type Unit struct{}

// UnitV is the unit value.
var UnitV Value = Unit{}

// NoneType is the type of None.
type NoneType struct{}

// None is the absent optional value.
var None Value = NoneType{}

// Bytes is an immutable byte string.
type Bytes string

// Duration is a signed nanosecond count.
type Duration int64

// Instant is a UTC instant in Unix nanoseconds.
type Instant struct{ NS int64 }

// Tuple is (a, b, ...).
type Tuple struct{ Elems []Value }

// Result is Ok(v) / Err(v).
type Result struct {
	IsOk bool
	V    Value
}

// Func is a callable value.
type Func struct {
	Arity int // -1: variadic / unchecked
	Name  string
	Fn    func(args []Value) Value
}

// Handler is the value produced by fs.fake(...) etc.
type Handler struct {
	Effect string
	Impl   any
}

var objCounter atomic.Uint64

func nextID() uint64 { return objCounter.Add(1) }

// Object is an instance of a user `data` or `class`.
type Object struct {
	T  *TypeInfo
	F  []Value
	id uint64
}

// ID returns a stable identity number (used for class hashing and cycles).
func (o *Object) ID() uint64 {
	if o.id == 0 {
		o.id = nextID()
	}
	return o.id
}

// Enum is an enum value.
type Enum struct {
	V *VariantInfo
	F []Value
}

// Ok builds Ok(v).
func Ok(v Value) Value { return &Result{IsOk: true, V: v} }

// Err builds Err(v).
func Err(v Value) Value { return &Result{IsOk: false, V: v} }

// Some builds Some(v) (identity).
func Some(v Value) Value { return v }

// Tup builds a tuple.
func Tup(xs ...Value) Value {
	if len(xs) == 0 {
		return UnitV
	}
	return &Tuple{Elems: xs}
}

// IsNone reports whether v is None.
func IsNone(v Value) bool { _, ok := v.(NoneType); return ok }

// Truthy converts a Bool value to a Go bool.
func Truthy(v Value) bool {
	b, ok := v.(bool)
	if !ok {
		panicInternal("expected Bool, got %s", typeName(v))
	}
	return b
}

// MkFunc wraps a Go function as a Pinion function value.
func MkFunc(arity int, name string, fn func(args []Value) Value) Value {
	return &Func{Arity: arity, Name: name, Fn: fn}
}

// Call calls a function value.
func Call(f Value, args ...Value) Value {
	fn, ok := f.(*Func)
	if !ok {
		panicInternal("value of type %s is not callable", typeName(f))
	}
	if fn.Arity >= 0 && fn.Arity != len(args) {
		panicInternal("function %s expects %d arguments, got %d", fn.Name, fn.Arity, len(args))
	}
	return fn.Fn(args)
}

func typeName(v Value) string {
	switch x := v.(type) {
	case int64:
		return "Int"
	case float64:
		return "Float"
	case bool:
		return "Bool"
	case string:
		return "Str"
	case Bytes:
		return "Bytes"
	case Unit:
		return "()"
	case NoneType:
		return "None"
	case *Dec:
		return "Dec"
	case *List:
		if x.Mut {
			return "MutList"
		}
		return "List"
	case *Map:
		if x.Mut {
			return "MutMap"
		}
		return "Map"
	case *Set:
		if x.Mut {
			return "MutSet"
		}
		return "Set"
	case *Tuple:
		return "tuple"
	case Range:
		return "Range"
	case *Result:
		return "Result"
	case *Object:
		return x.T.Name
	case *Enum:
		return x.V.T.Name
	case *Func:
		return "fn"
	case Instant:
		return "Instant"
	case Duration:
		return "Duration"
	case *Handler:
		return "handler"
	case nil:
		return "<nil>"
	}
	return fmt.Sprintf("%T", v)
}

// ---------- equality, hashing, ordering ----------

// Equal is structural equality (==).
func Equal(a, b Value) bool {
	switch x := a.(type) {
	case int64:
		y, ok := b.(int64)
		return ok && x == y
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	case Bytes:
		y, ok := b.(Bytes)
		return ok && x == y
	case Unit:
		_, ok := b.(Unit)
		return ok
	case NoneType:
		_, ok := b.(NoneType)
		return ok
	case *Dec:
		y, ok := b.(*Dec)
		return ok && x.Cmp(y) == 0
	case Instant:
		y, ok := b.(Instant)
		return ok && x == y
	case Duration:
		y, ok := b.(Duration)
		return ok && x == y
	case Range:
		y, ok := b.(Range)
		return ok && rangeEqual(x, y)
	case *List:
		y, ok := b.(*List)
		if !ok || len(x.Items) != len(y.Items) {
			return false
		}
		for i := range x.Items {
			if !Equal(x.Items[i], y.Items[i]) {
				return false
			}
		}
		return true
	case *Tuple:
		y, ok := b.(*Tuple)
		if !ok || len(x.Elems) != len(y.Elems) {
			return false
		}
		for i := range x.Elems {
			if !Equal(x.Elems[i], y.Elems[i]) {
				return false
			}
		}
		return true
	case *Map:
		y, ok := b.(*Map)
		if !ok || x.Len() != y.Len() {
			return false
		}
		for i, k := range x.keys.items {
			j := y.keys.find(k)
			if j < 0 || !Equal(x.vals[i], y.vals[j]) {
				return false
			}
		}
		return true
	case *Set:
		y, ok := b.(*Set)
		if !ok || x.Len() != y.Len() {
			return false
		}
		for _, k := range x.s.items {
			if y.s.find(k) < 0 {
				return false
			}
		}
		return true
	case *Result:
		y, ok := b.(*Result)
		return ok && x.IsOk == y.IsOk && Equal(x.V, y.V)
	case *Enum:
		y, ok := b.(*Enum)
		if !ok || x.V != y.V {
			return false
		}
		for i := range x.F {
			if !Equal(x.F[i], y.F[i]) {
				return false
			}
		}
		return true
	case *Object:
		y, ok := b.(*Object)
		if !ok {
			return false
		}
		if x == y {
			return true
		}
		if x.T != y.T || x.T.Class {
			return false
		}
		for i := range x.F {
			if !Equal(x.F[i], y.F[i]) {
				return false
			}
		}
		return true
	case *Func:
		return a == b
	}
	return a == b
}

func mix(h uint64) uint64 {
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	h *= 0xc4ceb9fe1a85ec53
	h ^= h >> 33
	return h
}

func hashStr(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

func combine(h, x uint64) uint64 { return mix(h*31 + x + 0x9e3779b97f4a7c15) }

// Hash is a structural hash consistent with Equal.
func Hash(v Value) uint64 {
	switch x := v.(type) {
	case int64:
		return mix(uint64(x))
	case float64:
		if x == 0 {
			x = 0
		}
		return mix(math.Float64bits(x) ^ 0x1234)
	case bool:
		if x {
			return 0xabc1
		}
		return 0xabc0
	case string:
		return hashStr(x)
	case Bytes:
		return hashStr(string(x)) ^ 0xb7
	case Unit:
		return 0x77
	case NoneType:
		return 0x55
	case *Dec:
		n := x.normalized()
		return combine(hashStr(n.u.String()), uint64(n.scale))
	case Instant:
		return mix(uint64(x.NS) ^ 0x11)
	case Duration:
		return mix(uint64(x) ^ 0x22)
	case Range:
		n := x.Len()
		if n == 0 {
			return 0x99
		}
		return combine(combine(mix(uint64(x.Start)), uint64(n)), uint64(x.Step))
	case *List:
		h := uint64(0x31)
		for _, e := range x.Items {
			h = combine(h, Hash(e))
		}
		return h
	case *Tuple:
		h := uint64(0x41)
		for _, e := range x.Elems {
			h = combine(h, Hash(e))
		}
		return h
	case *Map:
		var h uint64
		for i, k := range x.keys.items {
			h += mix(combine(Hash(k), Hash(x.vals[i])))
		}
		return mix(h ^ 0x51)
	case *Set:
		var h uint64
		for _, k := range x.s.items {
			h += mix(Hash(k))
		}
		return mix(h ^ 0x61)
	case *Result:
		h := Hash(x.V)
		if x.IsOk {
			return combine(h, 1)
		}
		return combine(h, 2)
	case *Enum:
		h := hashStr(x.V.T.Name + "." + x.V.Name)
		for _, e := range x.F {
			h = combine(h, Hash(e))
		}
		return h
	case *Object:
		if x.T.Class {
			return mix(x.ID())
		}
		h := hashStr(x.T.Name)
		for _, e := range x.F {
			h = combine(h, Hash(e))
		}
		return h
	case *Func:
		return 0xf0
	}
	return 0
}

// Compare orders two values of the same Ord type: -1, 0, 1.
func Compare(a, b Value) int {
	switch x := a.(type) {
	case int64:
		y := b.(int64)
		return cmp3(x < y, x > y)
	case float64:
		y := b.(float64)
		return cmp3(x < y, x > y)
	case bool:
		y := b.(bool)
		return cmp3(!x && y, x && !y)
	case string:
		y := b.(string)
		return cmp3(x < y, x > y) // UTF-8 byte order == code point order
	case Bytes:
		y := b.(Bytes)
		return cmp3(x < y, x > y)
	case *Dec:
		return x.Cmp(b.(*Dec))
	case Instant:
		y := b.(Instant)
		return cmp3(x.NS < y.NS, x.NS > y.NS)
	case Duration:
		y := b.(Duration)
		return cmp3(x < y, x > y)
	case *List:
		y := b.(*List)
		for i := 0; i < len(x.Items) && i < len(y.Items); i++ {
			if c := Compare(x.Items[i], y.Items[i]); c != 0 {
				return c
			}
		}
		return cmp3(len(x.Items) < len(y.Items), len(x.Items) > len(y.Items))
	case *Tuple:
		y := b.(*Tuple)
		for i := 0; i < len(x.Elems) && i < len(y.Elems); i++ {
			if c := Compare(x.Elems[i], y.Elems[i]); c != 0 {
				return c
			}
		}
		return cmp3(len(x.Elems) < len(y.Elems), len(x.Elems) > len(y.Elems))
	case NoneType:
		if IsNone(b) {
			return 0
		}
		return -1
	}
	if IsNone(b) {
		return 1
	}
	panicInternal("values of type %s are not ordered", typeName(a))
	return 0
}

func cmp3(lt, gt bool) int {
	if lt {
		return -1
	}
	if gt {
		return 1
	}
	return 0
}

// Eq / Ne / Lt / ... are the comparison operators.
func Eq(a, b Value) Value { return Equal(a, b) }
func Ne(a, b Value) Value { return !Equal(a, b) }
func Lt(a, b Value) Value { return Compare(a, b) < 0 }
func Le(a, b Value) Value { return Compare(a, b) <= 0 }
func Gt(a, b Value) Value { return Compare(a, b) > 0 }
func Ge(a, b Value) Value { return Compare(a, b) >= 0 }

// Not is logical negation.
func Not(a Value) Value { return !Truthy(a) }
