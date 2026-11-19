package check

import (
	"github.com/pinionlang/pinion/internal/ast"
	"github.com/pinionlang/pinion/internal/token"
)

func (c *checker) inferArgs(args []*ast.Arg) {
	for _, a := range args {
		c.expr(a.Value, nil)
	}
}

func (c *checker) call(e *ast.CallExpr, exp *Type) *Type {
	switch fn := e.Fn.(type) {
	case *ast.Ident:
		name := fn.Name
		if b := c.lookup(name); b != nil {
			return c.callValue(b.t, e)
		}
		switch name {
		case "Ok", "Err", "Some":
			return c.callWrap(name, e, exp)
		}
		if fi := c.mod.fns[name]; fi != nil {
			return c.callFn(fi, nil, e, exp)
		}
		if ti := c.mod.types[name]; ti != nil {
			return c.construct(ti, e, exp)
		}
		if fi := c.std.prelude[name]; fi != nil {
			return c.callFn(fi, nil, e, exp)
		}
		if ti := c.std.types[name]; ti != nil {
			return c.construct(ti, e, exp)
		}
		if li := c.mod.lets[name]; li != nil {
			return c.callValue(c.letType(li), e)
		}
		c.ident(fn, nil) // reports E1101 (with hints)
		c.inferArgs(e.Args)
		return errT
	case *ast.MemberExpr:
		if ti := c.moduleType(fn.X); ti != nil {
			if ti.Kind == DEnum {
				return c.callVariant(ti, fn, e, exp)
			}
			c.errorf("E1101", fn.Pos, "%s has no function %s", ti.Name, fn.Name)
			c.inferArgs(e.Args)
			return errT
		}
		if id, ok := fn.X.(*ast.Ident); ok && c.lookup(id.Name) == nil && id.Name != "self" {
			if r := c.mod.imports[id.Name]; r != nil {
				return c.callModuleMember(r, fn, e, exp)
			}
			if ti := c.lookupTypeQuiet(id.Name); ti != nil {
				if ti.Kind == DEnum {
					return c.callVariant(ti, fn, e, exp)
				}
				c.errorf("E1101", fn.Pos, "%s has no function %s; user types have no static functions (use a factory fn)", ti.Name, fn.Name)
				c.inferArgs(e.Args)
				return errT
			}
			if st := c.std.statics[id.Name]; st != nil {
				fi := st[fn.Name]
				if fi == nil {
					c.errorf("E1101", fn.Pos, "%s has no function %s", id.Name, fn.Name)
					c.inferArgs(e.Args)
					return errT
				}
				return c.callFn(fi, nil, e, exp)
			}
			if c.mod.fns[id.Name] == nil && c.mod.lets[id.Name] == nil && c.std.prelude[id.Name] == nil {
				c.ident(id, nil)
				c.inferArgs(e.Args)
				return errT
			}
		}
		recv := c.expr(fn.X, nil)
		return c.methodCall(recv, fn, e, exp)
	}
	t := c.expr(e.Fn, nil)
	return c.callValue(t, e)
}

func (c *checker) lookupTypeQuiet(name string) *TypeInfo {
	if ti := c.mod.types[name]; ti != nil {
		return ti
	}
	return c.std.types[name]
}

func (c *checker) callModuleMember(r *importRef, fn *ast.MemberExpr, e *ast.CallExpr, exp *Type) *Type {
	name := fn.Name
	if r.std != nil {
		if fi := r.std.fns[name]; fi != nil {
			return c.callFn(fi, nil, e, exp)
		}
		if ti := r.std.types[name]; ti != nil {
			return c.construct(ti, e, exp)
		}
		c.errorf("E1101", fn.Pos, "std/%s has no member %s", r.std.name, name)
		c.inferArgs(e.Args)
		return errT
	}
	m := r.mod
	if fi := m.fns[name]; fi != nil {
		if !fi.Pub {
			c.errorf("E1602", fn.Pos, "%s.%s is private to module %s", fn.X.(*ast.Ident).Name, name, m.path)
		}
		return c.callFn(fi, nil, e, exp)
	}
	if ti := m.types[name]; ti != nil {
		if !ti.Pub {
			c.errorf("E1602", fn.Pos, "type %s is private to module %s", name, m.path)
		}
		return c.construct(ti, e, exp)
	}
	if li := m.lets[name]; li != nil {
		if !li.decl.Pub {
			c.errorf("E1602", fn.Pos, "%s is private to module %s", name, m.path)
		}
		return c.callValue(c.letType(li), e)
	}
	c.errorf("E1101", fn.Pos, "module %s has no member %s", m.path, name)
	c.inferArgs(e.Args)
	return errT
}

