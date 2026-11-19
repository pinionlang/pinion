package pinionrt

// OptM calls an option method (is_some, is_none, map) on a value of static
// type T?. Some(x) is represented as x itself, so this must not dispatch on
// the inner value's own methods.
func OptM(recv Value, name string, names []string, args ...Value) Value {
	switch name {
	case "is_some":
		return !IsNone(recv)
	case "is_none":
		return IsNone(recv)
	case "map":
		if IsNone(recv) {
			return recv
		}
		return Call(args[0], recv)
	}
	return M(recv, name, names, args...)
}
