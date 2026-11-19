package check

func (c *checker) fresh() *Type {
	c.nextVar++
	return &Type{K: KVar, V: &TVar{id: c.nextVar}}
}

func (c *checker) bind(v *TVar, t *Type) {
	v.bound = t
	c.trail = append(c.trail, v)
}

func (c *checker) snapshot() int { return len(c.trail) }

func (c *checker) rollback(n int) {
	for len(c.trail) > n {
		v := c.trail[len(c.trail)-1]
		v.bound = nil
		c.trail = c.trail[:len(c.trail)-1]
	}
}

func occurs(v *TVar, t *Type) bool {
	t = prune(t)
	if t == nil {
		return false
	}
	if t.K == KVar {
		return t.V == v
	}
	for _, a := range t.Args {
		if occurs(v, a) {
			return true
		}
	}
	return occurs(v, t.Ret)
}

// unify makes a and b equal (no subtyping). Function effects must be equal
// unless one side is a std callback type (Poly).
func (c *checker) unify(a, b *Type) bool {
	a, b = prune(a), prune(b)
	if a == nil || b == nil {
		return a == b
	}
	if a == b {
		return true
	}
	if a.K == KError || b.K == KError {
		return true
	}
	if a.K == KVar {
		if occurs(a.V, b) {
			return false
		}
		c.bind(a.V, b)
		return true
	}
	if b.K == KVar {
		if occurs(b.V, a) {
			return false
		}
		c.bind(b.V, a)
		return true
	}
	if a.K == KVoid || b.K == KVoid {
		return a.K == b.K || a.K == KUnit || b.K == KUnit
	}
	if a.K != b.K {
		return false
	}
	switch a.K {
	case KNamed:
		if a.Info != b.Info {
			return false
		}
	case KParam:
		return a.Param == b.Param
	case KFn:
		if len(a.Args) != len(b.Args) {
			return false
		}
		if a.Eff != b.Eff && !a.Poly && !b.Poly {
			return false
		}
		if !c.unify(a.Ret, b.Ret) {
			return false
		}
	}
	if len(a.Args) != len(b.Args) {
		return false
	}
	for i := range a.Args {
		if !c.unify(a.Args[i], b.Args[i]) {
			return false
		}
	}
	return true
}

// tryUnify unifies or leaves no trace.
func (c *checker) tryUnify(a, b *Type) bool {
	s := c.snapshot()
	if c.unify(a, b) {
		return true
	}
	c.rollback(s)
	return false
}

// assignCode checks that a value of type src can be used where dst is
// expected, applying the four subtyping relations. It returns "" on success,
// "E3002" when only function effects are too large, else "E1201".
func (c *checker) assignCode(src, dst *Type) string {
	src, dst = prune(src), prune(dst)
	if src == nil || dst == nil {
		return ""
	}
	if dst.K == KVoid || src.K == KNever || src.K == KError || dst.K == KError {
		return ""
	}
	if src.K == KVar || dst.K == KVar {
		if c.tryUnify(src, dst) {
			return ""
		}
		return "E1201"
	}
	if c.tryUnify(src, dst) {
		return ""
	}
	switch {
	case dst.K == KOption && src.K != KOption:
		return c.assignCode(src, dst.Args[0])
	case dst.K == KNamed && dst.Info.Kind == DInterface && src.K == KNamed && src.Info.implements(dst.Info):
		return ""
	case dst.K == KFn && src.K == KFn && len(dst.Args) == len(src.Args):
		s := c.snapshot()
		ok := true
		for i := range src.Args {
			if !c.unify(src.Args[i], dst.Args[i]) {
				ok = false
				break
			}
		}
		if ok && !c.unify(src.Ret, dst.Ret) {
			ok = false
		}
		if !ok {
			c.rollback(s)
			return "E1201"
		}
		if dst.Poly || dst.Eff.has(src.Eff) {
			return ""
		}
		return "E3002"
	}
	return "E1201"
}

func (c *checker) assignable(src, dst *Type) bool { return c.assignCode(src, dst) == "" }

// join computes a common type of two branches, or nil.
func (c *checker) join(a, b *Type) *Type {
	a, b = prune(a), prune(b)
	if a.K == KNever {
		return b
	}
	if b.K == KNever {
		return a
	}
	if c.assignable(b, a) {
		return a
	}
	if c.assignable(a, b) {
		return b
	}
	if a.K != KOption && b.K == KOption && c.assignable(a, b.Args[0]) {
		return b
	}
	return nil
}
