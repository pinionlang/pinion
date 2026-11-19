package codegen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pinionlang/pinion/internal/ast"
	"github.com/pinionlang/pinion/internal/token"
)

type binding struct {
	goName  string
	mutable bool
	depth   int // lambda nesting depth where declared
}

type scope struct {
	vars   map[string]*binding
	parent *scope
}

type lambdaCtx struct {
	caps  map[string]string // outer go name -> captured copy
	order []string
}

type fnGen struct {
	p       *program
	m       *module
	owner   *ast.TypeDecl
	inInit  bool
	buf     *strings.Builder
	indent  int
	scope   *scope
	n       *int
	pos     token.Pos
	depth   int
	lambdas []*lambdaCtx
}

func (p *program) newFn(m *module, owner *ast.TypeDecl) *fnGen {
	n := 0
	return &fnGen{p: p, m: m, owner: owner, buf: &strings.Builder{}, scope: &scope{vars: map[string]*binding{}}, n: &n}
}

func (g *fnGen) linePos() string {
	if !g.pos.IsValid() {
		return ""
	}
	f := g.pos.File
	if g.p.opts.PathMap != nil {
		f = g.p.opts.PathMap(f)
	}
	key := f + ":" + strconv.Itoa(g.pos.Line)
	if c, ok := g.p.lineCols[key]; !ok || g.pos.Col < c {
		g.p.lineCols[key] = g.pos.Col
	}
	return fmt.Sprintf("/*line %s:%d:%d*/", f, g.pos.Line, g.pos.Col)
}

func (g *fnGen) emit(format string, args ...any) {
	g.buf.WriteString(strings.Repeat("\t", g.indent))
	g.buf.WriteString(g.linePos())
	fmt.Fprintf(g.buf, format, args...)
	g.buf.WriteByte('\n')
}

func (g *fnGen) open(format string, args ...any) {
	g.emit(format, args...)
	g.indent++
}

func (g *fnGen) close(s string) {
	g.indent--
	g.buf.WriteString(strings.Repeat("\t", g.indent) + s + "\n")
}

func (g *fnGen) fresh(base string) string {
	*g.n++
	return fmt.Sprintf("%s%d", base, *g.n)
}

func (g *fnGen) tmp(val string) string {
	t := g.fresh("_t")
	g.emit("var %s rt.Value = %s", t, val)
	return t
}

func sanitize(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('x')
		}
	}
	return b.String()
}

func (g *fnGen) declare(name string, mutable bool) string {
	gn := g.fresh(sanitize(name) + "_")
	g.scope.vars[name] = &binding{goName: gn, mutable: mutable, depth: g.depth}
	return gn
}

func (g *fnGen) push() { g.scope = &scope{vars: map[string]*binding{}, parent: g.scope} }
func (g *fnGen) pop()  { g.scope = g.scope.parent }

func (g *fnGen) lookup(name string) *binding {
	for s := g.scope; s != nil; s = s.parent {
		if b, ok := s.vars[name]; ok {
			return b
		}
	}
	return nil
}

// local returns the Go expression for a local binding (capturing `var`s by value in lambdas).
func (g *fnGen) local(name string) (string, bool) {
	b := g.lookup(name)
	if b == nil {
		return "", false
	}
	if b.mutable && b.depth < g.depth {
		lc := g.lambdas[len(g.lambdas)-1]
		if c, ok := lc.caps[b.goName]; ok {
			return c, true
		}
		c := g.fresh("cap_")
		lc.caps[b.goName] = c
		lc.order = append(lc.order, b.goName)
		return c, true
	}
	return b.goName, true
}

// capture runs f with a fresh buffer, returning the expression and the emitted code.
func (g *fnGen) capture(f func() string) (string, string) {
	saved := g.buf
	g.buf = &strings.Builder{}
	s := f()
	code := g.buf.String()
	g.buf = saved
	return s, code
}

