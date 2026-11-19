package check

import (
	"strings"

	"github.com/pinionlang/pinion/internal/ast"
	"github.com/pinionlang/pinion/internal/diag"
)

// sp is a simplified pattern for exhaustiveness checking.
type sp struct {
	wild bool
	ctor string
	args []*sp
	alts []*sp
}

var wildSP = &sp{wild: true}

func wilds(n int) []*sp {
	out := make([]*sp, n)
	for i := range out {
		out[i] = wildSP
	}
	return out
}

type ctorInfo struct {
	name string
	args []*Type
}

// ctors returns the constructors of t and whether they are a finite, complete set.
func (c *checker) ctors(t *Type) ([]ctorInfo, bool) {
	t = prune(t)
	switch t.K {
	case KBool:
		return []ctorInfo{{name: "true"}, {name: "false"}}, true
	case KOption:
		return []ctorInfo{{"Some", []*Type{t.Args[0]}}, {name: "None"}}, true
	case KResult:
		return []ctorInfo{{"Ok", []*Type{t.Args[0]}}, {"Err", []*Type{t.Args[1]}}}, true
	case KTuple:
		return []ctorInfo{{"()", t.Args}}, true
	case KUnit:
		return []ctorInfo{{name: "()"}}, true
	case KNamed:
		if t.Info.Kind == DAlias {
			return c.ctors(t.Info.Alias)
		}
		if t.Info.Kind != DEnum {
			return nil, false
		}
		m := recvSubst(t)
		var out []ctorInfo
		for _, v := range t.Info.Variants {
			ci := ctorInfo{name: v.Name}
			for _, f := range v.Fields {
				ci.args = append(ci.args, subst(f.Type, m))
			}
			out = append(out, ci)
		}
		return out, true
	}
	return nil, false
}

func (c *checker) ctorArgTypes(t *Type, name string, n int) []*Type {
	cs, _ := c.ctors(t)
	for _, ci := range cs {
		if ci.name == name && len(ci.args) == n {
			return ci.args
		}
	}
	out := make([]*Type, n)
	for i := range out {
		out[i] = errT
	}
	return out
}

func expandRows(rows [][]*sp) [][]*sp {
	var out [][]*sp
	for _, r := range rows {
		if len(r) > 0 && r[0].alts != nil {
			var sub [][]*sp
			for _, a := range r[0].alts {
				nr := append([]*sp{a}, r[1:]...)
				sub = append(sub, nr)
			}
			out = append(out, expandRows(sub)...)
			continue
		}
		out = append(out, r)
	}
	return out
}

func specialize(rows [][]*sp, name string, n int) [][]*sp {
	var out [][]*sp
	for _, r := range rows {
		h := r[0]
		switch {
		case h.wild:
			out = append(out, append(wilds(n), r[1:]...))
		case h.ctor == name:
			args := h.args
			if len(args) != n {
				args = wilds(n)
			}
			out = append(out, append(append([]*sp{}, args...), r[1:]...))
		}
	}
	return out
}

func (c *checker) useful(rows [][]*sp, q []*sp, tys []*Type) bool {
	if len(q) == 0 {
		return len(rows) == 0
	}
	if q[0].alts != nil {
		for _, a := range q[0].alts {
			if c.useful(rows, append([]*sp{a}, q[1:]...), tys) {
				return true
			}
		}
		return false
	}
	rows = expandRows(rows)
	if !q[0].wild {
		n := len(q[0].args)
		argTys := c.ctorArgTypes(tys[0], q[0].ctor, n)
		return c.useful(specialize(rows, q[0].ctor, n), append(append([]*sp{}, q[0].args...), q[1:]...),
			append(append([]*Type{}, argTys...), tys[1:]...))
	}
	cs, complete := c.ctors(tys[0])
	used := map[string]bool{}
	for _, r := range rows {
		if !r[0].wild {
			used[r[0].ctor] = true
		}
	}
	all := complete
	for _, ci := range cs {
		if !used[ci.name] {
			all = false
		}
	}
	if all {
		for _, ci := range cs {
			n := len(ci.args)
			if c.useful(specialize(rows, ci.name, n), append(wilds(n), q[1:]...),
				append(append([]*Type{}, ci.args...), tys[1:]...)) {
				return true
			}
		}
		return false
	}
	var def [][]*sp
	for _, r := range rows {
		if r[0].wild {
			def = append(def, r[1:])
		}
	}
	return c.useful(def, q[1:], tys[1:])
}

