package pinionrt

import (
	"math"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// builtin method parameter names (only needed when called with named args).
var builtinParams = map[string][]string{
	"div":       {"b", "scale", "round"},
	"round":     {"scale", "round"},
	"slice":     {"start", "end"},
	"fold":      {"init", "f"},
	"set":       {"k", "v"},
	"replace":   {"old", "new"},
	"get_or":    {"k", "d"},
	"update":    {"k", "default", "f"},
	"insert":    {"i", "x"},
	"pad_left":  {"n", "ch"},
	"pad_right": {"n", "ch"},
}

func normArgs(name string, names []string, args []Value) []Value {
	if len(names) == 0 {
		return args
	}
	named := false
	for _, n := range names {
		if n != "" {
			named = true
		}
	}
	if !named {
		return args
	}
	ps, ok := builtinParams[name]
	if !ok {
		return args
	}
	return Reorder(name, ps, names, args, nil)
}

// M calls method `name` on recv with args (names[i] is the name of named
// arg i or ""). The receiver and args are already evaluated.
func M(recv Value, name string, names []string, args ...Value) Value {
	if o, ok := recv.(*Object); ok {
		if m := o.T.Methods[name]; m != nil {
			if len(names) > 0 {
				args = Reorder(o.T.Name+"."+name, m.Params, names, args, nil)
			}
			return m.Fn(o, args)
		}
		if name == "with" && !o.T.Class {
			return With(o, names, args)
		}
		// a field holding a function
		if i := o.T.FieldIndex(name); i >= 0 {
			return Call(o.F[i], args...)
		}
		panicInternal("%s has no method %s", o.T.Name, name)
	}
	args = normArgs(name, names, args)
	switch name {
	case "is_some":
		if _, ok := recv.(*Result); !ok {
			return !IsNone(recv)
		}
	case "is_none":
		if _, ok := recv.(*Result); !ok {
			return IsNone(recv)
		}
	}
	switch x := recv.(type) {
	case int64:
		return intMethod(x, name, args)
	case float64:
		return floatMethod(x, name, args)
	case *Dec:
		return decMethod(x, name, args)
	case string:
		return strMethod(x, name, args)
	case *List:
		return listMethod(x, name, args)
	case *Map:
		return mapMethod(x, name, args)
	case *Set:
		return setMethod(x, name, args)
	case Range:
		return rangeMethod(x, name, args)
	case Bytes:
		return bytesMethod(x, name, args)
	case *Result:
		return resultMethod(x, name, args)
	case Instant:
		return instantMethod(x, name, args)
	case Duration:
		return durationMethod(x, name, args)
	case *Func:
		if name == "call" {
			return Call(x, args...)
		}
	case NoneType:
		if name == "map" {
			return None
		}
	case *Handler:
	}
	if name == "map" && len(args) == 1 { // Some(x).map(f)
		return Call(args[0], recv)
	}
	panicInternal("value of type %s has no method %s", typeName(recv), name)
	return nil
}

func noMethod(t, name string) Value {
	panicInternal("%s has no method %s", t, name)
	return nil
}

func intMethod(x int64, name string, a []Value) Value {
	switch name {
	case "to_float":
		return float64(x)
	case "to_dec":
		return intToDec(x)
	case "abs":
		if x == math.MinInt64 {
			overflow("abs")
		}
		if x < 0 {
			return -x
		}
		return x
	case "max":
		return max(x, a[0].(int64))
	case "min":
		return min(x, a[0].(int64))
	case "pow":
		e := a[0].(int64)
		if e < 0 {
			Abort("R0102", "negative exponent")
		}
		r := int64(1)
		for i := int64(0); i < e; i++ {
			r = mulInt(r, x)
		}
		return r
	case "to_str":
		return Display(x)
	}
	return noMethod("Int", name)
}

func floatMethod(x float64, name string, a []Value) Value {
	switch name {
	case "trunc_int":
		return floatToInt(math.Trunc(x))
	case "round_int":
		return floatToInt(math.Round(x))
	case "abs":
		return math.Abs(x)
	case "max":
		return math.Max(x, a[0].(float64))
	case "min":
		return math.Min(x, a[0].(float64))
	case "sqrt":
		if x < 0 {
			Abort("R0104", "sqrt of negative number")
		}
		return math.Sqrt(x)
	case "to_str":
		return Display(x)
	}
	return noMethod("Float", name)
}

func decMethod(x *Dec, name string, a []Value) Value {
	switch name {
	case "div":
		return x.DivDec(a[0].(*Dec), int(a[1].(int64)), roundMode(a[2]))
	case "round":
		return x.Round(int(a[0].(int64)), roundMode(a[1]))
	case "scale":
		return int64(x.scale)
	case "to_float":
		return checkFloat(x.toFloat())
	case "trunc_int":
		return x.truncInt()
	case "abs":
		if x.u.Sign() < 0 {
			return Neg(x)
		}
		return x
	case "to_str":
		return x.String()
	}
	return noMethod("Dec", name)
}

func strList(xs []string) Value {
	out := make([]Value, len(xs))
	for i, s := range xs {
		out[i] = s
	}
	return NewList(out)
}

func strMethod(s string, name string, a []Value) Value {
	switch name {
	case "len":
		return int64(utf8.RuneCountInString(s))
	case "byte_len":
		return int64(len(s))
	case "is_empty":
		return s == ""
	case "slice":
		return strSlice(s, a[0].(int64), a[1].(int64))
	case "chars":
		out := []Value{}
		for _, r := range s {
			out = append(out, string(r))
		}
		return NewList(out)
	case "trim":
		return strings.TrimSpace(s)
	case "lower":
		return strings.ToLower(s)
	case "upper":
		return strings.ToUpper(s)
	case "words":
		return strList(strings.FieldsFunc(s, unicode.IsSpace))
	case "lines":
		if s == "" {
			return NewList(nil)
		}
		t := strings.TrimSuffix(s, "\n")
		parts := strings.Split(t, "\n")
		for i, p := range parts {
			parts[i] = strings.TrimSuffix(p, "\r")
		}
		return strList(parts)
	case "split":
		return strList(strings.Split(s, a[0].(string)))
	case "contains":
		return strings.Contains(s, a[0].(string))
	case "starts_with":
		return strings.HasPrefix(s, a[0].(string))
	case "ends_with":
		return strings.HasSuffix(s, a[0].(string))
	case "index_of":
		i := strings.Index(s, a[0].(string))
		if i < 0 {
			return None
		}
		return int64(utf8.RuneCountInString(s[:i]))
	case "count":
		sub := a[0].(string)
		if sub == "" {
			return int64(utf8.RuneCountInString(s) + 1)
		}
		return int64(strings.Count(s, sub))
	case "replace":
		return strings.ReplaceAll(s, a[0].(string), a[1].(string))
	case "repeat":
		n := a[0].(int64)
		if n < 0 {
			Abort("R0102", "negative repeat count")
		}
		if n > 0 && int64(len(s))*n/n != int64(len(s)) {
			overflow("repeat")
		}
		return strings.Repeat(s, int(n))
	case "to_bytes":
		return Bytes(s)
	case "before":
		b, _, ok := strings.Cut(s, a[0].(string))
		if !ok {
			return None
		}
		return b
	case "after":
		_, af, ok := strings.Cut(s, a[0].(string))
		if !ok {
			return None
		}
		return af
	case "to_str":
		return s
	case "pad_left", "pad_right":
		ch := " "
		if len(a) > 1 && a[1] != nil {
			ch = a[1].(string)
		}
		n := a[0].(int64) - int64(utf8.RuneCountInString(s))
		if n <= 0 || ch == "" {
			return s
		}
		pad := []rune(strings.Repeat(ch, int(n)))[:n]
		if name == "pad_left" {
			return string(pad) + s
		}
		return s + string(pad)
	}
	return noMethod("Str", name)
}

// shared sequence higher-order functions.
func seqMethod(xs []Value, name string, a []Value) (Value, bool) {
	switch name {
	case "len":
		return int64(len(xs)), true
	case "is_empty":
		return len(xs) == 0, true
	case "map":
		out := make([]Value, 0, len(xs))
		for _, x := range xs {
			out = append(out, Call(a[0], x))
		}
		return NewList(out), true
	case "each":
		for _, x := range xs {
			Call(a[0], x)
		}
		return UnitV, true
	case "fold":
		acc := a[0]
		for _, x := range xs {
			acc = Call(a[1], acc, x)
		}
		return acc, true
	case "any":
		for _, x := range xs {
			if Truthy(Call(a[0], x)) {
				return true, true
			}
		}
		return false, true
	case "all":
		for _, x := range xs {
			if !Truthy(Call(a[0], x)) {
				return false, true
			}
		}
		return true, true
	case "find":
		for _, x := range xs {
			if Truthy(Call(a[0], x)) {
				return x, true
			}
		}
		return None, true
	case "contains":
		for _, x := range xs {
			if Equal(x, a[0]) {
				return true, true
			}
		}
		return false, true
	case "to_list":
		return NewList(append([]Value(nil), xs...)), true
	case "to_set":
		return NewSetFrom(xs), true
	case "join":
		parts := make([]string, len(xs))
		for i, x := range xs {
			parts[i] = x.(string)
		}
		return strings.Join(parts, a[0].(string)), true
	case "sum":
		if len(xs) == 0 {
			return int64(0), true
		}
		acc := xs[0]
		for _, x := range xs[1:] {
			acc = Add(acc, x)
		}
		return acc, true
	case "get":
		i := a[0].(int64)
		if i < 0 || i >= int64(len(xs)) {
			return None, true
		}
		return xs[i], true
	case "first":
		if len(xs) == 0 {
			return None, true
		}
		return xs[0], true
	case "last":
		if len(xs) == 0 {
			return None, true
		}
		return xs[len(xs)-1], true
	case "sort":
		out := append([]Value(nil), xs...)
		slices.SortStableFunc(out, Compare)
		return NewList(out), true
	case "sort_by":
		type kv struct{ k, v Value }
		ks := make([]kv, len(xs))
		for i, x := range xs {
			ks[i] = kv{Call(a[0], x), x}
		}
		slices.SortStableFunc(ks, func(p, q kv) int { return Compare(p.k, q.k) })
		out := make([]Value, len(xs))
		for i := range ks {
			out[i] = ks[i].v
		}
		return NewList(out), true
	case "reverse":
		out := make([]Value, len(xs))
		for i, x := range xs {
			out[len(xs)-1-i] = x
		}
		return NewList(out), true
	case "append":
		out := append(append([]Value(nil), xs...), a[0])
		return NewList(out), true
	case "concat":
		out := append(append([]Value(nil), xs...), elems(a[0])...)
		return NewList(out), true
	case "slice":
		s, e := a[0].(int64), a[1].(int64)
		checkSlice(s, e, len(xs))
		return NewList(append([]Value(nil), xs[s:e]...)), true
	case "enumerate":
		out := make([]Value, len(xs))
		for i, x := range xs {
			out[i] = &Tuple{Elems: []Value{int64(i), x}}
		}
		return NewList(out), true
	case "filter":
		out := []Value{}
		for _, x := range xs {
			if Truthy(Call(a[0], x)) {
				out = append(out, x)
			}
		}
		return NewList(out), true
	case "count_by":
		out := &Map{}
		for _, x := range xs {
			k := Call(a[0], x)
			i, added := out.keys.add(k)
			if added {
				out.vals = append(out.vals, int64(1))
			} else {
				out.vals[i] = out.vals[i].(int64) + 1
			}
		}
		return out, true
	case "group_by":
		out := &Map{}
		var groups [][]Value
		for _, x := range xs {
			i, added := out.keys.add(Call(a[0], x))
			if added {
				groups = append(groups, nil)
			}
			groups[i] = append(groups[i], x)
		}
		for _, g := range groups {
			out.vals = append(out.vals, NewList(g))
		}
		return out, true
	case "take", "drop":
		n := a[0].(int64)
		if n < 0 {
			Abort("R0102", name+": negative count")
		}
		n = min(n, int64(len(xs)))
		if name == "take" {
			return NewList(append([]Value(nil), xs[:n]...)), true
		}
		return NewList(append([]Value(nil), xs[n:]...)), true
	case "max_by", "min_by":
		var best, bestK Value
		for i, x := range xs {
			k := Call(a[0], x)
			c := 0
			if i > 0 {
				c = Compare(k, bestK)
			}
			if i == 0 || (name == "max_by" && c > 0) || (name == "min_by" && c < 0) {
				best, bestK = x, k
			}
		}
		if best == nil {
			return None, true
		}
		return best, true
	case "sum_by":
		if len(xs) == 0 {
			return int64(0), true
		}
		acc := Call(a[0], xs[0])
		for _, x := range xs[1:] {
			acc = Add(acc, Call(a[0], x))
		}
		return acc, true
	case "index_of":
		for i, x := range xs {
			if Equal(x, a[0]) {
				return int64(i), true
			}
		}
		return None, true
	case "distinct":
		return NewList(append([]Value(nil), NewSetFrom(xs).s.items...)), true
	}
	return nil, false
}

func listMethod(l *List, name string, a []Value) Value {
	if l.Mut {
		switch name {
		case "push":
			l.mutate()
			l.Items = append(l.Items, a[0])
			return UnitV
		case "pop":
			l.mutate()
			if len(l.Items) == 0 {
				return None
			}
			x := l.Items[len(l.Items)-1]
			l.Items = l.Items[:len(l.Items)-1]
			return x
		case "set":
			listSet(l, a[0].(int64), a[1])
			return UnitV
		case "clear":
			l.mutate()
			l.Items = nil
			return UnitV
		case "freeze":
			return NewList(append([]Value(nil), l.Items...))
		case "remove_at":
			i := a[0].(int64)
			if i < 0 || i >= int64(len(l.Items)) {
				Abort("R0102", "index "+Display(i)+" out of range for length "+Display(int64(len(l.Items))))
			}
			l.mutate()
			x := l.Items[i]
			l.Items = append(l.Items[:i:i], l.Items[i+1:]...)
			return x
		case "insert":
			i := a[0].(int64)
			if i < 0 || i > int64(len(l.Items)) {
				Abort("R0102", "index "+Display(i)+" out of range for insert into length "+Display(int64(len(l.Items))))
			}
			l.mutate()
			out := make([]Value, 0, len(l.Items)+1)
			out = append(append(append(out, l.Items[:i]...), a[1]), l.Items[i:]...)
			l.Items = out
			return UnitV
		case "retain":
			l.mutate()
			l.iter++
			var keep []Value
			func() {
				defer func() { l.iter-- }()
				for _, x := range l.Items {
					if Truthy(Call(a[0], x)) {
						keep = append(keep, x)
					}
				}
			}()
			l.Items = keep
			return UnitV
		}
	}
	// snapshot so callbacks mutating a MutList are detected
	l.iter++
	defer func() { l.iter-- }()
	if v, ok := seqMethod(l.Items, name, a); ok {
		return v
	}
	return noMethod(typeName(l), name)
}

func mapMethod(m *Map, name string, a []Value) Value {
	switch name {
	case "len":
		return int64(m.Len())
	case "is_empty":
		return m.Len() == 0
	case "get":
		v, ok := m.Get(a[0])
		if !ok {
			return None
		}
		return v
	case "contains_key":
		_, ok := m.Get(a[0])
		return ok
	case "keys":
		return NewList(append([]Value(nil), m.keys.items...))
	case "values":
		return NewList(append([]Value(nil), m.vals...))
	case "entries":
		out := make([]Value, m.Len())
		for i, k := range m.keys.items {
			out[i] = &Tuple{Elems: []Value{k, m.vals[i]}}
		}
		return NewList(out)
	case "to_mut":
		return m.copyAs(true)
	case "get_or":
		if v, ok := m.Get(a[0]); ok {
			return v
		}
		return a[1]
	case "map_values", "filter":
		m.iter++
		defer func() { m.iter-- }()
		out := &Map{}
		for i, k := range m.keys.items {
			v := m.vals[i]
			if name == "map_values" {
				out.put(k, Call(a[0], v))
			} else if Truthy(Call(a[0], k, v)) {
				out.put(k, v)
			}
		}
		return out
	}
	if m.Mut {
		switch name {
		case "set":
			m.mutate()
			m.put(a[0], a[1])
			return UnitV
		case "remove":
			m.mutate()
			i := m.keys.find(a[0])
			if i < 0 {
				return None
			}
			v := m.vals[i]
			m.keys.removeAt(i)
			m.vals = append(m.vals[:i:i], m.vals[i+1:]...)
			return v
		case "clear":
			m.mutate()
			m.keys = ordIndex{}
			m.vals = nil
			return UnitV
		case "freeze":
			return m.copyAs(false)
		case "inc":
			m.mutate()
			i, added := m.keys.add(a[0])
			if added {
				m.vals = append(m.vals, int64(1))
			} else {
				m.vals[i] = Add(m.vals[i], int64(1))
			}
			return UnitV
		case "update":
			old, ok := m.Get(a[0])
			if !ok {
				old = a[1]
			}
			m.iter++
			nv := func() Value {
				defer func() { m.iter-- }()
				return Call(a[2], old)
			}()
			m.mutate()
			m.put(a[0], nv)
			return UnitV
		}
	}
	switch name {
	case "each", "any", "all", "map", "fold", "find":
		ents := mapMethod(m, "entries", nil).(*List).Items
		m.iter++
		defer func() { m.iter-- }()
		v, _ := seqMethod(ents, name, a)
		return v
	}
	return noMethod(typeName(m), name)
}

func setMethod(s *Set, name string, a []Value) Value {
	if s.Mut {
		switch name {
		case "add":
			s.mutate()
			_, added := s.s.add(a[0])
			return added
		case "remove":
			s.mutate()
			i := s.s.find(a[0])
			if i < 0 {
				return false
			}
			s.s.removeAt(i)
			return true
		case "clear":
			s.mutate()
			s.s = ordIndex{}
			return UnitV
		case "freeze":
			return &Set{s: s.s.clone()}
		}
	}
	switch name {
	case "contains":
		return s.contains(a[0])
	case "union":
		b := a[0].(*Set)
		out := &Set{s: s.s.clone()}
		for _, x := range b.s.items {
			out.s.add(x)
		}
		return out
	case "intersect":
		b := a[0].(*Set)
		out := &Set{}
		for _, x := range s.s.items {
			if b.contains(x) {
				out.s.add(x)
			}
		}
		return out
	case "diff":
		return setDiff(s, a[0].(*Set))
	case "sym_diff":
		b := a[0].(*Set)
		out := setDiff(s, b)
		for _, x := range setDiff(b, s).s.items {
			out.s.add(x)
		}
		return out
	case "is_subset":
		b := a[0].(*Set)
		for _, x := range s.s.items {
			if !b.contains(x) {
				return false
			}
		}
		return true
	case "is_disjoint":
		b := a[0].(*Set)
		for _, x := range s.s.items {
			if b.contains(x) {
				return false
			}
		}
		return true
	case "filter":
		out := &Set{}
		s.iter++
		defer func() { s.iter-- }()
		for _, x := range s.s.items {
			if Truthy(Call(a[0], x)) {
				out.s.add(x)
			}
		}
		return out
	}
	s.iter++
	defer func() { s.iter-- }()
	if v, ok := seqMethod(s.s.items, name, a); ok {
		return v
	}
	return noMethod(typeName(s), name)
}

func setDiff(a, b *Set) *Set {
	out := &Set{}
	for _, x := range a.s.items {
		if !b.contains(x) {
			out.s.add(x)
		}
	}
	return out
}

func rangeMethod(r Range, name string, a []Value) Value {
	switch name {
	case "to_list":
		return NewList(r.toList())
	case "rev":
		return r.rev()
	case "step":
		n := a[0].(int64)
		if n <= 0 {
			Abort("R0102", "range step must be positive")
		}
		return Range{Start: r.Start, End: r.End, Step: mulInt(r.Step, n)}
	case "contains":
		return r.contains(a[0].(int64))
	case "len":
		return r.Len()
	}
	if v, ok := seqMethod(r.toList(), name, a); ok {
		return v
	}
	return noMethod("Range", name)
}

func bytesMethod(b Bytes, name string, a []Value) Value {
	switch name {
	case "len":
		return int64(len(b))
	case "is_empty":
		return len(b) == 0
	case "slice":
		s, e := a[0].(int64), a[1].(int64)
		checkSlice(s, e, len(b))
		return b[s:e]
	case "to_str":
		s := string(b)
		if !utf8.ValidString(s) {
			off := 0
			for off < len(s) {
				r, sz := utf8.DecodeRuneInString(s[off:])
				if r == utf8.RuneError && sz <= 1 {
					break
				}
				off += sz
			}
			return Err(&Object{T: Utf8ErrorType, F: []Value{int64(off)}})
		}
		return Ok(s)
	case "to_list":
		out := make([]Value, len(b))
		for i := range b {
			out[i] = int64(b[i])
		}
		return NewList(out)
	}
	return noMethod("Bytes", name)
}

func resultMethod(r *Result, name string, a []Value) Value {
	switch name {
	case "ok":
		if r.IsOk {
			return r.V
		}
		return None
	case "map":
		if r.IsOk {
			return Ok(Call(a[0], r.V))
		}
		return r
	case "map_err":
		if !r.IsOk {
			return Err(Call(a[0], r.V))
		}
		return r
	case "is_ok":
		return r.IsOk
	case "is_err":
		return !r.IsOk
	case "unwrap_or":
		if r.IsOk {
			return r.V
		}
		return a[0]
	case "err":
		if !r.IsOk {
			return r.V
		}
		return None
	}
	return noMethod("Result", name)
}

func addDur(a, b int64) int64 {
	r := a + b
	if (r > a) != (b > 0) {
		overflow("time arithmetic")
	}
	return r
}

func instantMethod(t Instant, name string, a []Value) Value {
	switch name {
	case "add":
		return Instant{NS: addDur(t.NS, int64(a[0].(Duration)))}
	case "sub":
		d := int64(a[0].(Duration))
		if d == math.MinInt64 {
			overflow("time arithmetic")
		}
		return Instant{NS: addDur(t.NS, -d)}
	case "since":
		o := a[0].(Instant).NS
		if o == math.MinInt64 {
			overflow("time arithmetic")
		}
		return Duration(addDur(t.NS, -o))
	case "unix":
		ns := t.NS
		s := ns / 1e9
		if ns%1e9 < 0 {
			s--
		}
		return s
	}
	return noMethod("Instant", name)
}

func durationMethod(d Duration, name string, a []Value) Value {
	switch name {
	case "plus":
		return Duration(addDur(int64(d), int64(a[0].(Duration))))
	case "minus":
		o := int64(a[0].(Duration))
		if o == math.MinInt64 {
			overflow("duration arithmetic")
		}
		return Duration(addDur(int64(d), -o))
	case "times":
		return Duration(mulInt(int64(d), a[0].(int64)))
	case "ms":
		return int64(d) / 1e6
	case "ns":
		return int64(d)
	}
	return noMethod("Duration", name)
}

// ---------- prelude / type-name functions ----------

// MkDuration is Duration(ms: 500) etc.
func MkDuration(names []string, args []Value) Value {
	if len(args) != 1 {
		panicInternal("Duration takes exactly one named argument")
	}
	unit := "ns"
	if len(names) > 0 && names[0] != "" {
		unit = names[0]
	}
	n := args[0].(int64)
	mult := map[string]int64{"ns": 1, "us": 1e3, "ms": 1e6, "s": 1e9, "m": 60e9, "h": 3600e9}[unit]
	if mult == 0 {
		panicInternal("Duration: unknown unit %s", unit)
	}
	return Duration(mulInt(n, mult))
}

// MkSet is Set(xs).
func MkSet(xs Value) Value { return NewSetFrom(elems(xs)) }

// StaticCall implements type-name functions: Int.parse, Bytes.from_list, ...
func StaticCall(typ, name string, args ...Value) Value {
	switch typ + "." + name {
	case "Int.parse":
		return parseIntStr(args[0].(string))
	case "Float.parse":
		return parseFloatStr(args[0].(string))
	case "Dec.parse":
		return parseDecStr(args[0].(string))
	case "Bytes.from_list":
		xs := elems(args[0])
		b := make([]byte, len(xs))
		for i, x := range xs {
			n := x.(int64)
			if n < 0 || n > 255 {
				Abort("R0101", "byte value out of range: "+Display(n))
			}
			b[i] = byte(n)
		}
		return Bytes(b)
	case "Bytes.empty":
		return Bytes("")
	}
	panicInternal("unknown function %s.%s", typ, name)
	return nil
}

// Debugf is the prelude debug(x).
func DebugF(x Value) Value { return Debug(x) }

// Todo is todo(msg).
func Todo(args ...Value) Value {
	msg := "not yet implemented"
	if len(args) > 0 && args[0].(string) != "" {
		msg = "todo: " + args[0].(string)
	}
	Abort("R0105", msg)
	return nil
}

// Unreachable is unreachable(msg).
func Unreachable(args ...Value) Value {
	msg := "entered unreachable code"
	if len(args) > 0 && args[0].(string) != "" {
		msg = "unreachable: " + args[0].(string)
	}
	Abort("R0106", msg)
	return nil
}