func isConst(s string) bool {
	if s == "" {
		return true
	}
	if s[0] == '"' || s[0] == '`' || (s[0] >= '0' && s[0] <= '9') {
		return true
	}
	for _, p := range []string{"int64(", "float64(", "rt.UnitV", "rt.None", "true", "false"} {
		if strings.HasPrefix(s, p) && !strings.ContainsAny(s[len(p):], "()") || s == p {
			return true
		}
	}
	return false
}

// exprs evaluates a list of expressions strictly left to right.
func (g *fnGen) exprs(es []ast.Expr) []string {
	out := make([]string, 0, len(es))
	for i, e := range es {
		s, code := g.capture(func() string { return g.expr(e) })
		if code != "" {
			for j := 0; j < i; j++ {
				if !isConst(out[j]) {
					out[j] = g.tmp(out[j])
				}
			}
			g.buf.WriteString(code)
		}
		out = append(out, s)
	}
	return out
}

func (g *fnGen) setPos(p token.Pos) {
	if p.IsValid() {
		g.pos = p
	}
}

// ---------- statements ----------

// blockValue emits a block's statements (in a new scope, without braces) and returns its value.
func (g *fnGen) blockValue(b *ast.Block) string {
	g.push()
	defer g.pop()
	val := "rt.UnitV"
	for i, s := range b.Stmts {
		if es, ok := s.(*ast.ExprStmt); ok && i == len(b.Stmts)-1 {
			g.setPos(es.Pos)
			v := g.expr(es.X)
			if isConst(v) {
				return v
			}
			// materialise so that later scope pops don't matter
			return v
		}
		g.stmt(s)
	}
	return val
}

func (g *fnGen) stmt(s ast.Stmt) {
	g.setPos(s.GetPos())
	switch s := s.(type) {
	case *ast.ExprStmt:
		v := g.expr(s.X)
		if !isConst(v) {
			g.emit("_ = %s", v)
		}
	case *ast.LetStmt:
		g.letStmt(s)
	case *ast.AssignStmt:
		g.assign(s)
	case *ast.Block:
		g.open("{")
		v := g.blockValue(s)
		if !isConst(v) {
			g.emit("_ = %s", v)
		}
		g.close("}")
	case *ast.ForStmt:
		it := g.expr(s.Iter)
		x := g.fresh("_it")
		g.open("for %s := range rt.Iter(%s) {", x, it)
		g.emit("_ = %s", x)
		g.push()
		if bp, ok := s.Pat.(*ast.BindPat); ok {
			gn := g.declare(bp.Name, false)
			g.emit("var %s rt.Value = %s", gn, x)
			g.emit("_ = %s", gn)
			g.loopBody(s.Body)
		} else {
			g.declarePatVars(s.Pat)
			g.pattern(s.Pat, x, func() { g.loopBody(s.Body) })
		}
		g.pop()
		g.close("}")
	case *ast.WhileStmt:
		g.open("for {")
		c := g.expr(s.Cond)
		g.emit("if !rt.Truthy(%s) { break }", c)
		g.loopBody(s.Body)
		g.close("}")
	case *ast.WithStmt:
		h := g.expr(s.Handler)
		g.emit("rt.PushHandler(%q, %s)", s.Effect, h)
		g.open("{")
		v := g.blockValue(s.Body)
		if !isConst(v) {
			g.emit("_ = %s", v)
		}
		g.close("}")
		g.emit("rt.PopHandler(%q)", s.Effect)
	case *ast.AssertStmt:
		c := g.expr(s.Cond)
		g.emit("rt.Assert(%s, %q)", c, fmt.Sprintf("%s:%d:%d", g.displayFile(s.Pos.File), s.Pos.Line, s.Pos.Col))
	default:
		fail(s.GetPos(), "unsupported statement %T", s)
	}
}

func (g *fnGen) displayFile(f string) string {
	if g.p.opts.PathMap != nil {
		return g.p.opts.PathMap(f)
	}
	return f
}

func (g *fnGen) loopBody(b *ast.Block) {
	v := g.blockValue(b)
	if !isConst(v) {
		g.emit("_ = %s", v)
	}
}