// ---------- patterns ----------

// pattern checks p against t, collecting bindings.
func (c *checker) pattern(p ast.Pattern, t *Type, binds map[string]*Type) *sp {
	switch p := p.(type) {
	case *ast.WildcardPat:
		return wildSP
	case *ast.BindPat:
		if p.Name != "_" {
			binds[p.Name] = t
		}
		return wildSP
	case *ast.LitPat:
		lt := c.expr(p.Value, t)
		c.assignOrReport(lt, t, p.Pos)
		switch v := p.Value.(type) {
		case *ast.BoolLit:
			if v.Value {
				return &sp{ctor: "true"}
			}
			return &sp{ctor: "false"}
		case *ast.IntLit:
			return &sp{ctor: "lit:" + v.Text}
		case *ast.FloatLit:
			return &sp{ctor: "lit:" + v.Text}
		case *ast.StrLit:
			var b strings.Builder
			for _, part := range v.Parts {
				b.WriteString(part.Text)
			}
			return &sp{ctor: "lit:\"" + b.String()}
		case *ast.UnaryExpr:
			if il, ok := v.X.(*ast.IntLit); ok {
				return &sp{ctor: "lit:-" + il.Text}
			}
		}
		return &sp{ctor: "lit:?"}
	case *ast.TuplePat:
		tt := prune(t)
		if tt.K != KTuple || len(tt.Args) != len(p.Elems) {
			if tt.K != KError {
				c.errorf("E1201", p.Pos, "tuple pattern does not match %s", typeString(t))
			}
			for _, e := range p.Elems {
				c.pattern(e, errT, binds)
			}
			return wildSP
		}
		out := &sp{ctor: "()"}
		for i, e := range p.Elems {
			out.args = append(out.args, c.pattern(e, tt.Args[i], binds))
		}
		return out
	case *ast.OrPat:
		out := &sp{}
		var first map[string]*Type
		bad := false
		for i, a := range p.Alts {
			b := map[string]*Type{}
			out.alts = append(out.alts, c.pattern(a, t, b))
			if i == 0 {
				first = b
				continue
			}
			if len(b) != len(first) {
				bad = true
			}
			for k, v := range b {
				f, ok := first[k]
				if !ok || !c.tryUnify(f, v) {
					bad = true
				}
			}
		}
		if bad {
			c.errorf("E2003", p.Pos, "both sides of | must bind the same names with the same types")
		}
		for k, v := range first {
			binds[k] = v
		}
		return out
	case *ast.VariantPat:
		return c.variantPat(p, t, binds)
	}
	return wildSP
}

func (c *checker) variantPat(p *ast.VariantPat, t *Type, binds map[string]*Type) *sp {
	tt := prune(t)
	var fields []*Type
	found := false
	switch tt.K {
	case KError:
		for _, a := range p.Args {
			c.pattern(a, errT, binds)
		}
		return wildSP
	case KOption:
		switch p.Name {
		case "Some":
			fields, found = []*Type{tt.Args[0]}, true
		case "None":
			found = true
		}
	case KResult:
		switch p.Name {
		case "Ok":
			fields, found = []*Type{tt.Args[0]}, true
		case "Err":
			fields, found = []*Type{tt.Args[1]}, true
		}
	case KNamed:
		ti := tt.Info
		if ti.Kind == DAlias {
			return c.variantPat(p, ti.Alias, binds)
		}
		if ti.Kind == DEnum && (p.Qualifier == "" || p.Qualifier == ti.Name) {
			if v := ti.variant(p.Name); v != nil {
				found = true
				m := recvSubst(tt)
				for _, f := range v.Fields {
					fields = append(fields, subst(f.Type, m))
				}
			}
		}
	}
	if !found {
		c.errorf("E1101", p.Pos, "%s is not a variant of %s", p.Name, typeString(t))
		for _, a := range p.Args {
			c.pattern(a, errT, binds)
		}
		return wildSP
	}
	if len(p.Args) != len(fields) {
		if !(len(p.Args) == 0 && !p.HasParens && len(fields) == 0) {
			c.errorf("E1201", p.Pos, "variant %s has %d payload values, pattern has %d", p.Name, len(fields), len(p.Args))
		}
		for _, a := range p.Args {
			c.pattern(a, errT, binds)
		}
		return &sp{ctor: p.Name, args: wilds(len(fields))}
	}
	out := &sp{ctor: p.Name}
	for i, a := range p.Args {
		out.args = append(out.args, c.pattern(a, fields[i], binds))
	}
	return out
}

