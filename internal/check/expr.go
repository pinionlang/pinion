package check

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pinionlang/pinion/internal/ast"
	"github.com/pinionlang/pinion/internal/diag"
	"github.com/pinionlang/pinion/internal/token"
)

// ---------- blocks and statements ----------

func isVoidish(t *Type) bool {
	t = prune(t)
	return t == nil || t.K == KVoid || t.K == KUnit
}

// block checks a block against exp (nil = infer).
func (c *checker) block(b *ast.Block, exp *Type) *Type {
	c.push()
	defer c.pop()
	n := len(b.Stmts)
	diverges := false
	for i, s := range b.Stmts {
		if i == n-1 {
			if es, ok := s.(*ast.ExprStmt); ok {
				if exp == nil {
					t := c.expr(es.X, nil)
					if diverges {
						return neverT
					}
					return t
				}
				t := c.check(es.X, exp)
				if prune(t).K == KNever || diverges {
					return neverT
				}
				if prune(exp).K == KVoid {
					return unitT
				}
				return exp
			}
		}
		if t := c.stmt(s); t != nil && prune(t).K == KNever {
			diverges = true
		}
	}
	if diverges {
		return neverT
	}
	if exp != nil && !isVoidish(exp) && !isErr(exp) && prune(exp).K != KVar {
		pos := b.End
		if n > 0 {
			pos = b.Stmts[n-1].GetPos()
		}
		c.errorf("E1201", pos, "block has no value; expected %s", typeString(exp))
		return exp
	}
	return unitT
}

func (c *checker) inTest() bool {
	for f := c.fn; f != nil; f = f.parent {
		if f.allowAll && f.ret != nil && f.owner == nil && !f.isInit {
			return true
		}
	}
	return false
}

func (c *checker) stmt(s ast.Stmt) *Type {
	switch s := s.(type) {
	case *ast.ExprStmt:
		return c.check(s.X, voidT)
	case *ast.Block:
		return c.block(s, voidT)
	case *ast.LetStmt:
		c.letStmt(s)
	case *ast.AssignStmt:
		c.assign(s)
	case *ast.ForStmt:
		it := prune(c.expr(s.Iter, nil))
		var elem *Type
		switch it.K {
		case KList, KMutList, KSet, KMutSet:
			elem = it.Args[0]
		case KMap, KMutMap:
			elem = tupleOf(it.Args[0], it.Args[1])
		case KRange, KBytes:
			elem = intT
		case KError:
			elem = errT
		default:
			c.errorf("E1201", s.Iter.GetPos(), "cannot iterate over %s", typeString(it))
			elem = errT
		}
		c.push()
		c.bindIrrefutable(s.Pat, elem, false)
		c.block(s.Body, voidT)
		c.pop()
	case *ast.WhileStmt:
		c.check(s.Cond, boolT)
		c.block(s.Body, voidT)
	case *ast.WithStmt:
		if !c.inTest() {
			c.errorf("E3201", s.Pos, "effect replacement `with %s:` is only allowed inside test blocks", s.Effect)
		}
		c.expr(s.Handler, nil)
		c.block(s.Body, voidT)
	case *ast.AssertStmt:
		c.check(s.Cond, boolT)
	}
	return unitT
}

func (c *checker) letStmt(s *ast.LetStmt) {
	var declT *Type
	if s.Type != nil {
		declT = c.resolveType(s.Type, c.typeEnvHere())
	}
	if s.Else != nil {
		var vexp *Type
		if declT != nil {
			vexp = optOf(declT)
		}
		vt := prune(c.expr(s.Value, vexp))
		inner := errT
		switch vt.K {
		case KOption:
			inner = vt.Args[0]
		case KError:
		case KResult:
			c.errorf("E1201", s.Value.GetPos(), "let ... else needs an optional value, got %s; use .ok() to drop the error", typeString(vt))
		default:
			c.errorf("E1201", s.Value.GetPos(), "let ... else needs an optional value, got %s", typeString(vt))
		}
		if declT != nil {
			c.assignOrReport(inner, declT, s.Value.GetPos())
			inner = declT
		}
		et := prune(c.expr(s.Else, nil))
		if et.K != KNever && et.K != KError {
			c.errorf("E1201", s.Else.GetPos(), "the else branch of let ... else must not complete normally (use return, break, continue or a Never call)")
		}
		c.bindIrrefutable(s.Pat, inner, s.Var)
		return
	}
	var t *Type
	if declT != nil {
		c.check(s.Value, declT)
		t = declT
	} else {
		t = c.expr(s.Value, nil)
		if pt := prune(t); pt.K == KVoid {
			t = unitT
		}
	}
	c.bindIrrefutable(s.Pat, t, s.Var)
}

