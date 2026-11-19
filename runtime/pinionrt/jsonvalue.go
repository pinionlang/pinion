package pinionrt

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ---------- std/json dynamic value: enum json.Json ----------

// JsonEnum is `enum Json { Null, Bool(Bool), Int(Int), Float(Float), Str(Str),
// Arr(List<Json>), Obj(Map<Str, Json>) }` from std/json.
var JsonEnum = NewEnum("Json", "std/json")

var (
	jvNull, jvBool, jvInt, jvFloat, jvStr, jvArr, jvObj *VariantInfo
)

func init() {
	self := TEnum(JsonEnum)
	jvNull = JsonEnum.AddVariant("Null", nil, nil)
	pos := func(name string, t *Type) *VariantInfo { return JsonEnum.AddVariant(name, []string{""}, []*Type{t}) }
	jvBool = pos("Bool", TBool)
	jvInt = pos("Int", TInt)
	jvFloat = pos("Float", TFloat)
	jvStr = pos("Str", TStr)
	jvArr = pos("Arr", TK(KList, self))
	jvObj = pos("Obj", TK(KMap, TStr, self))
	BuiltinEnums["Json"] = JsonEnum
	stdFns["json.parse"] = stdFn{[]string{"s"}, func(a []Value) Value { return JSONParse(a[0]) }}
	for _, t := range []*TypeInfo{HTTPRequestType, HTTPResponseType} {
		t.AddMethod("json_value", nil, func(self Value, _ []Value) Value { return JSONParse(GetField(self, "body")) })
	}
	HTTPRequestType.AddMethod("json_obj", nil, func(self Value, _ []Value) Value {
		r := JSONParse(GetField(self, "body")).(*Result)
		if !r.IsOk {
			return Err("invalid json")
		}
		if r.V.(*Enum).V != jvObj {
			return Err(notObject)
		}
		return r
	})
}

// JSONParse is json.parse(s): strict RFC 8259 into a Json value.
func JSONParse(s Value) Value {
	p := &jparser{s: s.(string)}
	n := p.value("$", 0)
	if p.err == nil {
		p.ws()
		if p.i != len(p.s) {
			p.fail("$", "trailing characters after JSON value")
		}
	}
	if p.err != nil {
		return Err(mkJSONError(p.err))
	}
	v, err := nodeToJSON(n, "$")
	if err != nil {
		return Err(mkJSONError(err))
	}
	return Ok(v)
}

