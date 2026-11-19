package pinionrt

import (
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ---------- encode ----------

// JSONEncode is json.encode(v).
func JSONEncode(v Value) Value {
	var b strings.Builder
	encodeJSON(&b, v)
	return b.String()
}

func writeJSONString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}

func encodeJSON(b *strings.Builder, v Value) {
	switch x := v.(type) {
	case NoneType:
		b.WriteString("null")
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case float64:
		b.WriteString(floatJSON(x))
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case string:
		writeJSONString(b, x)
	case *Dec:
		writeJSONString(b, x.String())
	case Bytes:
		writeJSONString(b, base64.StdEncoding.EncodeToString([]byte(x)))
	case Instant:
		writeJSONString(b, formatInstant(x))
	case Duration:
		b.WriteString(strconv.FormatInt(int64(x), 10))
	case *List:
		encodeArr(b, x.Items)
	case *Set:
		encodeArr(b, x.s.items)
	case *Map:
		b.WriteByte('{')
		for i, k := range x.keys.items {
			if i > 0 {
				b.WriteByte(',')
			}
			ks, ok := k.(string)
			if !ok {
				panicInternal("json.encode: map key must be Str")
			}
			writeJSONString(b, ks)
			b.WriteByte(':')
			encodeJSON(b, x.vals[i])
		}
		b.WriteByte('}')
	case *Object:
		b.WriteByte('{')
		first := true
		for i, f := range x.F {
			if IsNone(f) {
				continue
			}
			if !first {
				b.WriteByte(',')
			}
			first = false
			writeJSONString(b, x.T.Fields[i].Name)
			b.WriteByte(':')
			encodeJSON(b, f)
		}
		b.WriteByte('}')
	case *Enum:
		b.WriteString(`{"tag":`)
		writeJSONString(b, x.V.Name)
		if len(x.F) > 0 {
			b.WriteString(`,"value":`)
			if x.V.Named() {
				b.WriteByte('{')
				first := true
				for i, f := range x.F {
					if IsNone(f) {
						continue
					}
					if !first {
						b.WriteByte(',')
					}
					first = false
					writeJSONString(b, x.V.Fields[i])
					b.WriteByte(':')
					encodeJSON(b, f)
				}
				b.WriteByte('}')
			} else {
				encodeArr(b, x.F)
			}
		}
		b.WriteByte('}')
	default:
		panicInternal("json.encode: %s is not encodable", typeName(v))
	}
}

func encodeArr(b *strings.Builder, xs []Value) {
	b.WriteByte('[')
	for i, e := range xs {
		if i > 0 {
			b.WriteByte(',')
		}
		encodeJSON(b, e)
	}
	b.WriteByte(']')
}

// ---------- strict JSON parser ----------

type jkind int

const (
	jNull jkind = iota
	jBool
	jNum
	jStr
	jArr
	jObj
)

type jnode struct {
	k    jkind
	b    bool
	s    string // number text or string
	arr  []*jnode
	keys []string
	vals []*jnode
}

type jsonErr struct {
	kind string
	path string
	msg  string
}

type jparser struct {
	s   string
	i   int
	err *jsonErr
}

func (p *jparser) fail(path, msg string) {
	if p.err == nil {
		p.err = &jsonErr{"Syntax", path, msg}
	}
}

func (p *jparser) ws() {
	for p.i < len(p.s) && (p.s[p.i] == ' ' || p.s[p.i] == '\t' || p.s[p.i] == '\n' || p.s[p.i] == '\r') {
		p.i++
	}
}