func (c *checker) bindIrrefutable(p ast.Pattern, t *Type, mutable bool) {
	switch p := p.(type) {
	case nil:
	case *ast.WildcardPat:
	case *ast.BindPat:
		c.declare(p.Name, t, mutable)
	case *ast.TuplePat:
		tt := prune(t)
		if tt.K == KError {
			for _, e := range p.Elems {
				c.bindIrrefutable(e, errT, mutable)
			}
			return
		}
		if tt.K != KTuple || len(tt.Args) != len(p.Elems) {
			c.errorf("E1201", p.Pos, "tuple pattern does not match %s", typeString(t))
			for _, e := range p.Elems {
				c.bindIrrefutable(e, errT, mutable)
			}
			return
		}
		for i, e := range p.Elems {
			c.bindIrrefutable(e, tt.Args[i], mutable)
		}
	default:
		binds := map[string]*Type{}
		c.pattern(p, t, binds)
		for k, v := range binds {
			c.declare(k, v, mutable)
		}
	}
}

func (c *checker) typeEnvHere() *typeEnv {
	env := &typeEnv{}
	if c.fn != nil {
		for f := c.fn; f != nil; f = f.parent {
			if f.owner != nil {
				env.self = f.owner
				env = env.with(f.owner.Params)
				break
			}
		}
	}
	return env
}

func isSelf(c *checker, e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	if !ok || id.Name != "self" {
		return false
	}
	b := c.lookup("self")
	return b != nil && b.self
}

func (c *checker) assign(s *ast.AssignStmt) {
	var target *Type
	switch t := s.Target.(type) {
	case *ast.Ident:
		b := c.lookup(t.Name)
		switch {
		case b == nil:
			if c.mod.lets[t.Name] != nil {
				c.errorf("E1202", s.Pos, "cannot assign to top-level let %s", t.Name)
			} else {
				c.errorf("E1101", t.Pos, "unknown name %s", t.Name)
			}
			target = errT
		case !b.mutable:
			c.errorFix("E1202", s.Pos, diag.Fix{Kind: diag.FixReview, Title: "declare " + t.Name + " with var"},
				"cannot assign to %s: it is bound with let", t.Name)
			target = b.t
		case c.fn != nil && c.fn.depth > b.depth:
			c.errorf("E3102", s.Pos, "closure cannot assign to captured var %s; use a mutable collection or fold", t.Name)
			target = b.t
		default:
			target = b.t
		}
	case *ast.MemberExpr:
		xt := prune(c.expr(t.X, nil))
		target = errT
		if xt.K == KError {
			break
		}
		var fi *FieldInfo
		if xt.K == KNamed {
			fi = xt.Info.field(t.Name)
		}
		if fi == nil {
			c.errorf("E1101", t.Pos, "%s has no field %s", typeString(xt), t.Name)
			break
		}
		target = c.fieldType(xt, fi)
		if !isSelf(c, t.X) {
			c.errorf("E3105", s.Pos, "fields can only be assigned as self.%s inside a mut fn of %s itself", t.Name, xt.Info.Name)
			break
		}
		if !c.fn.mutFn {
			c.errorFix("E3101", s.Pos, diag.Fix{Kind: diag.FixReview, Title: "make the method mut fn"},
				"cannot assign self.%s in a method that is not mut fn", t.Name)
			break
		}
		if !fi.Mut {
			c.errorf("E3104", s.Pos, "field %s is not mut and cannot be assigned after construction", t.Name)
		}
	case *ast.IndexExpr:
		xt := prune(c.expr(t.X, nil))
		switch xt.K {
		case KMutList:
			c.check(t.Index, intT)
			target = xt.Args[0]
		case KMutMap:
			c.check(t.Index, xt.Args[0])
			target = xt.Args[1]
		case KError:
			c.expr(t.Index, nil)
			target = errT
		default:
			c.errorf("E1201", s.Pos, "cannot assign through index of %s", typeString(xt))
			c.expr(t.Index, nil)
			target = errT
		}
	default:
		c.errorf("E1201", s.Pos, "invalid assignment target")
		target = errT
	}
	if s.Op != token.Assign {
		vt := c.expr(s.Value, target)
		c.arith(s.Pos, opOf(s.Op), target, vt)
		return
	}
	c.check(s.Value, target)
}

