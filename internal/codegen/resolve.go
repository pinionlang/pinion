package codegen

import (
	"fmt"
	"strings"

	"github.com/pinionlang/pinion/internal/ast"
)

var builtinStaticTypes = map[string]bool{"Int": true, "Float": true, "Dec": true, "Bytes": true, "Str": true}

func (g *fnGen) ident(e *ast.Ident) string {
	if v, ok := g.local(e.Name); ok {
		return v
	}
	switch e.Name {
	case "self":
		return "self"
	case "None":
		return "rt.None"
	case "Ok":
		return `rt.MkFunc(1, "Ok", func(a []rt.Value) rt.Value { return rt.Ok(a[0]) })`
	case "Err":
		return `rt.MkFunc(1, "Err", func(a []rt.Value) rt.Value { return rt.Err(a[0]) })`
	case "Some":
		return `rt.MkFunc(1, "Some", func(a []rt.Value) rt.Value { return a[0] })`
	case "print":
		return `rt.MkFunc(1, "print", func(a []rt.Value) rt.Value { return rt.Print(a[0]) })`
	case "debug":
		return `rt.MkFunc(1, "debug", func(a []rt.Value) rt.Value { return rt.DebugF(a[0]) })`
	}
	return g.moduleValue(g.m, e.Name, e)
}

// moduleValue is a reference to a top-level name of module m used as a value.
func (g *fnGen) moduleValue(m *module, name string, at ast.Expr) string {
	if fd := m.fns[name]; fd != nil {
		var ps []string
		for i := range fd.Params {
			ps = append(ps, fmt.Sprintf("a[%d]", i))
		}
		return fmt.Sprintf("rt.MkFunc(%d, %q, func(a []rt.Value) rt.Value { return %s(%s) })", len(fd.Params), name, m.fnName(name), strings.Join(ps, ", "))
	}
	if m.lets[name] != nil {
		return m.globalVar(name) + ".Get()"
	}
	if m.types[name] != nil {
		return fmt.Sprintf("rt.MkFunc(-1, %q, func(a []rt.Value) rt.Value { return rt.Construct(%s, nil, a) })", name, m.typeVar(name))
	}
	fail(at.GetPos(), "unknown name %s", name)
	return ""
}

// enumOf resolves an expression naming an enum type: (module, decl) or builtin name.
func (g *fnGen) enumOf(x ast.Expr) (*module, *ast.EnumDecl, string) {
	switch x := x.(type) {
	case *ast.Ident:
		if g.lookup(x.Name) != nil {
			return nil, nil, ""
		}
		if d := g.m.enums[x.Name]; d != nil {
			return g.m, d, ""
		}
		if a := g.m.aliases[x.Name]; a != nil {
			if nt, ok := a.Type.(*ast.NamedType); ok && nt.Qualifier == "" {
				if d := g.m.enums[nt.Name]; d != nil {
					return g.m, d, ""
				}
			}
		}
		if builtinEnumNames[x.Name] {
			return nil, nil, x.Name
		}
	case *ast.MemberExpr:
		if m := g.importedModule(x.X); m != nil {
			if d := m.enums[x.Name]; d != nil {
				return m, d, ""
			}
		}
	}
	return nil, nil, ""
}

// importedModule returns the user module an identifier refers to (import alias).
func (g *fnGen) importedModule(x ast.Expr) *module {
	id, ok := x.(*ast.Ident)
	if !ok || g.lookup(id.Name) != nil {
		return nil
	}
	path, ok := g.m.imports[id.Name]
	if !ok {
		return nil
	}
	return g.p.mods[path]
}

func (g *fnGen) stdModule(x ast.Expr) string {
	id, ok := x.(*ast.Ident)
	if !ok || g.lookup(id.Name) != nil {
		return ""
	}
	path, ok := g.m.imports[id.Name]
	if !ok || !strings.HasPrefix(path, "std/") {
		return ""
	}
	return strings.TrimPrefix(path, "std/")
}