func (p *jparser) value(path string, depth int) *jnode {
	if p.err != nil {
		return nil
	}
	if depth > 1000 {
		p.fail(path, "nesting too deep")
		return nil
	}
	p.ws()
	if p.i >= len(p.s) {
		p.fail(path, "unexpected end of input")
		return nil
	}
	c := p.s[p.i]
	switch {
	case c == '{':
		p.i++
		n := &jnode{k: jObj}
		seen := map[string]bool{}
		p.ws()
		if p.i < len(p.s) && p.s[p.i] == '}' {
			p.i++
			return n
		}
		for {
			p.ws()
			if p.i >= len(p.s) || p.s[p.i] != '"' {
				p.fail(path, "expected object key")
				return nil
			}
			k, ok := p.str()
			if !ok {
				p.fail(path, "invalid string")
				return nil
			}
			kp := path + "." + k
			if seen[k] {
				p.err = &jsonErr{"DuplicateKey", kp, "duplicate key " + strconv.Quote(k)}
				return nil
			}
			seen[k] = true
			p.ws()
			if p.i >= len(p.s) || p.s[p.i] != ':' {
				p.fail(kp, "expected ':'")
				return nil
			}
			p.i++
			v := p.value(kp, depth+1)
			if p.err != nil {
				return nil
			}
			n.keys = append(n.keys, k)
			n.vals = append(n.vals, v)
			p.ws()
			if p.i < len(p.s) && p.s[p.i] == ',' {
				p.i++
				continue
			}
			if p.i < len(p.s) && p.s[p.i] == '}' {
				p.i++
				return n
			}
			p.fail(path, "expected ',' or '}'")
			return nil
		}
	case c == '[':
		p.i++
		n := &jnode{k: jArr}
		p.ws()
		if p.i < len(p.s) && p.s[p.i] == ']' {
			p.i++
			return n
		}
		for {
			v := p.value(fmt.Sprintf("%s[%d]", path, len(n.arr)), depth+1)
			if p.err != nil {
				return nil
			}
			n.arr = append(n.arr, v)
			p.ws()
			if p.i < len(p.s) && p.s[p.i] == ',' {
				p.i++
				continue
			}
			if p.i < len(p.s) && p.s[p.i] == ']' {
				p.i++
				return n
			}
			p.fail(path, "expected ',' or ']'")
			return nil
		}
	case c == '"':
		s, ok := p.str()
		if !ok {
			p.fail(path, "invalid string")
			return nil
		}
		return &jnode{k: jStr, s: s}
	case strings.HasPrefix(p.s[p.i:], "true"):
		p.i += 4
		return &jnode{k: jBool, b: true}
	case strings.HasPrefix(p.s[p.i:], "false"):
		p.i += 5
		return &jnode{k: jBool}
	case strings.HasPrefix(p.s[p.i:], "null"):
		p.i += 4
		return &jnode{k: jNull}
	case c == '-' || (c >= '0' && c <= '9'):
		st := p.i
		if p.s[p.i] == '-' {
			p.i++
		}
		digits := func() int {
			n := 0
			for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
				p.i++
				n++
			}
			return n
		}
		ds := p.i
		if digits() == 0 || (p.s[ds] == '0' && p.i-ds > 1) {
			p.fail(path, "invalid number")
			return nil
		}
		if p.i < len(p.s) && p.s[p.i] == '.' {
			p.i++
			if digits() == 0 {
				p.fail(path, "invalid number")
				return nil
			}
		}
		if p.i < len(p.s) && (p.s[p.i] == 'e' || p.s[p.i] == 'E') {
			p.i++
			if p.i < len(p.s) && (p.s[p.i] == '+' || p.s[p.i] == '-') {
				p.i++
			}
			if digits() == 0 {
				p.fail(path, "invalid number")
				return nil
			}
		}
		return &jnode{k: jNum, s: p.s[st:p.i]}
	}
	p.fail(path, fmt.Sprintf("unexpected character %q", c))
	return nil
}

func (p *jparser) str() (string, bool) {
	p.i++ // opening quote
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c == '"':
			p.i++
			return b.String(), true
		case c == '\\':
			if p.i+1 >= len(p.s) {
				return "", false
			}
			e := p.s[p.i+1]
			p.i += 2
			switch e {
			case '"', '\\', '/':
				b.WriteByte(e)
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'u':
				if p.i+4 > len(p.s) {
					return "", false
				}
				n, err := strconv.ParseUint(p.s[p.i:p.i+4], 16, 32)
				if err != nil {
					return "", false
				}
				p.i += 4
				r := rune(n)
				if r >= 0xD800 && r < 0xDC00 && strings.HasPrefix(p.s[p.i:], `\u`) && p.i+6 <= len(p.s) {
					if m, err := strconv.ParseUint(p.s[p.i+2:p.i+6], 16, 32); err == nil && m >= 0xDC00 && m < 0xE000 {
						r = (r-0xD800)<<10 + (rune(m) - 0xDC00) + 0x10000
						p.i += 6
					}
				}
				b.WriteRune(r)
			default:
				return "", false
			}
		case c < 0x20:
			return "", false
		default:
			r, sz := utf8.DecodeRuneInString(p.s[p.i:])
			if r == utf8.RuneError && sz <= 1 {
				return "", false
			}
			b.WriteString(p.s[p.i : p.i+sz])
			p.i += sz
		}
	}
	return "", false
}

// ---------- decode ----------

func mkJSONError(e *jsonErr) Value {
	return &Object{T: JsonErrorType, F: []Value{JsonErrorKindEnum.Variant(e.kind).Unit(), e.path, e.msg}}
}