func (g *fnGen) letStmt(s *ast.LetStmt) {
	val := g.expr(s.Value)
	if s.Else != nil {
		// let PAT = opt else { ... }: the else branch runs when opt is None;
		// otherwise PAT (irrefutable for the inner type) binds the value.
		t := g.tmp(val)
		g.open("if rt.IsNone(%s) {", t)
		g.elseExpr(s.Else)
		g.close("}")
		switch pt := s.Pat.(type) {
		case *ast.BindPat:
			gn := g.declare(pt.Name, s.Var)
			g.emit("var %s rt.Value = %s", gn, t)
			g.emit("_ = %s", gn)
		case *ast.WildcardPat:
		default:
			g.declarePatVars(s.Pat)
			ok := g.fresh("_ok")
			g.emit("%s := false", ok)
			g.pattern(s.Pat, t, func() { g.emit("%s = true", ok) })
			g.open("if !%s {", ok)
			g.elseExpr(s.Else)
			g.close("}")
		}
		return
	}
	switch pt := s.Pat.(type) {
	case *ast.BindPat:
		gn := g.declare(pt.Name, s.Var)
		g.emit("var %s rt.Value = %s", gn, val)
		g.emit("_ = %s", gn)
	case *ast.WildcardPat:
		if !isConst(val) {
			g.emit("_ = %s", val)
		}
	default:
		t := g.tmp(val)
		g.declarePatVars(s.Pat)
		ok := g.fresh("_ok")
		g.emit("%s := false", ok)
		g.pattern(s.Pat, t, func() { g.emit("%s = true", ok) })
		g.emit("if !%s { rt.Unreachable(\"let pattern did not match\") }", ok)
	}
}

func (g *fnGen) elseExpr(e ast.Expr) {
	if be, ok := e.(*ast.BlockExpr); ok {
		v := g.blockValue(be.Block)
		if !isConst(v) {
			g.emit("_ = %s", v)
		}
		return
	}
	v := g.expr(e)
	if !isConst(v) {
		g.emit("_ = %s", v)
	}
}

var assignOps = map[token.Kind]string{
	token.PlusAssign: "rt.Add", token.MinusAssign: "rt.Sub", token.StarAssign: "rt.Mul", token.SlashAssign: "rt.Div",
}

func (g *fnGen) assign(s *ast.AssignStmt) {
	op := assignOps[s.Op]
	combine := func(cur, v string) string {
		if op == "" {
			return v
		}
		return fmt.Sprintf("%s(%s, %s)", op, cur, v)
	}
	switch t := s.Target.(type) {
	case *ast.Ident:
		b := g.lookup(t.Name)
		if b == nil {
			fail(t.Pos, "assignment to unknown variable %s", t.Name)
		}
		v := g.expr(s.Value)
		g.emit("%s = %s", b.goName, combine(b.goName, v))
	case *ast.MemberExpr:
		obj := g.expr(t.X)
		if !isConst(obj) {
			obj = g.tmp(obj)
		}
		v := g.expr(s.Value)
		cur := fmt.Sprintf("rt.GetField(%s, %q)", obj, t.Name)
		if op != "" {
			v = g.tmp(v)
		}
		g.emit("rt.SetField(%s, %q, %s)", obj, t.Name, combine(cur, v))
	case *ast.IndexExpr:
		parts := g.exprs([]ast.Expr{t.X, t.Index, s.Value})
		for i := range parts[:2] {
			if !isConst(parts[i]) {
				parts[i] = g.tmp(parts[i])
			}
		}
		cur := fmt.Sprintf("rt.Index(%s, %s)", parts[0], parts[1])
		g.emit("rt.SetIndex(%s, %s, %s)", parts[0], parts[1], combine(cur, parts[2]))
	default:
		fail(s.Pos, "invalid assignment target")
	}
}

// ---------- patterns ----------