// callFn checks a call of a known signature. recv maps receiver type params.
func (c *checker) callFn(fi *FnInfo, recv map[*TypeParamInfo]*Type, e *ast.CallExpr, exp *Type) *Type {
	m := map[*TypeParamInfo]*Type{}
	for k, v := range recv {
		m[k] = v
	}
	for i, tp := range fi.TParams {
		if i < len(e.TypeArgs) {
			m[tp] = c.resolveType(e.TypeArgs[i], c.typeEnvHere())
		} else {
			m[tp] = c.fresh()
		}
	}
	ret := subst(fi.Ret, m)
	c.preUnify(ret, exp)
	poly := c.matchArgs(fi.Params, m, e.Args, e.Pos)
	c.useEffects(fi.Eff|poly, e.Pos)
	switch fi.Special {
	case "encode":
		if t := m[fi.TParams[0]]; !c.has(t, PEnc) {
			c.errorf("E1502", e.Pos, "%s cannot be encoded to JSON (no Encode property)", typeString(t))
		}
	case "decode":
		if t := m[fi.TParams[0]]; !c.has(t, PDec) {
			c.errorf("E1501", e.Pos, "%s cannot be decoded from JSON (no Decode property: init, private fields or non-decodable fields); decode a public DTO and construct from it", typeString(t))
		}
	}
	return ret
}

func (c *checker) preUnify(ret, exp *Type) {
	if exp == nil || !hasVars(ret) {
		return
	}
	ep := prune(exp)
	if ep.K == KVoid || ep.K == KError || prune(ret).K == KVar {
		return
	}
	c.tryUnify(ret, exp)
}

// matchArgs checks call arguments against parameters, returning the effects
// of callbacks passed to std higher-order parameters.
func (c *checker) matchArgs(params []*ParamInfo, m map[*TypeParamInfo]*Type, args []*ast.Arg, pos token.Pos) Effects {
	assigned := make([]bool, len(params))
	var poly Effects
	next := 0
	for _, a := range args {
		idx := -1
		if a.Name == "" {
			for next < len(params) && assigned[next] {
				next++
			}
			if next < len(params) {
				idx = next
			}
			next++
		} else {
			for i, p := range params {
				if p.Name == a.Name {
					idx = i
					break
				}
			}
		}
		if idx < 0 || assigned[idx] {
			if a.Name != "" {
				c.errorf("E1201", a.Pos, "unexpected argument %s", a.Name)
			} else {
				c.errorf("E1201", a.Value.GetPos(), "too many arguments")
			}
			c.expr(a.Value, nil)
			continue
		}
		assigned[idx] = true
		pt := subst(params[idx].Type, m)
		at := c.check(a.Value, pt)
		if pp := prune(pt); pp.K == KFn && pp.Poly {
			if ap := prune(at); ap.K == KFn {
				poly |= ap.Eff
			}
		}
	}
	for i, p := range params {
		if !assigned[i] && !p.HasDefault {
			c.errorf("E1201", pos, "missing argument %s", p.Name)
		}
	}
	return poly
}

func (c *checker) callValue(t *Type, e *ast.CallExpr) *Type {
	ft := prune(t)
	if ft.K == KError {
		c.inferArgs(e.Args)
		return errT
	}
	if ft.K != KFn {
		c.errorf("E1201", e.Pos, "%s is not callable", typeString(t))
		c.inferArgs(e.Args)
		return errT
	}
	ps := make([]*ParamInfo, len(ft.Args))
	for i, a := range ft.Args {
		ps[i] = &ParamInfo{Type: a}
	}
	poly := c.matchArgs(ps, nil, e.Args, e.Pos)
	eff := ft.Eff
	if ft.Poly {
		eff |= poly
	}
	c.useEffects(eff, e.Pos)
	return ft.Ret
}

