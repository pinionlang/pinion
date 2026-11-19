package check

import (
	"github.com/pinionlang/pinion/internal/ast"
)

func (c *checker) resolveSig(d *ast.FnDecl, env *typeEnv, owner *TypeInfo, fi *FnInfo) *FnInfo {
	if fi == nil {
		fi = &FnInfo{Name: d.Name, Pub: d.Pub, File: c.mod.file.Path, Decl: d, Pos: d.Pos}
	}
	fi.Owner = owner
	fi.Mut = d.Mut
	for _, tp := range d.TypeParams {
		fi.TParams = append(fi.TParams, &TypeParamInfo{Name: tp.Name, Cons: tp.Constraints})
	}
	env = env.with(fi.TParams)
	for _, p := range d.Params {
		var t *Type
		if p.Type == nil {
			t = errT
		} else {
			t = c.resolveType(p.Type, env)
		}
		fi.Params = append(fi.Params, &ParamInfo{Name: p.Name, Type: t})
	}
	if d.Ret == nil {
		fi.Ret = unitT
		if d.IsInit && owner != nil {
			fi.Ret = owner.selfType()
		}
	} else {
		fi.Ret = c.resolveType(d.Ret, env)
	}
	fi.Eff = effectsOf(d.Effects)
	return fi
}

func (c *checker) resolveDecls(m *module) {
	c.mod = m
	for _, d := range m.file.Decls {
		switch d := d.(type) {
		case *ast.AliasDecl:
			c.resolveAlias(m.types[d.Name])
		}
	}
	for _, d := range m.file.Decls {
		switch d := d.(type) {
		case *ast.TypeDecl:
			ti := m.types[d.Name]
			env := (&typeEnv{self: ti}).with(ti.Params)
			for _, f := range d.Fields {
				ti.Fields = append(ti.Fields, &FieldInfo{Name: f.Name, Type: c.resolveType(f.Type, env), Pub: f.Pub,
					Mut: f.Mut, HasDefault: f.Default != nil, Pos: f.Pos, Decl: f})
			}
			for _, md := range d.Methods {
				ti.Methods[md.Name] = c.resolveSig(md, env, ti, nil)
			}
			if d.Init != nil {
				ti.Init = c.resolveSig(d.Init, env, ti, nil)
			}
			for _, it := range d.Implements {
				t := prune(c.resolveType(it, env))
				if t.K == KNamed && t.Info.Kind == DInterface {
					ti.Impls = append(ti.Impls, t.Info)
				} else if t.K != KError {
					c.errorf("E1201", it.GetPos(), "%s is not an interface", typeString(t))
				}
			}
		case *ast.EnumDecl:
			ti := m.types[d.Name]
			env := (&typeEnv{self: ti}).with(ti.Params)
			for _, v := range d.Variants {
				vi := &VariantInfo{Name: v.Name, Owner: ti, Pos: v.Pos}
				for _, f := range v.Fields {
					vi.Fields = append(vi.Fields, &FieldInfo{Name: f.Name, Type: c.resolveType(f.Type, env), Pub: true, Pos: f.Pos})
				}
				ti.Variants = append(ti.Variants, vi)
			}
		case *ast.InterfaceDecl:
			ti := m.types[d.Name]
			env := (&typeEnv{self: ti}).with(ti.Params)
			for _, md := range d.Methods {
				fi := c.resolveSig(md, env, ti, nil)
				fi.Pub = true
				ti.Methods[md.Name] = fi
			}
		case *ast.FnDecl:
			c.resolveSig(d, &typeEnv{}, nil, m.fns[d.Name])
		case *ast.LetDecl:
			li := m.lets[d.Name]
			if d.Type != nil {
				li.t = c.resolveType(d.Type, &typeEnv{})
			}
		}
	}
	// link interface implementations
	for _, ti := range m.types {
		for _, iface := range ti.Impls {
			for name, im := range iface.Methods {
				if mm := ti.Methods[name]; mm != nil {
					mm.ImplOf = im
				}
			}
		}
	}
	c.mod = nil
}

// ---------- declaration-level checks ----------

func (c *checker) declChecks(m *module) {
	c.mod = m
	defer func() { c.mod = nil }()
	for _, d := range m.file.Decls {
		switch d := d.(type) {
		case *ast.TypeDecl:
			ti := m.types[d.Name]
			if ti.Kind == DData {
				c.checkDataDecl(ti, d)
			}
			c.checkImpls(ti, d)
		case *ast.EnumDecl:
			ti := m.types[d.Name]
			if len(ti.Params) > 0 {
				for _, v := range d.Variants {
					for _, f := range v.Fields {
						c.checkPolyRecursion(ti, f.Type)
					}
				}
			}
		}
	}
}