// JSONDecode is json.decode<T>(s).
func JSONDecode(t *Type, s Value) Value {
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
	v, err := decodeNode(t, n, "$")
	if err != nil {
		return Err(mkJSONError(err))
	}
	return Ok(v)
}

func kindName(n *jnode) string {
	return [...]string{"null", "boolean", "number", "string", "array", "object"}[n.k]
}

func mismatch(want string, n *jnode, path string) *jsonErr {
	return &jsonErr{"TypeMismatch", path, "expected " + want + ", found " + kindName(n)}
}

func decodeNode(t *Type, n *jnode, path string) (Value, *jsonErr) {
	switch t.K {
	case KOpt:
		if n.k == jNull {
			return None, nil
		}
		return decodeNode(t.Args[0], n, path)
	case KInt:
		if n.k != jNum {
			return nil, mismatch("integer", n, path)
		}
		if strings.ContainsAny(n.s, ".eE") {
			return nil, &jsonErr{"TypeMismatch", path, "expected integer, found " + n.s}
		}
		i, err := strconv.ParseInt(n.s, 10, 64)
		if err != nil {
			return nil, &jsonErr{"OutOfRange", path, "integer out of range: " + n.s}
		}
		return i, nil
	case KFloat:
		if n.k != jNum {
			return nil, mismatch("number", n, path)
		}
		f, err := strconv.ParseFloat(n.s, 64)
		if err != nil || math.IsInf(f, 0) {
			return nil, &jsonErr{"OutOfRange", path, "number out of range: " + n.s}
		}
		return f, nil
	case KBool:
		if n.k != jBool {
			return nil, mismatch("boolean", n, path)
		}
		return n.b, nil
	case KStr:
		if n.k != jStr {
			return nil, mismatch("string", n, path)
		}
		return n.s, nil
	case KDec:
		if n.k != jStr {
			return nil, mismatch("decimal string", n, path)
		}
		d, ok := ParseDec(n.s)
		if !ok {
			return nil, &jsonErr{"TypeMismatch", path, "invalid decimal " + strconv.Quote(n.s)}
		}
		return d, nil
	case KBytes:
		if n.k != jStr {
			return nil, mismatch("base64 string", n, path)
		}
		b, err := base64.StdEncoding.DecodeString(n.s)
		if err != nil {
			return nil, &jsonErr{"TypeMismatch", path, "invalid base64"}
		}
		return Bytes(b), nil
	case KInstant:
		if n.k != jStr {
			return nil, mismatch("RFC 3339 string", n, path)
		}
		tm, err := time.Parse(time.RFC3339Nano, n.s)
		if err != nil {
			return nil, &jsonErr{"TypeMismatch", path, "invalid RFC 3339 time"}
		}
		return Instant{NS: tm.UnixNano()}, nil
	case KDuration:
		v, err := decodeNode(TInt, n, path)
		if err != nil {
			return nil, err
		}
		return Duration(v.(int64)), nil
	case KList, KMutList:
		if n.k != jArr {
			return nil, mismatch("array", n, path)
		}
		out := make([]Value, len(n.arr))
		for i, e := range n.arr {
			v, err := decodeNode(elemT(t, 0), e, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return &List{Items: out, Mut: t.K == KMutList}, nil
	case KSet, KMutSet:
		if n.k != jArr {
			return nil, mismatch("array", n, path)
		}
		s := &Set{Mut: t.K == KMutSet}
		for i, e := range n.arr {
			p := fmt.Sprintf("%s[%d]", path, i)
			v, err := decodeNode(elemT(t, 0), e, p)
			if err != nil {
				return nil, err
			}
			if _, added := s.s.add(v); !added {
				return nil, &jsonErr{"DuplicateElement", p, "duplicate set element"}
			}
		}
		return s, nil
	case KMap, KMutMap:
		if n.k != jObj {
			return nil, mismatch("object", n, path)
		}
		m := &Map{Mut: t.K == KMutMap}
		for i, k := range n.keys {
			v, err := decodeNode(elemT(t, 1), n.vals[i], path+"."+k)
			if err != nil {
				return nil, err
			}
			m.put(k, v)
		}
		return m, nil
	case KObj:
		ti := t.Obj
		if n.k != jObj {
			return nil, mismatch("object", n, path)
		}
		vals := make([]Value, len(ti.Fields))
		set := make([]bool, len(ti.Fields))
		for i, k := range n.keys {
			fi := ti.FieldIndex(k)
			if fi < 0 {
				return nil, &jsonErr{"UnknownField", path + "." + k, "unknown field " + strconv.Quote(k)}
			}
			v, err := decodeNode(subst(ti.Fields[fi].Type, t.Args), n.vals[i], path+"."+k)
			if err != nil {
				return nil, err
			}
			vals[fi], set[fi] = v, true
		}
		for i, f := range ti.Fields {
			if set[i] {
				continue
			}
			ft := subst(f.Type, t.Args)
			switch {
			case ft.K == KOpt:
				vals[i] = None
			case f.Default != nil:
				vals[i] = f.Default()
			default:
				return nil, &jsonErr{"MissingField", path + "." + f.Name, "missing field " + strconv.Quote(f.Name)}
			}
		}
		return &Object{T: ti, F: vals}, nil
	case KEnum:
		return decodeEnum(t, n, path)
	case KResult, KTuple, KUnit, KRange, KFn:
		return nil, &jsonErr{"TypeMismatch", path, "type is not decodable"}
	case KAny:
		return decodeAny(n), nil
	}
	return nil, &jsonErr{"TypeMismatch", path, "type is not decodable"}
}

func elemT(t *Type, i int) *Type {
	if i < len(t.Args) && t.Args[i] != nil {
		return t.Args[i]
	}
	return TAnyT
}

func decodeAny(n *jnode) Value {
	switch n.k {
	case jNull:
		return None
	case jBool:
		return n.b
	case jNum:
		if i, err := strconv.ParseInt(n.s, 10, 64); err == nil {
			return i
		}
		f, _ := strconv.ParseFloat(n.s, 64)
		return f
	case jStr:
		return n.s
	case jArr:
		out := make([]Value, len(n.arr))
		for i, e := range n.arr {
			out[i] = decodeAny(e)
		}
		return NewList(out)
	}
	m := &Map{}
	for i, k := range n.keys {
		m.put(k, decodeAny(n.vals[i]))
	}
	return m
}

func decodeEnum(t *Type, n *jnode, path string) (Value, *jsonErr) {
	e := t.Enum
	if n.k != jObj {
		return nil, mismatch("object", n, path)
	}
	var tag, val *jnode
	for i, k := range n.keys {
		switch k {
		case "tag":
			tag = n.vals[i]
		case "value":
			val = n.vals[i]
		default:
			return nil, &jsonErr{"UnknownField", path + "." + k, "unknown field " + strconv.Quote(k)}
		}
	}
	if tag == nil {
		return nil, &jsonErr{"MissingField", path + ".tag", "missing field \"tag\""}
	}
	if tag.k != jStr {
		return nil, mismatch("string", tag, path+".tag")
	}
	v := e.byName[tag.s]
	if v == nil {
		return nil, &jsonErr{"TypeMismatch", path + ".tag", "unknown variant " + strconv.Quote(tag.s)}
	}
	vp := path + ".value"
	if len(v.Fields) == 0 {
		if val != nil {
			return nil, &jsonErr{"UnknownField", vp, "variant " + v.Name + " has no payload"}
		}
		return v.Unit(), nil
	}
	if val == nil {
		return nil, &jsonErr{"MissingField", vp, "missing field \"value\""}
	}
	ft := func(i int) *Type {
		if i < len(v.Types) && v.Types[i] != nil {
			return subst(v.Types[i], t.Args)
		}
		return TAnyT
	}
	fields := make([]Value, len(v.Fields))
	if v.Named() {
		if val.k != jObj {
			return nil, mismatch("object", val, vp)
		}
		set := make([]bool, len(v.Fields))
		for i, k := range val.keys {
			fi := -1
			for j, f := range v.Fields {
				if f == k {
					fi = j
				}
			}
			if fi < 0 {
				return nil, &jsonErr{"UnknownField", vp + "." + k, "unknown field " + strconv.Quote(k)}
			}
			x, err := decodeNode(ft(fi), val.vals[i], vp+"."+k)
			if err != nil {
				return nil, err
			}
			fields[fi], set[fi] = x, true
		}
		for i, f := range v.Fields {
			if !set[i] {
				if ft(i).K == KOpt {
					fields[i] = None
					continue
				}
				return nil, &jsonErr{"MissingField", vp + "." + f, "missing field " + strconv.Quote(f)}
			}
		}
	} else {
		if val.k != jArr {
			return nil, mismatch("array", val, vp)
		}
		if len(val.arr) != len(v.Fields) {
			return nil, &jsonErr{"TypeMismatch", vp, fmt.Sprintf("expected %d payload values, found %d", len(v.Fields), len(val.arr))}
		}
		for i, e := range val.arr {
			x, err := decodeNode(ft(i), e, fmt.Sprintf("%s[%d]", vp, i))
			if err != nil {
				return nil, err
			}
			fields[i] = x
		}
	}
	return &Enum{V: v, F: fields}, nil
}
