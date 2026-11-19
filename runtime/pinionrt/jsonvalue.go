package pinionrt

import (
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
	}
	return noMethod("Json", name)
}