func (c *checker) checkDataDecl(ti *TypeInfo, d *ast.TypeDecl) {
	for i, f := range d.Fields {
		fi := ti.Fields[i]
		if f.Mut {
			c.errorf("E1405", f.Pos, "data field %s cannot be mut; data values are immutable", f.Name)
		}
		if len(ti.Params) > 0 && c.checkPolyRecursion(ti, f.Type) {
			continue
		}
		if !c.has(fi.Type, PFrozen) {
			c.errorf("E1401", f.Pos, "data field %s has type %s, which is not deeply immutable", f.Name, typeString(fi.Type))
		}
	}
	for _, md := range d.Methods {
		if md.Mut {
			c.errorf("E1405", md.Pos, "data cannot have mut fn %s", md.Name)
		}
	}
	if c.directCycle(ti, ti, map[*TypeInfo]bool{}) {
		c.errorf("E1406", d.Pos, "data %s contains itself directly and can never be constructed; use T?, List or an enum", d.Name)
	}
	if d.Init != nil {
		in := ti.Init
		ok := len(in.Params) == len(ti.Fields)
		if ok {
			for i, p := range in.Params {
				if p.Name != ti.Fields[i].Name || !c.tryUnify(p.Type, ti.Fields[i].Type) {
					ok = false
					break
				}
			}
		}
		if !ok {
			c.errorf("E1503", d.Init.Pos, "init parameters of data %s must match its stored fields exactly (name, type, order); use a named factory fn for other inputs", d.Name)
			d.Init.Body = nil // body not checked further
		}
		if in.Eff != 0 {
			c.errorf("E3006", d.Init.Pos, "init of data %s cannot have effects (%s)", d.Name, in.Eff)
		}
	}
}

// directCycle reports whether data `start` reaches `target` through required
// (non-optional, non-collection) data fields.
func (c *checker) directCycle(start, target *TypeInfo, seen map[*TypeInfo]bool) bool {
	if seen[start] {
		return false
	}
	seen[start] = true
	for _, f := range start.Fields {
		t := prune(f.Type)
		if t.K != KNamed || t.Info.Kind != DData {
			continue
		}
		if t.Info == target || c.directCycle(t.Info, target, seen) {
			return true
		}
	}
	return false
}

// checkPolyRecursion reports E1407 when a field type mentions the generic
// type itself with arguments other than its own parameters.
func (c *checker) checkPolyRecursion(ti *TypeInfo, te ast.TypeExpr) bool {
	bad := false
	var walk func(te ast.TypeExpr)
	walk = func(te ast.TypeExpr) {
		switch t := te.(type) {
		case *ast.NamedType:
			if t.Qualifier == "" && (t.Name == ti.Name || t.Name == "Self") && len(t.Args) > 0 {
				for i, a := range t.Args {
					an, ok := a.(*ast.NamedType)
					if !ok || i >= len(ti.Params) || an.Name != ti.Params[i].Name || len(an.Args) > 0 || an.Qualifier != "" {
						bad = true
					}
				}
			}
			for _, a := range t.Args {
				walk(a)
			}
		case *ast.OptionalType:
			walk(t.Elem)
		case *ast.TupleType:
			for _, e := range t.Elems {
				walk(e)
			}
		case *ast.FnType:
			for _, e := range t.Params {
				walk(e)
			}
			if t.Ret != nil {
				walk(t.Ret)
			}
		}
	}
	walk(te)
	if bad {
		c.errorf("E1407", te.GetPos(), "recursive use of %s changes its type arguments (polymorphic recursion is not allowed in v0)", ti.Name)
	}
	return bad
}

func (c *checker) checkImpls(ti *TypeInfo, d *ast.TypeDecl) {
	for _, iface := range ti.Impls {
		for _, name := range sortedKeys(iface.Methods) {
			im := iface.Methods[name]
			mm := ti.Methods[name]
			if mm == nil {
				c.errorf("E1205", d.Pos, "%s does not implement %s.%s", ti.Name, iface.Name, name)
				continue
			}
			same := len(mm.Params) == len(im.Params) && c.tryUnify(mm.Ret, im.Ret)
			if same {
				for i := range mm.Params {
					if !c.tryUnify(mm.Params[i].Type, im.Params[i].Type) {
						same = false
						break
					}
				}
			}
			if !same {
				c.errorf("E1205", mm.Pos, "%s.%s: signature differs from interface %s (parameter and return types must match exactly)", ti.Name, name, iface.Name)
			}
			if !im.Eff.has(mm.Eff) {
				c.errorf("E3003", mm.Pos, "%s.%s: effects %s exceed those declared by interface %s (%s)", ti.Name, name, mm.Eff, iface.Name, im.Eff)
			}
			if mm.Mut && !im.Mut {
				c.errorf("E3004", mm.Pos, "%s.%s is mut fn but interface %s declares it without mut", ti.Name, name, iface.Name)
			}
		}
	}
}