func opOf(k token.Kind) token.Kind {
	switch k {
	case token.PlusAssign:
		return token.Plus
	case token.MinusAssign:
		return token.Minus
	case token.StarAssign:
		return token.Star
	case token.SlashAssign:
		return token.Slash
	}
	return k
}

// ---------- expressions ----------

// check infers e against exp and reports incompatibility.
func (c *checker) check(e ast.Expr, exp *Type) *Type {
	t := c.expr(e, exp)
	if exp == nil {
		return t
	}
	if prune(exp).K == KVoid {
		if pt := prune(t); pt.K == KResult {
			c.errorFix("E2101", e.GetPos(), diag.Fix{Kind: diag.FixReview, Title: "discard explicitly with let _ =",
				Edits: []diag.Edit{{Start: e.GetPos(), End: e.GetPos(), Text: "let _ = "}}},
				"returned %s is not used; handle it or discard it with `let _ = ...`", typeString(t))
		}
		return t
	}
	c.lastWrapCandidate = e
	c.assignOrReport(t, exp, e.GetPos())
	c.lastWrapCandidate = nil
	return t
}

func (c *checker) assignOrReport(t, exp *Type, pos token.Pos) bool {
	code := c.assignCode(t, exp)
	switch code {
	case "":
		return true
	case "E3002":
		c.errorf("E3002", pos, "function value with effects %s cannot be used as %s", prune(t).Eff, typeString(exp))
	default:
		if containsErr(t) || containsErr(exp) {
			return false
		}
		ep := prune(exp)
		if ep.K == KResult && c.assignable(t, ep.Args[0]) {
			fix := diag.Fix{Kind: diag.FixReview, Title: "wrap the value in Ok(...)"}
			if end, ok := exprEnd(c.lastWrapCandidate); ok && c.lastWrapCandidate != nil && c.lastWrapCandidate.GetPos() == pos {
				fix.Edits = []diag.Edit{{Start: pos, End: pos, Text: "Ok("}, {Start: end, End: end, Text: ")"}}
			}
			c.errorFix("E1201", pos, fix, "expected %s, found %s (success values are not wrapped in Ok automatically)", typeString(exp), typeString(t))
			return false
		}
		c.errorf("E1201", pos, "expected %s, found %s", typeString(exp), typeString(t))
	}
	return false
}

// exprEnd returns the end position of simple single-token expressions.
func exprEnd(e ast.Expr) (token.Pos, bool) {
	var text string
	var p token.Pos
	switch e := e.(type) {
	case *ast.Ident:
		text, p = e.Name, e.Pos
	case *ast.IntLit:
		text, p = e.Text, e.Pos
	case *ast.FloatLit:
		text, p = e.Text, e.Pos
	case *ast.BoolLit:
		p = e.Pos
		text = "false"
		if e.Value {
			text = "true"
		}
	default:
		return token.Pos{}, false
	}
	p.Col += utf8.RuneCountInString(text)
	p.Off += len(text)
	return p, true
}

