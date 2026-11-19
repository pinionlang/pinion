package check

// Prop is one of the six type properties (DESIGN 3.3).
type Prop int

const (
	PFrozen Prop = iota
	PEq
	POrd
	PHash
	PEnc
	PDec
)

var propNames = [...]string{"Frozen", "Eq", "Ord", "Hash", "Encode", "Decode"}

func (p Prop) String() string { return propNames[p] }

func (c *checker) has(t *Type, p Prop) bool {
	return c.hasProp(t, p, map[*TypeInfo]bool{})
}

func (c *checker) hasProp(t *Type, p Prop, visiting map[*TypeInfo]bool) bool {
	t = prune(t)
	if t == nil {
		return true
	}
	all := func(ts ...*Type) bool {
		for _, x := range ts {
			if !c.hasProp(x, p, visiting) {
				return false
			}
		}
		return true
	}
	switch t.K {
	case KError, KVar, KNever, KVoid:
		return true
	case KInt, KBool, KStr, KFloat, KDec, KBytes, KInstant, KDuration:
		return true
	case KUnit, KRange:
		return p == PFrozen || p == PEq || p == PHash
	case KOption:
		return p != POrd && all(t.Args...)
	case KList:
		return all(t.Args...)
	case KMap:
		switch p {
		case PFrozen:
			return all(t.Args...)
		case PEq, PHash:
			return all(t.Args[1])
		case PEnc, PDec:
			k := prune(t.Args[0])
			return (k.K == KStr || k.K == KError || k.K == KVar) && all(t.Args[1])
		}
		return false
	case KTuple:
		return p != PEnc && p != PDec && all(t.Args...)
	case KSet:
		switch p {
		case PEq, PHash:
			return true
		case POrd:
			return false
		}
		return all(t.Args...)
	case KResult:
		return (p == PFrozen || p == PEq || p == PHash) && all(t.Args...)
	case KParam:
		switch p {
		case PFrozen:
			return t.Param.Frozen
		case PEq, POrd, PHash:
			return t.Param.hasCons(p.String())
		}
		return false
	case KNamed:
		ti := t.Info
		switch ti.Kind {
		case DClass, DInterface:
			return false
		case DAlias:
			return c.hasProp(ti.Alias, p, visiting)
		}
		if p == POrd {
			return false
		}
		if ti.Kind == DData && p == PFrozen {
			return true
		}
		if ti.Kind == DData && p == PDec && (ti.Init != nil || ti.hasPrivateFields()) {
			return false
		}
		if visiting[ti] {
			return true // greatest fixed point
		}
		visiting[ti] = true
		defer delete(visiting, ti)
		m := map[*TypeParamInfo]*Type{}
		for i, tp := range ti.Params {
			if i < len(t.Args) {
				m[tp] = t.Args[i]
			}
		}
		if ti.Kind == DData {
			for _, f := range ti.Fields {
				if !c.hasProp(subst(f.Type, m), p, visiting) {
					return false
				}
			}
			return true
		}
		for _, v := range ti.Variants {
			for _, f := range v.Fields {
				if !c.hasProp(subst(f.Type, m), p, visiting) {
					return false
				}
			}
		}
		return true
	}
	// MutList, MutMap, MutSet, Fn, Handler
	return false
}

func isDisplay(t *Type) bool {
	t = prune(t)
	switch t.K {
	case KInt, KFloat, KDec, KBool, KStr, KInstant, KDuration, KError, KNever, KVar:
		return true
	}
	return false
}