func (c *checker) match(e *ast.MatchExpr, exp *Type) *Type {
	st := c.expr(e.Subject, nil)
	useExp := exp != nil && prune(exp).K != KVar
	var result *Type
	if exp != nil && !useExp {
		result = exp
	}
	var rows [][]*sp
	patErr := false
	allNever := len(e.Arms) > 0
	for _, arm := range e.Arms {
		c.push()
		binds := map[string]*Type{}
		nb := len(c.diags.Items)
		pat := c.pattern(arm.Pat, st, binds)
		if len(c.diags.Items) > nb {
			patErr = true
		}
		for _, k := range sortedKeys(binds) {
			c.declare(k, binds[k], false)
		}
		if arm.Guard != nil {
			c.check(arm.Guard, boolT)
		} else {
			rows = append(rows, []*sp{pat})
		}
		var bt *Type
		if useExp {
			bt = c.check(arm.Body, exp)
		} else {
			bt = c.expr(arm.Body, nil)
			if result == nil {
				result = bt
			} else if j := c.join(result, bt); j != nil {
				result = j
			} else if !containsErr(bt) && !containsErr(result) {
				c.errorf("E1201", arm.Body.GetPos(), "match arms have different types %s and %s", typeString(result), typeString(bt))
			}
		}
		if prune(bt).K != KNever {
			allNever = false
		}
		c.pop()
	}
	sub := prune(st)
	singleEnum := sub.K == KNamed && sub.Info.Kind == DEnum
	wildArm := false
	if singleEnum {
		for _, arm := range e.Arms {
			switch arm.Pat.(type) {
			case *ast.WildcardPat, *ast.BindPat:
				wildArm = true
				c.errorf("E2002", arm.Pos, "a match on a single enum cannot use a catch-all arm; list the variants (use A | B => ...)")
			}
		}
	}
	if !wildArm && !isErr(st) && !patErr {
		if c.useful(rows, []*sp{wildSP}, []*Type{st}) {
			c.reportNonExhaustive(e, st, rows)
		}
	}
	if useExp {
		if allNever {
			return neverT
		}
		if prune(exp).K == KVoid {
			return unitT
		}
		return exp
	}
	if result == nil {
		return neverT
	}
	return result
}

func (c *checker) reportNonExhaustive(e *ast.MatchExpr, st *Type, rows [][]*sp) {
	cs, complete := c.ctors(st)
	var missing []string
	var arms []string
	if complete {
		for _, ci := range cs {
			if ci.name == "()" {
				continue
			}
			q := &sp{ctor: ci.name, args: wilds(len(ci.args))}
			if c.useful(rows, []*sp{q}, []*Type{st}) {
				missing = append(missing, ci.name)
				arm := ci.name
				if len(ci.args) > 0 {
					arm += "(" + strings.TrimSuffix(strings.Repeat("_, ", len(ci.args)), ", ") + ")"
				}
				arms = append(arms, arm+" => todo()")
			}
		}
	}
	msg := "match is not exhaustive"
	if len(missing) > 0 {
		msg += ": missing " + strings.Join(missing, ", ")
	}
	var fixes []diag.Fix
	if len(arms) > 0 && len(e.Arms) > 0 {
		at := e.Arms[len(e.Arms)-1].Pos
		indent := strings.Repeat(" ", max(at.Col-1, 0))
		text := ""
		for _, a := range arms {
			text += a + "\n" + indent
		}
		fixes = append(fixes, diag.Fix{Kind: diag.FixScaffold, Title: "add missing arms",
			Edits: []diag.Edit{{Start: at, End: at, Text: text}}})
	}
	if c.fn != nil {
		c.fn.markError()
	}
	c.diags.Add(diag.Diagnostic{Code: "E2001", Severity: diag.Error, Pos: e.Pos, Msg: msg, Fixes: fixes})
}