func (c *checker) expr(e ast.Expr, exp *Type) *Type {
	switch e := e.(type) {
	case *ast.Ident:
		return c.ident(e, exp)
	case *ast.IntLit:
		return intT
	case *ast.FloatLit:
		return floatT
	case *ast.DecLit:
		return decT
	case *ast.BoolLit:
		return boolT
	case *ast.StrLit:
		for _, p := range e.Parts {
			if p.Expr == nil {
				continue
			}
			t := c.expr(p.Expr, nil)
			if !isDisplay(t) {
				c.errorf("E1208", p.Expr.GetPos(), "%s cannot be interpolated; convert it explicitly (e.g. .len(), ?? \"-\", debug(x))", typeString(t))
			}
		}
		return strT
	case *ast.ListLit:
		var elem *Type
		if ep := prune(exp); ep != nil && (ep.K == KList || ep.K == KMutList) {
			elem = ep.Args[0]
		}
		if elem != nil {
			for _, x := range e.Elems {
				c.check(x, elem)
			}
			return listOf(elem)
		}
		for _, x := range e.Elems {
			t := c.expr(x, elem)
			if elem == nil {
				elem = t
				continue
			}
			if j := c.join(elem, t); j != nil {
				elem = j
			} else {
				c.assignOrReport(t, elem, x.GetPos())
			}
		}
		if elem == nil {
			elem = c.fresh()
		}
		return listOf(elem)
	case *ast.MapLit:
		var kt, vt *Type
		if ep := prune(exp); ep != nil && (ep.K == KMap || ep.K == KMutMap) {
			kt, vt = ep.Args[0], ep.Args[1]
			for _, en := range e.Entries {
				c.check(en.Key, kt)
				c.check(en.Value, vt)
			}
			return &Type{K: KMap, Args: []*Type{kt, vt}}
		}
		for _, en := range e.Entries {
			k := c.expr(en.Key, kt)
			v := c.expr(en.Value, vt)
			if kt == nil {
				kt, vt = k, v
				continue
			}
			c.assignOrReport(k, kt, en.Key.GetPos())
			if j := c.join(vt, v); j != nil {
				vt = j
			} else {
				c.assignOrReport(v, vt, en.Value.GetPos())
			}
		}
		if kt == nil {
			kt, vt = c.fresh(), c.fresh()
		}
		return &Type{K: KMap, Args: []*Type{kt, vt}}
	case *ast.TupleLit:
		if len(e.Elems) == 0 {
			return unitT
		}
		ep := prune(exp)
		ts := make([]*Type, len(e.Elems))
		for i, x := range e.Elems {
			if ep != nil && ep.K == KTuple && len(ep.Args) == len(e.Elems) {
				ts[i] = c.expr(x, ep.Args[i])
			} else {
				ts[i] = c.expr(x, nil)
			}
		}
		return tupleOf(ts...)
	case *ast.BinaryExpr:
		return c.binary(e, exp)
	case *ast.UnaryExpr:
		if e.Op == token.Bang {
			c.check(e.X, boolT)
			return boolT
		}
		t := c.expr(e.X, exp)
		if pt := prune(t); !isNumeric(pt) && pt.K != KError {
			c.errorf("E1201", e.Pos, "unary - needs a number, found %s", typeString(t))
			return errT
		}
		return t
	case *ast.CallExpr:
		return c.call(e, exp)
	case *ast.MemberExpr:
		return c.memberValue(e, exp)
	case *ast.IndexExpr:
		xt := prune(c.expr(e.X, nil))
		switch xt.K {
		case KList, KMutList:
			c.check(e.Index, intT)
			return xt.Args[0]
		case KBytes:
			c.check(e.Index, intT)
			return intT
		case KError:
			c.expr(e.Index, nil)
			return errT
		case KStr:
			c.expr(e.Index, nil)
			c.errorf("E1201", e.Pos, "Str cannot be indexed; use s.slice(i, i + 1) or s.chars()")
			return errT
		}
		c.expr(e.Index, nil)
		c.errorf("E1201", e.Pos, "%s cannot be indexed", typeString(xt))
		return errT
	case *ast.TryExpr:
		return c.try(e)
	case *ast.IfExpr:
		return c.ifExpr(e, exp)
	case *ast.MatchExpr:
		return c.match(e, exp)
	case *ast.BlockExpr:
		return c.block(e.Block, exp)
	case *ast.LambdaExpr:
		return c.lambda(e, exp)
	case *ast.RangeExpr:
		c.check(e.Lo, intT)
		c.check(e.Hi, intT)
		return rangeT
	case *ast.SelfLit:
		return c.selfLit(e)
	case *ast.ReturnExpr:
		c.returnExpr(e)
		return neverT
	case *ast.BreakExpr, *ast.ContinueExpr:
		return neverT
	}
	return errT
}

