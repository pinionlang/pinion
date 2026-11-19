package pinionrt

import (
	"fmt"
	"os"
)

// SnapOn enables value snapshots in runtime diagnostics (DESIGN 5.2).
// Generated functions only install their snapshot hook when it is set, so
// the disabled path costs one global load per call.
var SnapOn = os.Getenv("PINION_SNAPSHOT") == "1"

// pending is set right before a Pinion defect or assert failure panics;
// the first snapshot hook that runs while it is set is the innermost frame.
var pending bool

type snapFrame struct {
	hasSelf bool
	self    Value
	names   []string
	args    []Value
}

var snapshot *snapFrame

func raise(v any) {
	pending = true
	panic(v)
}

// SnapExit is deferred by generated functions when SnapOn is set. It does
// not recover, so the panic keeps its stack for piStack.
func SnapExit(hasSelf bool, self Value, names []string, args ...Value) {
	if pending && snapshot == nil {
		snapshot = &snapFrame{hasSelf: hasSelf, self: self, names: names, args: args}
	}
}

func snapshotJSON() any {
	if !SnapOn {
		return nil
	}
	out := map[string]any{}
	args := map[string]any{}
	if f := snapshot; f != nil {
		if f.hasSelf {
			out["self"] = DebugJSON(f.self)
		}
		for i, n := range f.names {
			if i < len(f.args) {
				args[n] = DebugJSON(f.args[i])
			}
		}
	}
	out["args"] = args
	return out
}

// DebugJSON renders the debug representation (DESIGN 3.8: depth 4, 20
// items, cycles marked) as structured JSON. Values with no natural JSON
// form use their textual debug representation.
func DebugJSON(v Value) any {
	return debugJSON(v, 0, map[*Object]bool{})
}

func jsonSeq(xs []Value, depth int, seen map[*Object]bool) any {
	if depth >= debugDepth && len(xs) > 0 {
		return "…"
	}
	out := make([]any, 0, min(len(xs), debugElems+1))
	for i, x := range xs {
		if i == debugElems {
			out = append(out, fmt.Sprintf("…(+%d)", len(xs)-debugElems))
			break
		}
		out = append(out, debugJSON(x, depth+1, seen))
	}
	return out
}

func debugJSON(v Value, depth int, seen map[*Object]bool) any {
	switch x := v.(type) {
	case string, int64, bool:
		return x
	case float64:
		if x != x || x > 1e308 || x < -1e308 {
			return Debug(x)
		}
		return x
	case *List:
		return jsonSeq(x.Items, depth, seen)
	case *Set:
		return jsonSeq(x.s.items, depth, seen)
	case *Tuple:
		return jsonSeq(x.Elems, depth, seen)
	case *Map:
		for _, k := range x.keys.items {
			if _, ok := k.(string); !ok {
				return Debug(x)
			}
		}
		if depth >= debugDepth && x.Len() > 0 {
			return "…"
		}
		out := map[string]any{}
		for i, k := range x.keys.items {
			if i == debugElems {
				out["…"] = fmt.Sprintf("+%d", x.Len()-debugElems)
				break
			}
			out[k.(string)] = debugJSON(x.vals[i], depth+1, seen)
		}
		return out
	case *Object:
		if seen[x] {
			return fmt.Sprintf("<cycle #%d>", x.ID())
		}
		out := map[string]any{"class": x.T.Name}
		if depth >= debugDepth && len(x.F) > 0 {
			out["…"] = true
			return out
		}
		seen[x] = true
		for i, f := range x.F {
			out[x.T.Fields[i].Name] = debugJSON(f, depth+1, seen)
		}
		delete(seen, x)
		return out
	}
	return Debug(v)
}
