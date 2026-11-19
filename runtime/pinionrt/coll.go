package pinionrt

import (
	"fmt"
	"iter"
	"unicode/utf8"
)

// List is List<T> (Mut=false) or MutList<T> (Mut=true).
type List struct {
	Items []Value
	Mut   bool
	iter  int
}

// ListLit builds an immutable list.
func ListLit(xs ...Value) Value { return &List{Items: xs} }

// NewList builds a list from a slice (taken over).
func NewList(xs []Value) *List { return &List{Items: xs} }

// NewMutList is MutList<T>().
func NewMutList() Value { return &List{Mut: true} }

func (l *List) mutate() {
	if l.iter > 0 {
		Abort("R0107", "MutList modified while being iterated")
	}
}

// ordered hash index shared by Map and Set.
type ordIndex struct {
	items []Value
	idx   map[uint64][]int
}

func (o *ordIndex) find(k Value) int {
	if o.idx == nil {
		return -1
	}
	for _, i := range o.idx[Hash(k)] {
		if Equal(o.items[i], k) {
			return i
		}
	}
	return -1
}

func (o *ordIndex) add(k Value) (int, bool) {
	if i := o.find(k); i >= 0 {
		return i, false
	}
	if o.idx == nil {
		o.idx = map[uint64][]int{}
	}
	h := Hash(k)
	o.items = append(o.items, k)
	o.idx[h] = append(o.idx[h], len(o.items)-1)
	return len(o.items) - 1, true
}

func (o *ordIndex) removeAt(i int) {
	o.items = append(o.items[:i:i], o.items[i+1:]...)
	o.reindex()
}

func (o *ordIndex) reindex() {
	o.idx = make(map[uint64][]int, len(o.items))
	for i, k := range o.items {
		h := Hash(k)
		o.idx[h] = append(o.idx[h], i)
	}
}

func (o *ordIndex) clone() ordIndex {
	n := ordIndex{items: append([]Value(nil), o.items...)}
	n.reindex()
	return n
}

// Map is Map<K,V> / MutMap<K,V>, iterating in insertion order.
type Map struct {
	keys ordIndex
	vals []Value
	Mut  bool
	iter int
}

// MapLit builds an immutable map from k1, v1, k2, v2, ...
func MapLit(kv ...Value) Value {
	m := &Map{}
	for i := 0; i+1 < len(kv); i += 2 {
		m.put(kv[i], kv[i+1])
	}
	return m
}

// NewMutMap is MutMap<K,V>().
func NewMutMap() Value { return &Map{Mut: true} }

// Len returns the number of entries.
func (m *Map) Len() int { return len(m.keys.items) }

func (m *Map) put(k, v Value) {
	i, added := m.keys.add(k)
	if added {
		m.vals = append(m.vals, v)
	} else {
		m.vals[i] = v
	}
}

// Get returns the value for k or (nil, false).
func (m *Map) Get(k Value) (Value, bool) {
	i := m.keys.find(k)
	if i < 0 {
		return nil, false
	}
	return m.vals[i], true
}

// Keys returns keys in insertion order.
func (m *Map) Keys() []Value { return m.keys.items }

// Vals returns values in insertion order.
func (m *Map) Vals() []Value { return m.vals }

func (m *Map) mutate() {
	if m.iter > 0 {
		Abort("R0107", "MutMap modified while being iterated")
	}
}

func (m *Map) copyAs(mut bool) *Map {
	return &Map{keys: m.keys.clone(), vals: append([]Value(nil), m.vals...), Mut: mut}
}

// Set is Set<T> / MutSet<T>, iterating in insertion order.
type Set struct {
	s    ordIndex
	Mut  bool
	iter int
}

// Len returns the element count.
func (s *Set) Len() int { return len(s.s.items) }

// Items returns elements in insertion order.
func (s *Set) Items() []Value { return s.s.items }

// NewSetFrom builds an immutable set from a list (dedup, first occurrence order).
func NewSetFrom(xs []Value) *Set {
	s := &Set{}
	for _, x := range xs {
		s.s.add(x)
	}
	return s
}

// NewMutSet is MutSet<T>().
func NewMutSet() Value { return &Set{Mut: true} }

func (s *Set) contains(x Value) bool { return s.s.find(x) >= 0 }

func (s *Set) mutate() {
	if s.iter > 0 {
		Abort("R0107", "MutSet modified while being iterated")
	}
}

// Range is an integer range: Start, Start+Step, ... up to (not incl.) End.
type Range struct {
	Start, End, Step int64
}

// MkRange builds a..b or a..=b.
func MkRange(lo, hi Value, inclusive bool) Value {
	a, b := lo.(int64), hi.(int64)
	if inclusive {
		if b == 1<<63-1 {
			overflow("..=")
		}
		b++
	}
	return Range{Start: a, End: b, Step: 1}
}

// Len returns the number of elements.
func (r Range) Len() int64 {
	if r.Step > 0 {
		if r.Start >= r.End {
			return 0
		}
		return (r.End-r.Start-1)/r.Step + 1
	}
	if r.Start <= r.End {
		return 0
	}
	return (r.Start-r.End-1)/(-r.Step) + 1
}