func patVars(p ast.Pattern, out *[]string) {
	switch p := p.(type) {
	case *ast.BindPat:
		*out = append(*out, p.Name)
	case *ast.VariantPat:
		for _, a := range p.Args {
			patVars(a, out)
		}
	case *ast.TuplePat:
		for _, a := range p.Elems {
			patVars(a, out)
		}
	case *ast.OrPat:
		if len(p.Alts) > 0 {
			patVars(p.Alts[0], out)
		}
	}
}

func (g *fnGen) declarePatVars(p ast.Pattern) {
	var names []string
	patVars(p, &names)
	for _, n := range names {
		gn := g.declare(n, false)
		g.emit("var %s rt.Value", gn)
		g.emit("_ = %s", gn)
	}
}

// pattern emits code testing subj (a Go variable) against p, assigning
// bindings (already declared) and calling onMatch inside the success branch.
func (g *fnGen) pattern(p ast.Pattern, subj string, onMatch func()) {
	switch p := p.(type) {
	case *ast.WildcardPat:
		onMatch()
	case *ast.BindPat:
		b := g.lookup(p.Name)
		g.emit("%s = %s", b.goName, subj)
		onMatch()
	case *ast.LitPat:
		v := g.expr(p.Value)
		g.open("if rt.Equal(%s, %s) {", subj, v)
		onMatch()
		g.close("}")
	case *ast.TuplePat:
		t := g.fresh("_tp")
		g.open("if %s, ok := %s.(*rt.Tuple); ok && len(%s.Elems) == %d {", t, subj, t, len(p.Elems))
		g.tupleElems(p.Elems, t, 0, onMatch)
		g.close("}")
	case *ast.OrPat:
		ok := g.fresh("_or")
		g.emit("%s := false", ok)
		for _, alt := range p.Alts {
			g.open("if !%s {", ok)
			g.pattern(alt, subj, func() { g.emit("%s = true", ok) })
			g.close("}")
		}
		g.open("if %s {", ok)
		onMatch()
		g.close("}")
	case *ast.VariantPat:
		switch {
		case p.Qualifier == "" && p.Name == "None":
			g.open("if rt.IsNone(%s) {", subj)
			onMatch()
			g.close("}")
		case p.Qualifier == "" && p.Name == "Some":
			g.open("if !rt.IsNone(%s) {", subj)
			if len(p.Args) == 1 {
				g.pattern(p.Args[0], subj, onMatch)
			} else {
				onMatch()
			}
			g.close("}")
		case p.Qualifier == "" && (p.Name == "Ok" || p.Name == "Err"):
			r := g.fresh("_r")
			cond := "r.IsOk"
			if p.Name == "Err" {
				cond = "!r.IsOk"
			}
			g.open("if %s, ok := %s.(*rt.Result); ok && %s {", r, subj, strings.Replace(cond, "r.", r+".", 1))
			if len(p.Args) == 1 {
				v := g.fresh("_v")
				g.emit("%s := %s.V", v, r)
				g.emit("_ = %s", v)
				g.pattern(p.Args[0], v, onMatch)
			} else {
				onMatch()
			}
			g.close("}")
		default:
			g.open("if rt.IsVariant(%s, %q) {", subj, p.Name)
			g.payloads(p.Args, subj, 0, onMatch)
			g.close("}")
		}
	default:
		fail(p.GetPos(), "unsupported pattern %T", p)
	}
}

func (g *fnGen) tupleElems(ps []ast.Pattern, t string, i int, onMatch func()) {
	if i == len(ps) {
		onMatch()
		return
	}
	if _, ok := ps[i].(*ast.WildcardPat); ok {
		g.tupleElems(ps, t, i+1, onMatch)
		return
	}
	v := g.fresh("_e")
	g.emit("%s := %s.Elems[%d]", v, t, i)
	g.pattern(ps[i], v, func() { g.tupleElems(ps, t, i+1, onMatch) })
}