func (c *checker) returnExpr(e *ast.ReturnExpr) {
	if c.fn == nil || c.fn.ret == nil {
		if e.Value != nil {
			c.expr(e.Value, nil)
		}
		return
	}
	ret := c.fn.ret
	if e.Value == nil {
		if !isVoidish(ret) && !isErr(ret) {
			if pr := prune(ret); pr.K == KVar {
				c.unify(pr, unitT)
				return
			}
			c.errorf("E1201", e.Pos, "return without a value in a function returning %s", typeString(ret))
		}
		return
	}
	if isVoidish(ret) {
		c.check(e.Value, voidT)
		return
	}
	c.check(e.Value, ret)
}

func isNumeric(t *Type) bool {
	t = prune(t)
	return t.K == KInt || t.K == KFloat || t.K == KDec
}

func (c *checker) binary(e *ast.BinaryExpr, exp *Type) *Type {
	switch e.Op {
	case token.AndAnd, token.OrOr:
		c.check(e.X, boolT)
		c.check(e.Y, boolT)
		return boolT
	case token.QQ:
		var xexp *Type
		if exp != nil && !isVoidish(exp) {
			xexp = optOf(exp)
			if prune(exp).K == KOption {
				xexp = exp
			}
		}
		lt := prune(c.expr(e.X, xexp))
		switch lt.K {
		case KError:
			c.expr(e.Y, nil)
			return errT
		case KOption:
			inner := lt.Args[0]
			rt := c.expr(e.Y, inner)
			if c.assignable(rt, inner) {
				return inner
			}
			if c.assignable(rt, lt) {
				return lt
			}
			c.assignOrReport(rt, inner, e.Y.GetPos())
			return inner
		case KResult:
			c.expr(e.Y, nil)
			c.errorf("E1201", e.X.GetPos(), "?? needs an optional value, found %s; use .ok() to drop the error", typeString(lt))
			return errT
		}
		c.expr(e.Y, nil)
		c.errorf("E1201", e.X.GetPos(), "?? needs an optional value, found %s", typeString(lt))
		return errT
	case token.Eq, token.NotEq, token.Lt, token.Gt, token.LtEq, token.GtEq:
		lt := c.expr(e.X, nil)
		rt := c.expr(e.Y, lt)
		j := c.join(lt, rt)
		if j == nil {
			if !containsErr(lt) && !containsErr(rt) {
				c.errorf("E1201", e.Pos, "cannot compare %s with %s", typeString(lt), typeString(rt))
			}
			return boolT
		}
		if e.Op == token.Eq || e.Op == token.NotEq {
			if !c.has(j, PEq) {
				c.errorf("E1301", e.Pos, "%s has no Eq property; == is not available", typeString(j))
			}
		} else if !c.has(j, POrd) {
			c.errorf("E1209", e.Pos, "%s has no Ord property; ordering comparison is not available", typeString(j))
		}
		return boolT
	}
	var xexp *Type
	if exp != nil && isNumeric(exp) {
		xexp = exp
	}
	lt := c.expr(e.X, xexp)
	rt := c.expr(e.Y, lt)
	return c.arith(e.Pos, e.Op, lt, rt)
}

