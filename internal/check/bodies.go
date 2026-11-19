package check

import (
	"github.com/pinionlang/pinion/internal/ast"
	"github.com/pinionlang/pinion/internal/diag"
)

func effectFix(missing Effects) diag.Fix {
	return diag.Fix{Kind: diag.FixReview, Title: "declare " + missing.String() + " on the enclosing function"}
}

func (c *checker) checkBodies(m *module) {
	c.mod = m
	defer func() { c.mod = nil }()
	for _, d := range m.file.Decls {
		switch d := d.(type) {
		case *ast.FnDecl:
			c.checkFn(m.fns[d.Name], nil)
		case *ast.TypeDecl:
			ti := m.types[d.Name]
			for i, f := range d.Fields {
				if f.Default == nil {
					continue
				}
				c.fn = &fnCtx{effCode: "E3005"}
				c.sc = nil
				c.push()
				c.check(f.Default, ti.Fields[i].Type)
				c.pop()
				c.fn = nil
			}
			for _, md := range d.Methods {
				c.checkFn(ti.Methods[md.Name], ti)
			}
			if d.Init != nil {
				c.checkFn(ti.Init, ti)
			}
		case *ast.LetDecl:
			c.letType(m.lets[d.Name])
		case *ast.TestDecl:
			c.checkTest(d)
		}
	}
}

func (c *checker) checkFn(fi *FnInfo, owner *TypeInfo) {
	d := fi.Decl
	if d == nil || d.Body == nil {
		return
	}
	ctx := &fnCtx{ret: fi.Ret, declared: fi.Eff, owner: owner, mutFn: d.Mut, isInit: d.IsInit, infer: d.Script}
	if owner != nil {
		ctx.selfT = owner.selfType()
		ctx.method = !d.IsInit
	}
	if owner != nil && owner.Kind == DData && d.IsInit && fi.Eff != 0 {
		ctx.allowAll = true // E3006 already reported
	}
	c.fn = ctx
	c.sc = nil
	c.push()
	if ctx.method {
		b := c.declare("self", ctx.selfT, false)
		b.self = true
	}
	for _, p := range fi.Params {
		c.declare(p.Name, p.Type, false)
	}
	exp := fi.Ret
	if prune(exp).K == KUnit {
		exp = voidT
	}
	c.block(d.Body, exp)
	c.pop()
	c.fn = nil
	if d.Script {
		// script mode: the implicit main declares exactly what it uses
		fi.Eff = ctx.used
		d.Effects = nil
		for _, n := range ctx.used.names() {
			d.Effects = append(d.Effects, ast.Effect{Pos: d.Pos, Name: n})
		}
		return
	}
	if !ctx.hadError {
		if unused := fi.Eff &^ ctx.used; unused != 0 {
			c.warnUnused(unused, d.Pos, !fi.Pub)
		}
	}
}

func (c *checker) warnUnused(unused Effects, pos ast.Pos, private bool) {
	kind := diag.FixReview
	if private {
		kind = diag.FixAuto
	}
	c.diags.Add(diag.Diagnostic{Code: "W3002", Severity: diag.Warning, Pos: pos,
		Msg:   "effect " + unused.String() + " is declared but never used",
		Fixes: []diag.Fix{{Kind: kind, Title: "remove " + unused.String()}}})
}

func (c *checker) checkTest(d *ast.TestDecl) {
	ctx := &fnCtx{ret: unitT, declared: effectsOf(d.Effects), allowAll: true}
	c.fn = ctx
	c.sc = nil
	c.push()
	if d.Body != nil {
		c.block(d.Body, voidT)
	}
	c.pop()
	c.fn = nil
	if !ctx.hadError {
		if unused := ctx.declared &^ ctx.used; unused != 0 {
			c.warnUnused(unused, d.Pos, true)
		}
	}
}

func (c *checker) letType(li *letInfo) *Type {
	if li.done || li.checking {
		if li.t == nil {
			return errT
		}
		return li.t
	}
	li.checking = true
	savedFn, savedSc, savedMod := c.fn, c.sc, c.mod
	c.fn = &fnCtx{}
	c.sc = nil
	c.mod = li.mod
	c.push()
	var t *Type
	if li.t != nil {
		c.check(li.decl.Value, li.t)
		t = li.t
	} else {
		t = c.expr(li.decl.Value, nil)
	}
	c.fn, c.sc, c.mod = savedFn, savedSc, savedMod
	li.t = t
	li.done = true
	if !li.decl.Var && !c.has(t, PFrozen) {
		c.diags.Errorf("E1403", li.decl.Pos, "top-level let %s has type %s, which is not deeply immutable", li.decl.Name, typeString(t))
	}
	return t
}