func (g *fnGen) payloads(ps []ast.Pattern, subj string, i int, onMatch func()) {
	if i == len(ps) {
		onMatch()
		return
	}
	if _, ok := ps[i].(*ast.WildcardPat); ok {
		g.payloads(ps, subj, i+1, onMatch)
		return
	}
	v := g.fresh("_e")
	g.emit("%s := rt.Payload(%s, %d)", v, subj, i)
	g.pattern(ps[i], v, func() { g.payloads(ps, subj, i+1, onMatch) })
}

// ---------- expressions ----------

func quote(s string) string { return strconv.Quote(s) }

func (g *fnGen) expr(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.IntLit:
		return g.intLit(e.Text, false)
	case *ast.FloatLit:
		t := strings.ReplaceAll(e.Text, "_", "")
		if !strings.ContainsAny(t, ".eE") {
			t += ".0"
		}
		return "float64(" + t + ")"
	case *ast.DecLit:
		return "rt.DecLit(" + quote(strings.ReplaceAll(e.Text, "_", "")) + ")"
	case *ast.BoolLit:
		if e.Value {
			return "true"
		}
		return "false"
	case *ast.StrLit:
		return g.strLit(e)
	case *ast.ListLit:
		return "rt.ListLit(" + strings.Join(g.exprs(e.Elems), ", ") + ")"
	case *ast.MapLit:
		var es []ast.Expr
		for _, en := range e.Entries {
			es = append(es, en.Key, en.Value)
		}
		return "rt.MapLit(" + strings.Join(g.exprs(es), ", ") + ")"
	case *ast.TupleLit:
		if len(e.Elems) == 0 {
			return "rt.UnitV"
		}
		return "rt.Tup(" + strings.Join(g.exprs(e.Elems), ", ") + ")"
	case *ast.Ident:
		return g.ident(e)
	case *ast.BinaryExpr:
		return g.binary(e)
	case *ast.UnaryExpr:
		if e.Op == token.Minus {
			if il, ok := e.X.(*ast.IntLit); ok {
				return g.intLit(il.Text, true)
			}
			if fl, ok := e.X.(*ast.FloatLit); ok {
				return "float64(-" + strings.TrimPrefix(g.expr(fl), "float64(")
			}
			return "rt.Neg(" + g.expr(e.X) + ")"
		}
		return "rt.Not(" + g.expr(e.X) + ")"
	case *ast.CallExpr:
		g.setPos(e.Pos)
		return g.call(e)
	case *ast.MemberExpr:
		return g.member(e)
	case *ast.IndexExpr:
		g.setPos(e.Pos)
		xs := g.exprs([]ast.Expr{e.X, e.Index})
		return "rt.Index(" + xs[0] + ", " + xs[1] + ")"
	case *ast.TryExpr:
		v := g.expr(e.X)
		r := g.fresh("_try")
		g.emit("%s := %s.(*rt.Result)", r, v)
		g.emit("if !%s.IsOk { return %s }", r, r)
		return r + ".V"
	case *ast.IfExpr:
		return g.ifExpr(e)
	case *ast.MatchExpr:
		return g.match(e)
	case *ast.BlockExpr:
		t := g.fresh("_b")
		g.emit("var %s rt.Value = rt.UnitV", t)
		g.open("{")
		v := g.blockValue(e.Block)
		g.emit("%s = %s", t, v)
		g.close("}")
		return t
	case *ast.LambdaExpr:
		return g.lambda(e)
	case *ast.RangeExpr:
		xs := g.exprs([]ast.Expr{e.Lo, e.Hi})
		return fmt.Sprintf("rt.MkRange(%s, %s, %v)", xs[0], xs[1], e.Inclusive)
	case *ast.SelfLit:
		if g.owner == nil {
			fail(e.Pos, "Self { } outside init")
		}
		names, vals := g.args(e.Fields)
		return fmt.Sprintf("rt.SelfLit(%s, %s, %s)", g.m.typeVar(g.owner.Name), names, vals)
	case *ast.ReturnExpr:
		v := "rt.UnitV"
		if e.Value != nil {
			v = g.expr(e.Value)
		}
		g.emit("return %s", v)
		return "rt.UnitV"
	case *ast.BreakExpr:
		g.emit("break")
		return "rt.UnitV"
	case *ast.ContinueExpr:
		g.emit("continue")
		return "rt.UnitV"
	}
	fail(e.GetPos(), "unsupported expression %T", e)
	return ""
}