func (c *checker) arith(pos token.Pos, op token.Kind, lt, rt *Type) *Type {
	l, r := prune(lt), prune(rt)
	if l.K == KError || r.K == KError {
		return errT
	}
	if l.K == KStr && op == token.Plus {
		if r.K != KStr {
			c.errorf("E1201", pos, "cannot add %s to Str", typeString(r))
		}
		return strT
	}
	if !isNumeric(l) {
		if l.K == KVar && c.tryUnify(l, r) && isNumeric(r) {
			return r
		}
		c.errorf("E1201", pos, "arithmetic needs Int, Float or Dec, found %s", typeString(l))
		return errT
	}
	if !c.unify(l, r) {
		c.errorf("E1201", pos, "mixed numeric types %s and %s; convert explicitly (to_float(), to_dec())", typeString(l), typeString(r))
		return errT
	}
	if l.K == KDec && op == token.Slash {
		c.errorf("E1206", pos, "Dec has no / operator; use a.div(b, scale: n, round: Round.HalfEven)")
	}
	return l
}

func (c *checker) try(e *ast.TryExpr) *Type {
	t := prune(c.expr(e.X, nil))
	if t.K == KError {
		return errT
	}
	if t.K != KResult {
		c.errorf("E2102", e.Pos, "? can only be used on Result, found %s; use let ... else or ?? for optional values", typeString(t))
		return errT
	}
	var fr *Type
	if c.fn != nil {
		fr = prune(c.fn.ret)
	}
	switch {
	case fr == nil || fr.K == KError:
	case fr.K == KVar:
		c.unify(fr, resultOf(c.fresh(), t.Args[1]))
	case fr.K == KResult:
		if !c.tryUnify(t.Args[1], fr.Args[1]) {
			c.errorFix("E2103", e.Pos, diag.Fix{Kind: diag.FixReview, Title: "convert the error with .map_err(...) before ?"},
				"? propagates %s but the function returns errors of type %s; convert with .map_err(...)", typeString(t.Args[1]), typeString(fr.Args[1]))
		}
	default:
		c.errorf("E2102", e.Pos, "? needs the enclosing function to return Result, but it returns %s", typeString(fr))
	}
	return t.Args[0]
}

func (c *checker) ifExpr(e *ast.IfExpr, exp *Type) *Type {
	c.check(e.Cond, boolT)
	if e.Else == nil {
		c.block(e.Then, voidT)
		return unitT
	}
	if exp != nil && prune(exp).K != KVar {
		tt := c.block(e.Then, exp)
		et := c.check(e.Else, exp)
		if prune(tt).K == KNever && prune(et).K == KNever {
			return neverT
		}
		if prune(exp).K == KVoid {
			return unitT
		}
		return exp
	}
	tt := c.block(e.Then, nil)
	et := c.expr(e.Else, nil)
	j := c.join(tt, et)
	if j == nil {
		if !containsErr(tt) && !containsErr(et) {
			c.errorf("E1201", e.Else.GetPos(), "if branches have different types %s and %s", typeString(tt), typeString(et))
		}
		return tt
	}
	return j
}

func (c *checker) lambda(e *ast.LambdaExpr, exp *Type) *Type {
	var ft *Type
	if ep := prune(exp); ep != nil && ep.K == KFn && len(ep.Args) == len(e.Params) {
		ft = ep
	}
	parent := c.fn
	ctx := &fnCtx{parent: parent, lambda: true}
	if parent != nil {
		ctx.depth = parent.depth + 1
		ctx.owner, ctx.selfT, ctx.method, ctx.mutFn, ctx.isInit = parent.owner, parent.selfT, parent.method, parent.mutFn, parent.isInit
	} else {
		ctx.depth = 1
	}
	var ret *Type
	if ft != nil {
		ret = ft.Ret
	} else {
		ret = c.fresh()
	}
	ctx.ret = ret
	c.push()
	params := make([]*Type, len(e.Params))
	for i, p := range e.Params {
		switch {
		case p.Type != nil:
			params[i] = c.resolveType(p.Type, c.typeEnvHere())
			if ft != nil {
				c.unify(params[i], ft.Args[i])
			}
		case ft != nil:
			params[i] = ft.Args[i]
		default:
			params[i] = c.fresh()
		}
	}
	c.fn = ctx
	for i, p := range e.Params {
		c.declare(p.Name, params[i], false)
	}
	bexp := ret
	if isVoidish(ret) {
		bexp = voidT
	}
	bt := c.check(e.Body, bexp)
	c.fn = parent
	c.pop()
	if isVoidish(ret) {
		ret = unitT
	} else if prune(ret).K == KVar && prune(bt).K != KNever {
		c.unify(ret, bt)
	}
	return &Type{K: KFn, Args: params, Ret: ret, Eff: ctx.used}
}