func (r Range) at(i int64) int64 { return r.Start + i*r.Step }

func rangeEqual(a, b Range) bool {
	na, nb := a.Len(), b.Len()
	if na != nb {
		return false
	}
	if na == 0 {
		return true
	}
	return a.Start == b.Start && (na == 1 || a.Step == b.Step)
}

func (r Range) contains(x int64) bool {
	n := r.Len()
	if n == 0 {
		return false
	}
	if r.Step > 0 {
		return x >= r.Start && x < r.End && (x-r.Start)%r.Step == 0
	}
	return x <= r.Start && x > r.End && (r.Start-x)%(-r.Step) == 0
}

func (r Range) rev() Range {
	n := r.Len()
	if n == 0 {
		return Range{Start: 0, End: 0, Step: 1}
	}
	last := r.at(n - 1)
	return Range{Start: last, End: r.Start - r.Step, Step: -r.Step}
}

func (r Range) toList() []Value {
	n := r.Len()
	out := make([]Value, 0, n)
	for i := int64(0); i < n; i++ {
		out = append(out, r.at(i))
	}
	return out
}

// ---------- iteration ----------

// Iter returns an iterator for `for x in v`.
func Iter(v Value) iter.Seq[Value] {
	switch x := v.(type) {
	case *List:
		return func(yield func(Value) bool) {
			x.iter++
			defer func() { x.iter-- }()
			for i := 0; i < len(x.Items); i++ {
				if !yield(x.Items[i]) {
					return
				}
			}
		}
	case *Set:
		return func(yield func(Value) bool) {
			x.iter++
			defer func() { x.iter-- }()
			for i := 0; i < len(x.s.items); i++ {
				if !yield(x.s.items[i]) {
					return
				}
			}
		}
	case *Map:
		return func(yield func(Value) bool) {
			x.iter++
			defer func() { x.iter-- }()
			for i := 0; i < len(x.keys.items); i++ {
				if !yield(&Tuple{Elems: []Value{x.keys.items[i], x.vals[i]}}) {
					return
				}
			}
		}
	case Range:
		return func(yield func(Value) bool) {
			n := x.Len()
			for i := int64(0); i < n; i++ {
				if !yield(x.at(i)) {
					return
				}
			}
		}
	case Bytes:
		return func(yield func(Value) bool) {
			for i := 0; i < len(x); i++ {
				if !yield(int64(x[i])) {
					return
				}
			}
		}
	}
	panicInternal("cannot iterate over %s", typeName(v))
	return nil
}

// elems returns the elements of a list-like value (for HOFs).
func elems(v Value) []Value {
	switch x := v.(type) {
	case *List:
		return x.Items
	case *Set:
		return x.s.items
	case Range:
		return x.toList()
	}
	panicInternal("not a sequence: %s", typeName(v))
	return nil
}

// Index is `x[i]`.
func Index(x, i Value) Value {
	switch c := x.(type) {
	case *List:
		n := i.(int64)
		if n < 0 || n >= int64(len(c.Items)) {
			Abort("R0102", fmt.Sprintf("index out of range: %d >= %d", n, len(c.Items)))
		}
		return c.Items[n]
	case Bytes:
		n := i.(int64)
		if n < 0 || n >= int64(len(c)) {
			Abort("R0102", fmt.Sprintf("index out of range: %d >= %d", n, len(c)))
		}
		return int64(c[n])
	case *Map:
		v, ok := c.Get(i)
		if !ok {
			Abort("R0102", "key not found: "+Debug(i))
		}
		return v
	case *Tuple:
		n := i.(int64)
		return c.Elems[n]
	}
	panicInternal("cannot index %s", typeName(x))
	return nil
}

// SetIndex is `x[i] = v` (MutList / MutMap).
func SetIndex(x, i, v Value) {
	switch c := x.(type) {
	case *List:
		listSet(c, i.(int64), v)
		return
	case *Map:
		c.mutate()
		c.put(i, v)
		return
	}
	panicInternal("cannot assign index on %s", typeName(x))
}

func listSet(c *List, n int64, v Value) {
	if !c.Mut {
		panicInternal("List is immutable")
	}
	c.mutate()
	if n < 0 || n >= int64(len(c.Items)) {
		Abort("R0102", fmt.Sprintf("index out of range: %d >= %d", n, len(c.Items)))
	}
	c.Items[n] = v
}

func checkSlice(start, end int64, n int) {
	if start < 0 || end < start || end > int64(n) {
		Abort("R0102", fmt.Sprintf("slice [%d, %d) out of range for length %d", start, end, n))
	}
}

func strSlice(s string, start, end int64) string {
	n := utf8.RuneCountInString(s)
	checkSlice(start, end, n)
	i := 0
	bs, be := len(s), len(s)
	for off := range s {
		if int64(i) == start {
			bs = off
		}
		if int64(i) == end {
			be = off
			break
		}
		i++
	}
	if start == int64(n) {
		bs = len(s)
	}
	return s[bs:be]
}