func variantRef(m *module, d *ast.EnumDecl, builtin, name string) (string, *ast.Variant) {
	if builtin != "" {
		return fmt.Sprintf("rt.BuiltinVariant(%q, %q)", builtin, name), nil
	}
	for _, v := range d.Variants {
		if v.Name == name {
			return m.variantVar(d.Name, name), v
		}
	}
	return "", nil
}

func (g *fnGen) member(e *ast.MemberExpr) string {
	if m, d, b := g.enumOf(e.X); d != nil || b != "" {
		ref, v := variantRef(m, d, b, e.Name)
		if ref == "" {
			fail(e.Pos, "unknown variant %s", e.Name)
		}
		if v != nil && len(v.Fields) == 0 {
			return ref + ".Unit()"
		}
		return "rt.VariantFunc(" + ref + ")"
	}
	if m := g.importedModule(e.X); m != nil {
		return g.moduleValue(m, e.Name, e)
	}
	if s := g.stdModule(e.X); s != "" {
		return fmt.Sprintf("rt.StdFunc(%q, %q)", s, e.Name)
	}
	x := g.expr(e.X)
	return fmt.Sprintf("rt.GetField(%s, %q)", x, e.Name)
}

func (g *fnGen) call(e *ast.CallExpr) string {
	switch fn := e.Fn.(type) {
	case *ast.Ident:
		if _, ok := g.local(fn.Name); !ok {
			if s, ok := g.preludeCall(fn.Name, e); ok {
				return s
			}
			if fd := g.m.fns[fn.Name]; fd != nil {
				return g.directCall(g.m, fd, e.Args)
			}
			if g.m.types[fn.Name] != nil {
				ns, vs := g.args(e.Args)
				return fmt.Sprintf("rt.Construct(%s, %s, %s)", g.m.typeVar(fn.Name), ns, vs)
			}
			if a := g.m.aliases[fn.Name]; a != nil {
				if nt, ok := a.Type.(*ast.NamedType); ok {
					tm := g.m
					if nt.Qualifier != "" {
						tm = g.p.mods[g.m.imports[nt.Qualifier]]
					}
					if tm != nil && tm.types[nt.Name] != nil {
						ns, vs := g.args(e.Args)
						return fmt.Sprintf("rt.Construct(%s, %s, %s)", tm.typeVar(nt.Name), ns, vs)
					}
				}
			}
		}
	case *ast.MemberExpr:
		if m, d, b := g.enumOf(fn.X); d != nil || b != "" {
			ref, _ := variantRef(m, d, b, fn.Name)
			if ref == "" {
				fail(fn.Pos, "unknown variant %s", fn.Name)
			}
			ns, vs := g.args(e.Args)
			return fmt.Sprintf("rt.MkEnum(%s, %s, %s)", ref, ns, vs)
		}
		if m := g.importedModule(fn.X); m != nil {
			if fd := m.fns[fn.Name]; fd != nil {
				return g.directCall(m, fd, e.Args)
			}
			if m.types[fn.Name] != nil {
				ns, vs := g.args(e.Args)
				return fmt.Sprintf("rt.Construct(%s, %s, %s)", m.typeVar(fn.Name), ns, vs)
			}
			fail(fn.Pos, "module %s has no function %s", m.path, fn.Name)
		}
		if s := g.stdModule(fn.X); s != "" {
			if s == "json" && fn.Name == "decode" {
				t := "rt.TAnyT"
				if len(e.TypeArgs) > 0 {
					t = g.p.typeDesc(g.m, g.owner, nil, e.TypeArgs[0])
				}
				_, vs := g.argList(e.Args)
				return fmt.Sprintf("rt.JSONDecode(%s, %s)", t, strings.Join(vs, ", "))
			}
			ns, vs := g.argList(e.Args)
			return fmt.Sprintf("rt.Std(%q, %q, %s%s)", s, fn.Name, ns, joinPre(vs))
		}
		if id, ok := fn.X.(*ast.Ident); ok && g.lookup(id.Name) == nil && builtinStaticTypes[id.Name] && g.m.types[id.Name] == nil {
			_, vs := g.argList(e.Args)
			return fmt.Sprintf("rt.StaticCall(%q, %q%s)", id.Name, fn.Name, joinPre(vs))
		}
		recv := g.expr(fn.X)
		s, code := g.capture(func() string {
			ns, vs := g.argList(e.Args)
			return fmt.Sprintf("%s%s", ns, joinPre(vs))
		})
		if code != "" && !isConst(recv) {
			recv = g.tmp(recv)
		}
		g.buf.WriteString(code)
		g.setPos(e.Pos)
		if fn.OnOption {
			return fmt.Sprintf("rt.OptM(%s, %q, %s)", recv, fn.Name, s)
		}
		if len(e.TypeArgs) > 0 {
			var ts []string
			for _, ta := range e.TypeArgs {
				ts = append(ts, g.p.typeDesc(g.m, g.owner, nil, ta))
			}
			return fmt.Sprintf("rt.MT(%s, %q, []*rt.Type{%s}, %s)", recv, fn.Name, strings.Join(ts, ", "), s)
		}
		return fmt.Sprintf("rt.M(%s, %q, %s)", recv, fn.Name, s)
	}
	f := g.expr(e.Fn)
	s, code := g.capture(func() string {
		_, vs := g.argList(e.Args)
		return joinPre(vs)
	})
	if code != "" && !isConst(f) {
		f = g.tmp(f)
	}
	g.buf.WriteString(code)
	return fmt.Sprintf("rt.Call(%s%s)", f, s)
}