func (g *fnGen) intLit(text string, neg bool) string {
	t := strings.ReplaceAll(text, "_", "")
	n, err := strconv.ParseInt(t, 0, 64)
	if neg {
		if err != nil {
			if t == "9223372036854775808" {
				return "int64(-9223372036854775808)"
			}
			fail(g.pos, "integer literal out of range")
		}
		return fmt.Sprintf("int64(%d)", -n)
	}
	if err != nil {
		fail(g.pos, "integer literal out of range: %s", text)
	}
	return fmt.Sprintf("int64(%d)", n)
}

func (g *fnGen) strLit(e *ast.StrLit) string {
	if len(e.Parts) == 0 {
		return `""`
	}
	if len(e.Parts) == 1 && e.Parts[0].Expr == nil {
		return quote(e.Parts[0].Text)
	}
	var es []ast.Expr
	idx := map[int]int{}
	for i, p := range e.Parts {
		if p.Expr != nil {
			idx[i] = len(es)
			es = append(es, p.Expr)
		}
	}
	vals := g.exprs(es)
	var parts []string
	for i, p := range e.Parts {
		if p.Expr != nil {
			parts = append(parts, vals[idx[i]])
		} else {
			parts = append(parts, quote(p.Text))
		}
	}
	return "rt.Interp(" + strings.Join(parts, ", ") + ")"
}

var binOps = map[token.Kind]string{
	token.Plus: "rt.Add", token.Minus: "rt.Sub", token.Star: "rt.Mul", token.Slash: "rt.Div", token.Percent: "rt.Mod",
	token.Eq: "rt.Eq", token.NotEq: "rt.Ne", token.Lt: "rt.Lt", token.LtEq: "rt.Le", token.Gt: "rt.Gt", token.GtEq: "rt.Ge",
}

func (g *fnGen) binary(e *ast.BinaryExpr) string {
	switch e.Op {
	case token.AndAnd, token.OrOr, token.QQ:
		x := g.expr(e.X)
		t := g.fresh("_s")
		g.emit("var %s rt.Value = %s", t, x)
		switch e.Op {
		case token.AndAnd:
			g.open("if rt.Truthy(%s) {", t)
		case token.OrOr:
			g.open("if !rt.Truthy(%s) {", t)
		default:
			g.open("if rt.IsNone(%s) {", t)
		}
		y := g.expr(e.Y)
		g.emit("%s = %s", t, y)
		g.close("}")
		return t
	}
	op, ok := binOps[e.Op]
	if !ok {
		fail(e.Pos, "unsupported operator %s", e.Op)
	}
	g.setPos(e.Pos)
	xs := g.exprs([]ast.Expr{e.X, e.Y})
	return op + "(" + xs[0] + ", " + xs[1] + ")"
}

func (g *fnGen) ifExpr(e *ast.IfExpr) string {
	t := g.fresh("_if")
	g.emit("var %s rt.Value = rt.UnitV", t)
	g.ifChain(e, t)
	return t
}

func (g *fnGen) ifChain(e *ast.IfExpr, t string) {
	c := g.expr(e.Cond)
	g.open("if rt.Truthy(%s) {", c)
	v := g.blockValue(e.Then)
	g.emit("%s = %s", t, v)
	if e.Else == nil {
		g.close("}")
		return
	}
	g.indent--
	g.buf.WriteString(strings.Repeat("\t", g.indent) + "} else {\n")
	g.indent++
	switch el := e.Else.(type) {
	case *ast.IfExpr:
		g.ifChain(el, t)
	case *ast.BlockExpr:
		v := g.blockValue(el.Block)
		g.emit("%s = %s", t, v)
	default:
		v := g.expr(el)
		g.emit("%s = %s", t, v)
	}
	g.close("}")
}