func (c *checker) callWrap(name string, e *ast.CallExpr, exp *Type) *Type {
	if len(e.Args) != 1 || e.Args[0].Name != "" {
		c.errorf("E1201", e.Pos, "%s takes exactly one positional argument", name)
		c.inferArgs(e.Args)
		return errT
	}
	arg := e.Args[0].Value
	ep := prune(exp)
	switch name {
	case "Some":
		if ep != nil && ep.K == KOption {
			c.check(arg, ep.Args[0])
			return ep
		}
		return optOf(c.expr(arg, nil))
	case "Ok":
		if ep != nil && ep.K == KResult {
			c.check(arg, ep.Args[0])
			return ep
		}
		return resultOf(c.expr(arg, nil), c.fresh())
	default:
		if ep != nil && ep.K == KResult {
			c.check(arg, ep.Args[1])
			return ep
		}
		return resultOf(c.fresh(), c.expr(arg, nil))
	}
}

func (c *checker) typeArgsFor(params []*TypeParamInfo, targs []ast.TypeExpr) (map[*TypeParamInfo]*Type, []*Type) {
	m := map[*TypeParamInfo]*Type{}
	args := make([]*Type, len(params))
	for i, tp := range params {
		if i < len(targs) {
			args[i] = c.resolveType(targs[i], c.typeEnvHere())
		} else {
			args[i] = c.fresh()
		}
		m[tp] = args[i]
	}
	return m, args
}

func (c *checker) construct(ti *TypeInfo, e *ast.CallExpr, exp *Type) *Type {
	if ti.Kind == DAlias {
		c.resolveAlias(ti)
		if at := prune(ti.Alias); at.K == KNamed {
			return c.construct(at.Info, e, exp)
		}
	}
	if ti.Kind != DData && ti.Kind != DClass {
		c.errorf("E1201", e.Pos, "%s cannot be constructed like this", ti.Name)
		c.inferArgs(e.Args)
		return errT
	}
	if ti.hasPrivateFields() && ti.File != "" && ti.File != c.filePath() {
		c.errorf("E1506", e.Pos, "%s has private fields and cannot be constructed outside its module; use a pub factory fn", ti.Name)
		c.inferArgs(e.Args)
		_, args := c.typeArgsFor(ti.Params, e.TypeArgs)
		if ti.Init != nil {
			return errT
		}
		return named(ti, args...)
	}
	m, args := c.typeArgsFor(ti.Params, e.TypeArgs)
	selfT := named(ti, args...)
	if ti.Init != nil {
		ret := subst(ti.Init.Ret, m)
		c.preUnify(ret, exp)
		poly := c.matchArgs(ti.Init.Params, m, e.Args, e.Pos)
		c.useEffects(ti.Init.Eff|poly, e.Pos)
		return ret
	}
	c.preUnify(selfT, exp)
	ps := make([]*ParamInfo, len(ti.Fields))
	for i, f := range ti.Fields {
		ps[i] = &ParamInfo{Name: f.Name, Type: f.Type, HasDefault: f.HasDefault}
	}
	poly := c.matchArgs(ps, m, e.Args, e.Pos)
	c.useEffects(poly, e.Pos)
	return selfT
}

func (c *checker) callVariant(ti *TypeInfo, fn *ast.MemberExpr, e *ast.CallExpr, exp *Type) *Type {
	v := ti.variant(fn.Name)
	if v == nil {
		c.errorf("E1101", fn.Pos, "enum %s has no variant %s", ti.Name, fn.Name)
		c.inferArgs(e.Args)
		return errT
	}
	m, args := c.typeArgsFor(ti.Params, e.TypeArgs)
	ret := named(ti, args...)
	c.preUnify(ret, exp)
	ps := make([]*ParamInfo, len(v.Fields))
	for i, f := range v.Fields {
		ps[i] = &ParamInfo{Name: f.Name, Type: f.Type}
	}
	c.matchArgs(ps, m, e.Args, e.Pos)
	return ret
}