func joinPre(vs []string) string {
	if len(vs) == 0 {
		return ""
	}
	return ", " + strings.Join(vs, ", ")
}

func (g *fnGen) directCall(m *module, fd *ast.FnDecl, as []*ast.Arg) string {
	_, vals := g.argList(as)
	named := false
	for _, a := range as {
		if a.Name != "" {
			named = true
		}
	}
	out := make([]string, len(fd.Params))
	if named {
		for i := range vals {
			if !isConst(vals[i]) {
				vals[i] = g.tmp(vals[i])
			}
		}
	}
	for i, a := range as {
		idx := i
		if a.Name != "" {
			idx = -1
			for j, p := range fd.Params {
				if p.Name == a.Name {
					idx = j
				}
			}
			if idx < 0 {
				fail(a.Pos, "%s has no parameter %s", fd.Name, a.Name)
			}
		}
		if idx >= len(out) {
			fail(a.Pos, "too many arguments to %s", fd.Name)
		}
		out[idx] = vals[i]
	}
	for i := range out {
		if out[i] == "" {
			fail(fd.Pos, "missing argument %s in call to %s", fd.Params[i].Name, fd.Name)
		}
	}
	return m.fnName(fd.Name) + "(" + strings.Join(out, ", ") + ")"
}

func (g *fnGen) preludeCall(name string, e *ast.CallExpr) (string, bool) {
	switch name {
	case "print":
		_, vs := g.argList(e.Args)
		return "rt.Print(" + strings.Join(vs, ", ") + ")", true
	case "debug":
		_, vs := g.argList(e.Args)
		return "rt.DebugF(" + strings.Join(vs, ", ") + ")", true
	case "todo":
		_, vs := g.argList(e.Args)
		return "rt.Todo(" + strings.Join(vs, ", ") + ")", true
	case "unreachable":
		_, vs := g.argList(e.Args)
		return "rt.Unreachable(" + strings.Join(vs, ", ") + ")", true
	case "Ok", "Err":
		_, vs := g.argList(e.Args)
		if len(vs) == 0 {
			vs = []string{"rt.UnitV"}
		}
		return "rt." + name + "(" + vs[0] + ")", true
	case "Some":
		_, vs := g.argList(e.Args)
		return vs[0], true
	case "Set":
		if g.m.types["Set"] != nil {
			return "", false
		}
		_, vs := g.argList(e.Args)
		if len(vs) == 0 {
			return "rt.MkSet(rt.ListLit())", true
		}
		return "rt.MkSet(" + vs[0] + ")", true
	case "MutList", "MutMap", "MutSet":
		if g.m.types[name] != nil {
			return "", false
		}
		return "rt.New" + name + "()", true
	case "Duration":
		ns, vs := g.args(e.Args)
		return "rt.MkDuration(" + ns + ", " + vs + ")", true
	}
	return "", false
}