func nodeToJSON(n *jnode, path string) (Value, *jsonErr) {
	one := func(v *VariantInfo, x Value) Value { return &Enum{V: v, F: []Value{x}} }
	switch n.k {
	case jNull:
		return jvNull.Unit(), nil
	case jBool:
		return one(jvBool, n.b), nil
	case jNum:
		if !strings.ContainsAny(n.s, ".eE") {
			if i, err := strconv.ParseInt(n.s, 10, 64); err == nil {
				return one(jvInt, i), nil
			}
		}
		f, err := strconv.ParseFloat(n.s, 64)
		if err != nil || math.IsInf(f, 0) {
			return nil, &jsonErr{"OutOfRange", path, "number out of range: " + n.s}
		}
		return one(jvFloat, f), nil
	case jStr:
		return one(jvStr, n.s), nil
	case jArr:
		out := make([]Value, len(n.arr))
		for i, e := range n.arr {
			v, err := nodeToJSON(e, path+"["+strconv.Itoa(i)+"]")
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return one(jvArr, NewList(out)), nil
	}
	m := &Map{}
	for i, k := range n.keys {
		v, err := nodeToJSON(n.vals[i], path+"."+k)
		if err != nil {
			return nil, err
		}
		m.put(k, v)
	}
	return one(jvObj, m), nil
}

// encodeJSONValue writes a Json value as plain (untagged) JSON.
func encodeJSONValue(b *strings.Builder, e *Enum) {
	switch e.V {
	case jvNull:
		b.WriteString("null")
	case jvFloat:
		s := floatJSON(e.F[0].(float64))
		if !strings.ContainsAny(s, ".eE") {
			s += ".0" // keep Float distinct from Int across a round trip
		}
		b.WriteString(s)
	case jvArr, jvObj, jvBool, jvInt, jvStr:
		encodeJSON(b, e.F[0])
	}
}

// jsonLifted are the Json methods that also accept a Json? receiver (None -> None).
var jsonLifted = map[string]bool{"get": true, "at": true, "as_str": true, "as_int": true, "as_float": true,
	"as_bool": true, "as_list": true, "as_obj": true}

func jsonMethod(e *Enum, name string, a []Value) Value {
	payload := func(v *VariantInfo) Value {
		if e.V == v {
			return e.F[0]
		}
		return None
	}
	switch name {
	case "get":
		if e.V == jvObj {
			if v, ok := e.F[0].(*Map).Get(a[0]); ok {
				return v
			}
		}
		return None
	case "has":
		if e.V == jvObj {
			_, ok := e.F[0].(*Map).Get(a[0])
			return ok
		}
		return false
	case "at":
		if e.V == jvArr {
			xs := e.F[0].(*List).Items
			if i := a[0].(int64); i >= 0 && i < int64(len(xs)) {
				return xs[i]
			}
		}
		return None
	case "keys":
		if e.V == jvObj {
			return NewList(append([]Value(nil), e.F[0].(*Map).keys.items...))
		}
		return NewList(nil)
	case "as_str":
		return payload(jvStr)
	case "as_int":
		return payload(jvInt)
	case "as_float":
		if e.V == jvInt {
			return float64(e.F[0].(int64))
		}
		return payload(jvFloat)
	case "as_bool":
		return payload(jvBool)
	case "as_list":
		return payload(jvArr)
	case "as_obj":
		return payload(jvObj)
	case "is_null":
		return e.V == jvNull
	case "kind":
		return strings.ToLower(e.V.Name)
	case "only":
		if e.V != jvObj {
			return Err(notObject)
		}
		allowed := map[string]bool{}
		for _, k := range a[0].(*List).Items {
			allowed[k.(string)] = true
		}
		for _, k := range e.F[0].(*Map).keys.items {
			if !allowed[k.(string)] {
				return Err("unknown field: " + k.(string))
			}
		}
		return Ok(UnitV)
	case "req_str", "opt_str", "req_int", "opt_int", "req_float", "opt_float", "req_bool", "opt_bool", "str_list":
		return jsonField(e, name, a)
	}
	return noMethod("Json", name)
}

const notObject = "body must be a JSON object"

func arg(a []Value, i int) Value {
	if i < len(a) {
		return a[i]
	}
	return nil
}

// jsonField implements the req_*/opt_*/str_list validation helpers.
func jsonField(e *Enum, name string, a []Value) Value {
	if e.V != jvObj {
		return Err(notObject)
	}
	k := a[0].(string)
	v, present := e.F[0].(*Map).Get(k)
	opt := !strings.HasPrefix(name, "req_")
	if !present {
		if opt {
			return Ok(None)
		}
		return Err(k + ": required")
	}
	wrap := func(x Value) Value {
		if opt {
			return Ok(Some(x))
		}
		return Ok(x)
	}
	fail := func(what string) Value { return Err(k + ": must be " + what) }
	j := v.(*Enum)
	lo, hi := arg(a, 1), arg(a, 2)
	switch strings.TrimPrefix(strings.TrimPrefix(name, "req_"), "opt_") {
	case "str":
		if j.V != jvStr {
			return fail("a string")
		}
		s := j.F[0].(string)
		n := int64(len([]rune(s)))
		if lo != nil && n < lo.(int64) {
			if lo.(int64) == 1 {
				return Err(k + ": must not be empty")
			}
			return Err(fmt.Sprintf("%s: must have at least %d characters", k, lo.(int64)))
		}
		if hi != nil && n > hi.(int64) {
			return Err(fmt.Sprintf("%s: must have at most %d characters", k, hi.(int64)))
		}
		return wrap(s)
	case "int":
		if j.V != jvInt {
			return fail("an integer")
		}
		n := j.F[0].(int64)
		if lo != nil && n < lo.(int64) {
			return Err(fmt.Sprintf("%s: must be >= %d", k, lo.(int64)))
		}
		if hi != nil && n > hi.(int64) {
			return Err(fmt.Sprintf("%s: must be <= %d", k, hi.(int64)))
		}
		return wrap(n)
	case "float":
		var f float64
		switch j.V {
		case jvInt:
			f = float64(j.F[0].(int64))
		case jvFloat:
			f = j.F[0].(float64)
		default:
			return fail("a number")
		}
		if lo != nil && f < lo.(float64) {
			return Err(k + ": must be >= " + Display(lo))
		}
		if hi != nil && f > hi.(float64) {
			return Err(k + ": must be <= " + Display(hi))
		}
		return wrap(f)
	case "bool":
		if j.V != jvBool {
			return fail("a boolean")
		}
		return wrap(j.F[0])
	}
	// str_list
	if j.V != jvArr {
		return fail("a list of strings")
	}
	items := j.F[0].(*List).Items
	out := make([]Value, len(items))
	for i, x := range items {
		xe := x.(*Enum)
		if xe.V != jvStr {
			return fail("a list of strings")
		}
		out[i] = xe.F[0]
	}
	return Ok(Some(NewList(out)))
}