func (c *checker) fieldType(recv *Type, fi *FieldInfo) *Type {
	r := prune(recv)
	m := map[*TypeParamInfo]*Type{}
	for i, tp := range r.Info.Params {
		if i < len(r.Args) && r.Args[i] != nil {
			m[tp] = r.Args[i]
		} else {
			m[tp] = errT
		}
	}
	return subst(fi.Type, m)
}

func recvSubst(r *Type) map[*TypeParamInfo]*Type {
	m := map[*TypeParamInfo]*Type{}
	for i, tp := range r.Info.Params {
		if i < len(r.Args) && r.Args[i] != nil {
			m[tp] = r.Args[i]
		} else {
			m[tp] = errT
		}
	}
	return m
}

func (c *checker) methodCall(recv *Type, mem *ast.MemberExpr, e *ast.CallExpr, exp *Type) *Type {
	r := prune(recv)
	name := mem.Name
	switch r.K {
	case KError:
		c.inferArgs(e.Args)
		return errT
	case KNamed:
		ti := r.Info
		if ti.Kind == DAlias {
			return c.methodCall(ti.Alias, mem, e, exp)
		}
		if name == "with" && ti.Kind == DData {
			return c.withCall(r, e)
		}
		if fi := ti.Methods[name]; fi != nil {
			if !fi.Pub && fi.ImplOf == nil && ti.File != "" && ti.File != c.filePath() {
				c.errorf("E1602", mem.Pos, "method %s.%s is private to its module", ti.Name, name)
			}
			if fi.Mut && isSelf(c, mem.X) && c.fn != nil && !c.fn.mutFn {
				c.errorf("E3103", e.Pos, "calling mut fn %s on self requires the enclosing method to be mut fn", name)
			}
			return c.callFn(fi, recvSubst(r), e, exp)
		}
		if fi := ti.field(name); fi != nil {
			c.fieldAccess(ti, fi, mem)
			return c.callValue(c.fieldType(r, fi), e)
		}
		c.errorf("E1101", mem.Pos, "%s has no method %s", ti.Name, name)
		c.inferArgs(e.Args)
		return errT
	}
	tbl := c.std.methods[r.K]
	fi := tbl[name]
	if r.K == KOption && fi != nil {
		mem.OnOption = true
	}
	if fi == nil {
		c.errorf("E1101", mem.Pos, "%s has no method %s", typeString(r), name)
		c.inferArgs(e.Args)
		return errT
	}
	m := map[*TypeParamInfo]*Type{}
	for i, tp := range c.std.recvParams[r.K] {
		if i < len(r.Args) {
			m[tp] = r.Args[i]
		}
	}
	if r.K == KMutMap && name == "inc" && !c.unify(r.Args[1], intT) {
		c.errorf("E1201", mem.Pos, "inc needs a MutMap with Int values, got %s", typeString(r))
	}
	return c.callFn(fi, m, e, exp)
}

func (c *checker) fieldAccess(ti *TypeInfo, fi *FieldInfo, mem *ast.MemberExpr) {
	if !fi.Pub && ti.File != "" && ti.File != c.filePath() {
		c.errorf("E1602", mem.Pos, "field %s.%s is private to its module", ti.Name, fi.Name)
	}
}

func (c *checker) withCall(r *Type, e *ast.CallExpr) *Type {
	ti := r.Info
	if ti.hasPrivateFields() && ti.File != "" && ti.File != c.filePath() {
		c.errorf("E1506", e.Pos, "%s has private fields; with cannot be used outside its module", ti.Name)
		c.inferArgs(e.Args)
		return errT
	}
	m := recvSubst(r)
	for _, a := range e.Args {
		fi := ti.field(a.Name)
		if a.Name == "" || fi == nil {
			c.errorf("E1201", a.Pos, "with takes named field arguments of %s", ti.Name)
			c.expr(a.Value, nil)
			continue
		}
		c.check(a.Value, subst(fi.Type, m))
	}
	if ti.Init != nil {
		ret := subst(ti.Init.Ret, m)
		c.useEffects(ti.Init.Eff, e.Pos)
		return ret
	}
	return r
}

