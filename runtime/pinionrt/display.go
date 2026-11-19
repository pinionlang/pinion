package pinionrt

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Display renders a value inside string interpolation "{x}".
func Display(v Value) string {
	switch x := v.(type) {
	case string:
		return x
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return FormatFloat(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case *Dec:
		return x.String()
	case Instant:
		return formatInstant(x)
	case Duration:
		return time.Duration(x).String()
	}
	return Debug(v)
}

// Interp concatenates interpolation parts.
func Interp(parts ...Value) Value {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(Display(p))
	}
	return b.String()
}

func formatInstant(t Instant) string {
	tm := time.Unix(0, t.NS).UTC()
	return tm.Format("2006-01-02T15:04:05.999999999Z07:00")
}

const (
	debugDepth = 4
	debugElems = 20
)

// Debug renders the debug representation (DESIGN 3.8).
func Debug(v Value) string {
	var b strings.Builder
	d := debugger{b: &b, seen: map[*Object]bool{}}
	d.val(v, 0)
	return b.String()
}

type debugger struct {
	b    *strings.Builder
	seen map[*Object]bool
}

func (d *debugger) w(s string) { d.b.WriteString(s) }

func (d *debugger) seq(open, close string, xs []Value, depth int) {
	d.w(open)
	if depth >= debugDepth && len(xs) > 0 {
		d.w("…")
		d.w(close)
		return
	}
	for i, x := range xs {
		if i == debugElems {
			d.w(fmt.Sprintf(", …(+%d)", len(xs)-debugElems))
			break
		}
		if i > 0 {
			d.w(", ")
		}
		d.val(x, depth+1)
	}
	d.w(close)
}

func (d *debugger) val(v Value, depth int) {
	switch x := v.(type) {
	case string:
		d.w(strconv.Quote(x))
	case int64, float64, bool, Instant, Duration:
		d.w(Display(x))
	case *Dec:
		d.w(x.String() + "d")
	case Unit:
		d.w("()")
	case NoneType:
		d.w("None")
	case Bytes:
		xs := make([]Value, len(x))
		for i := range x {
			xs[i] = int64(x[i])
		}
		d.seq("Bytes[", "]", xs, depth)
	case *List:
		if x.Mut {
			d.seq("MutList[", "]", x.Items, depth)
		} else {
			d.seq("[", "]", x.Items, depth)
		}
	case *Set:
		if x.Mut {
			d.seq("MutSet{", "}", x.s.items, depth)
		} else {
			d.seq("Set{", "}", x.s.items, depth)
		}
	case *Map:
		if x.Mut {
			d.w("MutMap")
		}
		d.w("{")
		if depth >= debugDepth && x.Len() > 0 {
			d.w("…}")
			return
		}
		for i, k := range x.keys.items {
			if i == debugElems {
				d.w(fmt.Sprintf(", …(+%d)", x.Len()-debugElems))
				break
			}
			if i > 0 {
				d.w(", ")
			}
			d.val(k, depth+1)
			d.w(": ")
			d.val(x.vals[i], depth+1)
		}
		d.w("}")
	case *Tuple:
		d.seq("(", ")", x.Elems, depth)
	case Range:
		d.w(fmt.Sprintf("Range(%d, %d, step: %d)", x.Start, x.End, x.Step))
	case *Result:
		if x.IsOk {
			d.w("Ok(")
		} else {
			d.w("Err(")
		}
		d.val(x.V, depth+1)
		d.w(")")
	case *Enum:
		d.w(x.V.T.Name + "." + x.V.Name)
		if len(x.F) > 0 {
			d.w("(")
			if depth >= debugDepth {
				d.w("…)")
				return
			}
			for i, f := range x.F {
				if i > 0 {
					d.w(", ")
				}
				if n := x.V.Fields[i]; n != "" {
					d.w(n + ": ")
				}
				d.val(f, depth+1)
			}
			d.w(")")
		}
	case *Object:
		if d.seen[x] {
			d.w(fmt.Sprintf("<cycle #%d>", x.ID()))
			return
		}
		d.w(x.T.Name + "(")
		if depth >= debugDepth && len(x.F) > 0 {
			d.w("…)")
			return
		}
		d.seen[x] = true
		for i, f := range x.F {
			if i > 0 {
				d.w(", ")
			}
			d.w(x.T.Fields[i].Name + ": ")
			d.val(f, depth+1)
		}
		delete(d.seen, x)
		d.w(")")
	case *Func:
		if x.Arity >= 0 {
			ps := make([]string, x.Arity)
			for i := range ps {
				ps[i] = "_"
			}
			d.w("<fn (" + strings.Join(ps, ", ") + ")>")
		} else {
			d.w("<fn>")
		}
	case *Handler:
		d.w("<" + x.Effect + " handler>")
	default:
		d.w(fmt.Sprintf("<%T>", v))
	}
}