func (g *fnGen) match(e *ast.MatchExpr) string {
	subj := g.expr(e.Subject)
	s := g.fresh("_m")
	g.emit("var %s rt.Value = %s", s, subj)
	g.emit("_ = %s", s)
	res := g.fresh("_mr")
	g.emit("var %s rt.Value = rt.UnitV", res)
	label := g.fresh("_end")
	for _, arm := range e.Arms {
		g.setPos(arm.Pos)
		g.open("{")
		g.push()
		g.declarePatVars(arm.Pat)
		g.pattern(arm.Pat, s, func() {
			body := func() {
				var v string
				if be, ok := arm.Body.(*ast.BlockExpr); ok {
					v = g.blockValue(be.Block)
				} else {
					v = g.expr(arm.Body)
				}
				g.emit("%s = %s", res, v)
				g.emit("goto %s", label)
			}
			if arm.Guard != nil {
				c := g.expr(arm.Guard)
				g.open("if rt.Truthy(%s) {", c)
				body()
				g.close("}")
			} else {
				body()
			}
		})
		g.pop()
		g.close("}")
	}
	g.emit("rt.Unreachable(\"no match arm matched\")")
	g.buf.WriteString(label + ":\n")
	return res
}

func (g *fnGen) lambda(e *ast.LambdaExpr) string {
	lc := &lambdaCtx{caps: map[string]string{}}
	g.lambdas = append(g.lambdas, lc)
	g.depth++
	savedIndent := g.indent
	body, code := g.capture(func() string {
		g.indent = savedIndent + 1
		g.push()
		for i, p := range e.Params {
			gn := g.declare(p.Name, false)
			g.emit("var %s rt.Value = a[%d]", gn, i)
			g.emit("_ = %s", gn)
		}
		var v string
		if be, ok := e.Body.(*ast.BlockExpr); ok {
			v = g.blockValue(be.Block)
		} else {
			v = g.expr(e.Body)
		}
		g.emit("return %s", v)
		g.pop()
		return ""
	})
	_ = body
	g.indent = savedIndent
	g.depth--
	g.lambdas = g.lambdas[:len(g.lambdas)-1]
	for _, outer := range lc.order {
		src := outer
		// the outer binding may itself need capturing in an enclosing lambda
		if len(g.lambdas) > 0 {
			for s := g.scope; s != nil; s = s.parent {
				found := false
				for name, b := range s.vars {
					if b.goName == outer {
						if v, ok := g.local(name); ok {
							src = v
						}
						found = true
						break
					}
				}
				if found {
					break
				}
			}
		}
		g.emit("%s := %s", lc.caps[outer], src)
	}
	f := g.fresh("_fn")
	g.emit("%s := rt.MkFunc(%d, \"<lambda>\", func(a []rt.Value) rt.Value {", f, len(e.Params))
	g.buf.WriteString(code)
	g.buf.WriteString(strings.Repeat("\t", g.indent) + "})\n")
	return f
}

// args evaluates call arguments in written order and returns Go expressions
// for the names slice and the values slice.
func (g *fnGen) args(as []*ast.Arg) (string, string) {
	var es []ast.Expr
	var names []string
	named := false
	for _, a := range as {
		es = append(es, a.Value)
		names = append(names, quote(a.Name))
		if a.Name != "" {
			named = true
		}
	}
	vals := g.exprs(es)
	ns := "nil"
	if named {
		ns = "[]string{" + strings.Join(names, ", ") + "}"
	}
	return ns, "[]rt.Value{" + strings.Join(vals, ", ") + "}"
}

func (g *fnGen) argList(as []*ast.Arg) (string, []string) {
	var es []ast.Expr
	var names []string
	named := false
	for _, a := range as {
		es = append(es, a.Value)
		names = append(names, quote(a.Name))
		if a.Name != "" {
			named = true
		}
	}
	vals := g.exprs(es)
	ns := "nil"
	if named {
		ns = "[]string{" + strings.Join(names, ", ") + "}"
	}
	return ns, vals
}