// memberValue checks a member expression that is not called.
func (c *checker) memberValue(e *ast.MemberExpr, exp *Type) *Type {
	if ti := c.moduleType(e.X); ti != nil {
		return c.variantValue(ti, e, exp)
	}
	if id, ok := e.X.(*ast.Ident); ok && c.lookup(id.Name) == nil && id.Name != "self" {
		if r := c.mod.imports[id.Name]; r != nil {
			return c.moduleValue(r, e)
		}
		if ti := c.lookupTypeQuiet(id.Name); ti != nil {
			return c.variantValue(ti, e, exp)
		}
		if st := c.std.statics[id.Name]; st != nil {
			if fi := st[e.Name]; fi != nil {
				return c.instantiate(fi, nil)
			}
			c.errorf("E1101", e.Pos, "%s has no function %s", id.Name, e.Name)
			return errT
		}
	}
	r := prune(c.expr(e.X, nil))
	switch r.K {
	case KError:
		return errT
	case KNamed:
		ti := r.Info
		if fi := ti.field(e.Name); fi != nil {
			c.fieldAccess(ti, fi, e)
			return c.fieldType(r, fi)
		}
		if fi := ti.Methods[e.Name]; fi != nil {
			return c.instantiate(fi, recvSubst(r))
		}
		c.errorf("E1101", e.Pos, "%s has no field %s", ti.Name, e.Name)
		return errT
	}
	if fi := c.std.methods[r.K][e.Name]; fi != nil {
		m := map[*TypeParamInfo]*Type{}
		for i, tp := range c.std.recvParams[r.K] {
			if i < len(r.Args) {
				m[tp] = r.Args[i]
			}
		}
		return c.instantiate(fi, m)
	}
	c.errorf("E1101", e.Pos, "%s has no field %s", typeString(r), e.Name)
	return errT
}

func (c *checker) moduleValue(r *importRef, e *ast.MemberExpr) *Type {
	if r.std != nil {
		if fi := r.std.fns[e.Name]; fi != nil {
			return c.instantiate(fi, nil)
		}
		c.errorf("E1101", e.Pos, "std/%s has no member %s", r.std.name, e.Name)
		return errT
	}
	m := r.mod
	if fi := m.fns[e.Name]; fi != nil {
		if !fi.Pub {
			c.errorf("E1602", e.Pos, "%s is private to module %s", e.Name, m.path)
		}
		return c.instantiate(fi, nil)
	}
	if li := m.lets[e.Name]; li != nil {
		if !li.decl.Pub {
			c.errorf("E1602", e.Pos, "%s is private to module %s", e.Name, m.path)
		}
		return c.letType(li)
	}
	c.errorf("E1101", e.Pos, "module %s has no value %s", m.path, e.Name)
	return errT
}

// moduleType resolves `mod.Type` (an imported module's type used as a qualifier).
func (c *checker) moduleType(x ast.Expr) *TypeInfo {
	me, ok := x.(*ast.MemberExpr)
	if !ok {
		return nil
	}
	id, ok := me.X.(*ast.Ident)
	if !ok || c.lookup(id.Name) != nil {
		return nil
	}
	r := c.mod.imports[id.Name]
	if r == nil {
		return nil
	}
	if r.std != nil {
		return r.std.types[me.Name]
	}
	ti := r.mod.types[me.Name]
	if ti != nil && !ti.Pub {
		c.errorf("E1602", me.Pos, "type %s is private to module %s", me.Name, r.mod.path)
	}
	return ti
}

func (c *checker) variantValue(ti *TypeInfo, e *ast.MemberExpr, exp *Type) *Type {
	if ti.Kind != DEnum {
		c.errorf("E1101", e.Pos, "%s has no member %s", ti.Name, e.Name)
		return errT
	}
	v := ti.variant(e.Name)
	if v == nil {
		c.errorf("E1101", e.Pos, "enum %s has no variant %s", ti.Name, e.Name)
		return errT
	}
	m, args := c.typeArgsFor(ti.Params, nil)
	et := named(ti, args...)
	c.preUnify(et, exp)
	if len(v.Fields) == 0 {
		return et
	}
	ps := make([]*Type, len(v.Fields))
	for i, f := range v.Fields {
		ps[i] = subst(f.Type, m)
	}
	return &Type{K: KFn, Args: ps, Ret: et}
}