func (c *checker) selfLit(e *ast.SelfLit) *Type {
	f := c.fn
	for f != nil && f.lambda {
		f = f.parent
	}
	var owner *TypeInfo
	if f != nil {
		owner = f.owner
	}
	if f == nil || !f.isInit || owner == nil {
		c.errorf("E1504", e.Pos, "Self { ... } can only be used inside init; use the constructor or a factory fn")
		for _, a := range e.Fields {
			c.expr(a.Value, nil)
		}
		if owner != nil {
			return owner.selfType()
		}
		return errT
	}
	seen := map[string]bool{}
	for _, a := range e.Fields {
		fi := owner.field(a.Name)
		if fi == nil {
			c.errorf("E1101", a.Pos, "%s has no field %s", owner.Name, a.Name)
			c.expr(a.Value, nil)
			continue
		}
		seen[a.Name] = true
		c.check(a.Value, fi.Type)
	}
	var missing []string
	for _, fi := range owner.Fields {
		if !seen[fi.Name] && !fi.HasDefault {
			missing = append(missing, fi.Name)
		}
	}
	if len(missing) > 0 {
		c.errorf("E1505", e.Pos, "Self { ... } must set every field; missing %s", strings.Join(missing, ", "))
	}
	return owner.selfType()
}

// ---------- names ----------

func (c *checker) instantiate(fi *FnInfo, m map[*TypeParamInfo]*Type) *Type {
	mm := map[*TypeParamInfo]*Type{}
	for k, v := range m {
		mm[k] = v
	}
	for _, tp := range fi.TParams {
		mm[tp] = c.fresh()
	}
	return subst(fi.fnType(), mm)
}

func (c *checker) variantHint(name string) string {
	for _, ti := range c.mod.types {
		if ti.Kind == DEnum && ti.variant(name) != nil {
			return ti.Name
		}
	}
	return ""
}

func (c *checker) ident(e *ast.Ident, exp *Type) *Type {
	if b := c.lookup(e.Name); b != nil {
		return b.t
	}
	if e.Name == "None" {
		if ep := prune(exp); ep != nil && ep.K == KOption {
			return ep
		}
		return optOf(c.fresh())
	}
	if fi := c.mod.fns[e.Name]; fi != nil {
		return c.instantiate(fi, nil)
	}
	if li := c.mod.lets[e.Name]; li != nil {
		return c.letType(li)
	}
	if fi := c.std.prelude[e.Name]; fi != nil {
		return c.instantiate(fi, nil)
	}
	if e.Name == "self" {
		c.errorf("E1101", e.Pos, "self is only available inside methods")
		return errT
	}
	if en := c.variantHint(e.Name); en != "" {
		c.errorFix("E1101", e.Pos, diag.Fix{Kind: diag.FixAuto, Title: "qualify the variant as " + en + "." + e.Name,
			Edits: []diag.Edit{{Start: e.Pos, End: e.Pos, Text: en + "."}}},
			"unknown name %s; enum variants are written with their type name in expressions (%s.%s)", e.Name, en, e.Name)
		return errT
	}
	c.errorf("E1101", e.Pos, "unknown name %s", e.Name)
	return errT
}

func isUpper(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsUpper(r)
}
