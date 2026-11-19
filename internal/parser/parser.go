// Package parser is a hand-written recursive-descent + Pratt parser for Pinion.
package parser

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pinionlang/pinion/internal/ast"
	"github.com/pinionlang/pinion/internal/diag"
	"github.com/pinionlang/pinion/internal/lexer"
	"github.com/pinionlang/pinion/internal/token"
)

// Info records token roles the formatter cannot infer from tokens alone.
// Keys are byte offsets of tokens.
type Info struct {
	MapBrace  map[int]bool // `{` / `}` of map literals
	Generic   map[int]bool // `<` / `>` used as type brackets
	ImplColon map[int]bool // `:` in `data X : Iface`
	Unary     map[int]bool // unary `-` / `!`
	Effect    map[int]bool // `!` of an effect marker
	PathSlash map[int]bool // `/` inside an import path
}

func newInfo() *Info {
	return &Info{map[int]bool{}, map[int]bool{}, map[int]bool{}, map[int]bool{}, map[int]bool{}, map[int]bool{}}
}

// ParseFile parses one source file.
func ParseFile(path string, src []byte) (*ast.File, *diag.List) {
	f, d, _ := ParseWithInfo(path, src)
	return f, d
}

// ModulePath derives the module path from a file path ("src/todo/store.pi" -> "todo/store").
func ModulePath(path string) string {
	p := strings.ReplaceAll(path, "\\", "/")
	idx := -1
	if strings.HasPrefix(p, "src/") {
		idx = 0
	}
	if j := strings.LastIndex(p, "/src/"); j >= 0 {
		idx = j + 1
	}
	if idx < 0 {
		return ""
	}
	return strings.TrimSuffix(p[idx+4:], ".pi")
}

// ParseWithInfo parses a file and also returns formatter hints.
func ParseWithInfo(path string, src []byte) (*ast.File, *diag.List, *Info) {
	d := &diag.List{}
	lx := lexer.Lex(path, src, d)
	p := &parser{toks: lx.Tokens, before: lx.Before, comments: lx.Comments, docUsed: make([]bool, len(lx.Comments)),
		d: d, info: newInfo(), paren: map[ast.Expr]bool{}}
	f := &ast.File{Path: path, Module: ModulePath(path)}
	for _, c := range lx.Comments {
		f.Comments = append(f.Comments, &ast.Comment{Pos: c.Pos, Text: c.Text, Doc: c.Doc})
	}
	p.parseFile(f)
	for i, c := range lx.Comments {
		if c.Doc && !p.docUsed[i] {
			d.Warnf("W0201", c.Pos, "doc comment is not attached to a declaration")
		}
	}
	return f, d, p.info
}

type parser struct {
	toks     []token.Token
	before   [][]int
	comments []lexer.Comment
	docUsed  []bool
	i        int
	d        *diag.List
	info     *Info
	paren    map[ast.Expr]bool

	silent      int
	failed      bool
	lastErrLine int
	lastErrFile string
}

// ---------- helpers ----------

func (p *parser) tok() token.Token     { return p.toks[p.i] }
func (p *parser) kind() token.Kind     { return p.toks[p.i].Kind }
func (p *parser) at(k token.Kind) bool { return p.toks[p.i].Kind == k }
func (p *parser) peek(n int) token.Token {
	if p.i+n < len(p.toks) {
		return p.toks[p.i+n]
	}
	return p.toks[len(p.toks)-1]
}

func (p *parser) next() token.Token {
	t := p.toks[p.i]
	if p.i < len(p.toks)-1 {
		p.i++
	}
	return t
}

func (p *parser) accept(k token.Kind) bool {
	if p.at(k) {
		p.next()
		return true
	}
	return false
}

func (p *parser) skipNL() {
	for p.at(token.Newline) {
		p.next()
	}
}

func (p *parser) errAt(pos token.Pos, format string, args ...any) {
	if p.silent > 0 {
		p.failed = true
		return
	}
	if pos.Line == p.lastErrLine && pos.File == p.lastErrFile {
		return
	}
	p.lastErrLine, p.lastErrFile = pos.Line, pos.File
	p.d.Errorf("E0001", pos, format, args...)
}

func (p *parser) unexpected(what string) {
	t := p.tok()
	p.errAt(t.Pos, "expected %s, found %s", what, describe(t))
}

func describe(t token.Token) string {
	switch t.Kind {
	case token.Ident:
		return "identifier " + t.Text
	case token.EOF, token.Newline:
		return t.Kind.String()
	}
	if t.Text != "" {
		return "'" + t.Text + "'"
	}
	return t.Kind.String()
}

func (p *parser) expect(k token.Kind) token.Token {
	if p.at(k) {
		return p.next()
	}
	p.unexpected("'" + k.String() + "'")
	return token.Token{Kind: k, Pos: p.tok().Pos}
}

func (p *parser) rec(m map[int]bool, off int) {
	if p.silent == 0 {
		m[off] = true
	}
}

// identName accepts an identifier (or keyword when kwOK).
func (p *parser) ident(kwOK bool) (string, token.Pos) {
	t := p.tok()
	if t.Kind == token.Ident || (kwOK && t.Kind >= token.KwFn && t.Kind <= token.KwSelfType) {
		p.next()
		return t.Text, t.Pos
	}
	p.unexpected("identifier")
	return "", t.Pos
}

// sync skips to the next newline at bracket depth 0, or a closing brace that
// would close the enclosing block (not consumed).
func (p *parser) sync() {
	depth := 0
	start := p.i
	for !p.at(token.EOF) {
		switch p.kind() {
		case token.Newline:
			if depth == 0 {
				return
			}
		case token.LParen, token.LBracket, token.LBrace:
			depth++
		case token.RParen, token.RBracket, token.RBrace:
			if depth == 0 {
				if p.i == start && p.kind() != token.RBrace {
					p.next()
					continue
				}
				return
			}
			depth--
		}
		p.next()
	}
}

// endItem requires a newline (or one of the closers) after an item.
func (p *parser) endItem(closers ...token.Kind) {
	if p.at(token.Newline) || p.at(token.EOF) {
		p.skipNL()
		return
	}
	for _, k := range closers {
		if p.at(k) {
			return
		}
	}
	p.unexpected("newline")
	p.sync()
	p.skipNL()
}

func (p *parser) takeDoc(idx int) string {
	if idx >= len(p.before) {
		return ""
	}
	cs := p.before[idx]
	var docs []int
	for j := len(cs) - 1; j >= 0; j-- {
		c := p.comments[cs[j]]
		if !c.Doc || c.Trailing {
			break
		}
		docs = append([]int{cs[j]}, docs...)
	}
	if len(docs) == 0 || p.silent > 0 {
		return ""
	}
	var lines []string
	for _, j := range docs {
		p.docUsed[j] = true
		lines = append(lines, strings.TrimPrefix(p.comments[j].Text, " "))
	}
	return strings.Join(lines, "\n")
}

// ---------- declarations ----------

func (p *parser) parseFile(f *ast.File) {
	for {
		p.skipNL()
		if p.at(token.EOF) {
			return
		}
		start := p.i
		p.parseTopDecl(f)
		p.endItem()
		if p.i == start {
			p.next()
		}
	}
}

func (p *parser) parseTopDecl(f *ast.File) {
	startIdx := p.i
	startPos := p.tok().Pos
	doc := p.takeDoc(startIdx)
	pub := p.accept(token.KwPub)
	switch p.kind() {
	case token.KwImport:
		p.next()
		imp := &ast.Import{Pos: startPos}
		name, _ := p.ident(true)
		path := name
		for p.at(token.Slash) {
			p.rec(p.info.PathSlash, p.next().Pos.Off)
			n, _ := p.ident(true)
			path += "/" + n
		}
		imp.Path = path
		if p.accept(token.KwAs) {
			imp.Alias, _ = p.ident(false)
		}
		if p.at(token.LBrace) {
			if p.silent == 0 {
				p.d.Errorf("E0104", p.tok().Pos, "selective import is not supported; import the module and qualify names")
			}
			depth := 0
			for !p.at(token.EOF) {
				if p.at(token.LBrace) {
					depth++
				} else if p.at(token.RBrace) {
					depth--
					if depth == 0 {
						p.next()
						break
					}
				}
				p.next()
			}
		}
		f.Imports = append(f.Imports, imp)
	case token.KwFn:
		f.Decls = append(f.Decls, p.parseFn(startPos, doc, pub, false, true))
	case token.KwData, token.KwClass:
		f.Decls = append(f.Decls, p.parseTypeDecl(startPos, doc, pub))
	case token.KwInterface:
		f.Decls = append(f.Decls, p.parseInterface(startPos, doc, pub))
	case token.KwEnum:
		f.Decls = append(f.Decls, p.parseEnum(startPos, doc, pub))
	case token.KwType:
		p.next()
		a := &ast.AliasDecl{Pos: startPos, Doc: doc, Pub: pub}
		a.Name, _ = p.ident(false)
		a.TypeParams = p.parseTypeParams()
		p.expect(token.Assign)
		a.Type = p.parseType()
		f.Decls = append(f.Decls, a)
	case token.KwTest:
		p.next()
		td := &ast.TestDecl{Pos: startPos}
		if p.at(token.String) {
			t := p.next()
			for _, part := range t.Parts {
				td.Name += part.Text
			}
		} else {
			p.unexpected("test name string")
		}
		td.Effects = p.parseEffects()
		td.Body = p.parseBlock()
		f.Decls = append(f.Decls, td)
	case token.KwLet, token.KwVar:
		ld := &ast.LetDecl{Pos: startPos, Doc: doc, Pub: pub, Var: p.at(token.KwVar)}
		p.next()
		ld.Name, _ = p.ident(false)
		if p.accept(token.Colon) {
			ld.Type = p.parseType()
		}
		p.expect(token.Assign)
		ld.Value = p.parseExpr()
		f.Decls = append(f.Decls, ld)
	default:
		p.unexpected("declaration")
		p.sync()
	}
}

func (p *parser) parseTypeParams() []*ast.TypeParam {
	if !p.at(token.Lt) {
		return nil
	}
	p.rec(p.info.Generic, p.next().Pos.Off)
	var tps []*ast.TypeParam
	for !p.at(token.Gt) && !p.at(token.EOF) {
		name, pos := p.ident(false)
		tp := &ast.TypeParam{Pos: pos, Name: name}
		if p.accept(token.Colon) {
			for {
				c, _ := p.ident(false)
				tp.Constraints = append(tp.Constraints, c)
				if !p.accept(token.Plus) {
					break
				}
			}
		}
		tps = append(tps, tp)
		if !p.accept(token.Comma) {
			break
		}
	}
	if p.at(token.Gt) {
		p.rec(p.info.Generic, p.next().Pos.Off)
	} else {
		p.unexpected("'>'")
	}
	return tps
}

func (p *parser) parseEffects() []ast.Effect {
	var effs []ast.Effect
	for p.at(token.Bang) {
		p.rec(p.info.Effect, p.next().Pos.Off)
		name, pos := p.ident(true)
		effs = append(effs, ast.Effect{Pos: pos, Name: name})
	}
	return effs
}

func (p *parser) parseParams() []*ast.Param {
	p.expect(token.LParen)
	var ps []*ast.Param
	for !p.at(token.RParen) && !p.at(token.EOF) {
		p.skipNL()
		name, pos := p.ident(true)
		prm := &ast.Param{Pos: pos, Name: name}
		p.expect(token.Colon)
		prm.Type = p.parseType()
		ps = append(ps, prm)
		p.skipNL()
		if !p.accept(token.Comma) {
			break
		}
		p.skipNL()
	}
	p.expect(token.RParen)
	return ps
}

// parseFn parses `fn name<T>(params) -> Ret !eff { body }` (current token: fn).
func (p *parser) parseFn(pos token.Pos, doc string, pub, mut, needBody bool) *ast.FnDecl {
	p.expect(token.KwFn)
	fd := &ast.FnDecl{Pos: pos, Doc: doc, Pub: pub, Mut: mut}
	fd.Name, _ = p.ident(true)
	fd.TypeParams = p.parseTypeParams()
	p.parseSigRest(fd)
	if p.at(token.LBrace) {
		fd.Body = p.parseBlock()
	} else if needBody {
		p.unexpected("'{'")
	}
	return fd
}

func (p *parser) parseSigRest(fd *ast.FnDecl) {
	fd.Params = p.parseParams()
	if p.accept(token.Arrow) {
		fd.Ret = p.parseType()
	}
	fd.Effects = p.parseEffects()
}

func (p *parser) parseTypeDecl(pos token.Pos, doc string, pub bool) *ast.TypeDecl {
	td := &ast.TypeDecl{Pos: pos, Doc: doc, Pub: pub, Kind: ast.KindData}
	if p.next().Kind == token.KwClass {
		td.Kind = ast.KindClass
	}
	td.Name, _ = p.ident(false)
	td.TypeParams = p.parseTypeParams()
	if p.at(token.Colon) {
		p.rec(p.info.ImplColon, p.next().Pos.Off)
		for {
			td.Implements = append(td.Implements, p.parseType())
			if !p.accept(token.Comma) {
				break
			}
		}
	}
	p.expect(token.LBrace)
	for {
		p.skipNL()
		if p.at(token.RBrace) || p.at(token.EOF) {
			break
		}
		start := p.i
		mpos := p.tok().Pos
		mdoc := p.takeDoc(p.i)
		mpub := p.accept(token.KwPub)
		mmut := p.accept(token.KwMut)
		switch p.kind() {
		case token.KwFn:
			td.Methods = append(td.Methods, p.parseFn(mpos, mdoc, mpub, mmut, true))
		case token.KwInit:
			p.next()
			fd := &ast.FnDecl{Pos: mpos, Doc: mdoc, Pub: mpub, IsInit: true, Name: "init"}
			p.parseSigRest(fd)
			fd.Body = p.parseBlock()
			td.Init = fd
		case token.Ident:
			fl := &ast.Field{Pos: mpos, Doc: mdoc, Pub: mpub, Mut: mmut}
			fl.Name, _ = p.ident(false)
			p.expect(token.Colon)
			fl.Type = p.parseType()
			if p.accept(token.Assign) {
				fl.Default = p.parseExpr()
			}
			td.Fields = append(td.Fields, fl)
		default:
			p.unexpected("field or method")
			p.sync()
		}
		p.endItem(token.RBrace)
		if p.i == start {
			p.next()
		}
	}
	p.expect(token.RBrace)
	return td
}

func (p *parser) parseInterface(pos token.Pos, doc string, pub bool) *ast.InterfaceDecl {
	p.next()
	id := &ast.InterfaceDecl{Pos: pos, Doc: doc, Pub: pub}
	id.Name, _ = p.ident(false)
	id.TypeParams = p.parseTypeParams()
	p.expect(token.LBrace)
	for {
		p.skipNL()
		if p.at(token.RBrace) || p.at(token.EOF) {
			break
		}
		start := p.i
		mpos := p.tok().Pos
		mdoc := p.takeDoc(p.i)
		mpub := p.accept(token.KwPub)
		mmut := p.accept(token.KwMut)
		if p.at(token.KwFn) {
			id.Methods = append(id.Methods, p.parseFn(mpos, mdoc, mpub, mmut, false))
		} else {
			p.unexpected("method signature")
			p.sync()
		}
		p.endItem(token.RBrace)
		if p.i == start {
			p.next()
		}
	}
	p.expect(token.RBrace)
	return id
}

func (p *parser) parseEnum(pos token.Pos, doc string, pub bool) *ast.EnumDecl {
	p.next()
	ed := &ast.EnumDecl{Pos: pos, Doc: doc, Pub: pub}
	ed.Name, _ = p.ident(false)
	ed.TypeParams = p.parseTypeParams()
	p.expect(token.LBrace)
	for {
		p.skipNL()
		if p.at(token.RBrace) || p.at(token.EOF) {
			break
		}
		start := p.i
		v := &ast.Variant{Pos: p.tok().Pos, Doc: p.takeDoc(p.i)}
		v.Name, _ = p.ident(false)
		if p.accept(token.LParen) {
			for !p.at(token.RParen) && !p.at(token.EOF) {
				vf := &ast.VariantField{Pos: p.tok().Pos}
				if p.at(token.Ident) && p.peek(1).Kind == token.Colon {
					vf.Name = p.next().Text
					p.next()
				}
				vf.Type = p.parseType()
				v.Fields = append(v.Fields, vf)
				if !p.accept(token.Comma) {
					break
				}
			}
			p.expect(token.RParen)
		}
		ed.Variants = append(ed.Variants, v)
		if !p.accept(token.Comma) {
			p.endItem(token.RBrace)
		}
		if p.i == start {
			p.next()
		}
	}
	p.expect(token.RBrace)
	return ed
}

// ---------- types ----------

func (p *parser) parseType() ast.TypeExpr {
	t := p.tok()
	var ty ast.TypeExpr
	switch t.Kind {
	case token.LParen:
		p.next()
		var elems []ast.TypeExpr
		trailing := false
		for !p.at(token.RParen) && !p.at(token.EOF) {
			elems = append(elems, p.parseType())
			trailing = false
			if !p.accept(token.Comma) {
				break
			}
			trailing = true
		}
		p.expect(token.RParen)
		if len(elems) == 1 && !trailing {
			ty = elems[0]
		} else {
			ty = &ast.TupleType{Pos: t.Pos, Elems: elems}
		}
	case token.KwFn:
		p.next()
		ft := &ast.FnType{Pos: t.Pos}
		p.expect(token.LParen)
		for !p.at(token.RParen) && !p.at(token.EOF) {
			ft.Params = append(ft.Params, p.parseType())
			if !p.accept(token.Comma) {
				break
			}
		}
		p.expect(token.RParen)
		if p.accept(token.Arrow) {
			ft.Ret = p.parseType()
		}
		ft.Effects = p.parseEffects()
		ty = ft
	case token.Ident, token.KwSelfType:
		p.next()
		nt := &ast.NamedType{Pos: t.Pos, Name: t.Text}
		if p.at(token.Dot) && p.peek(1).Kind == token.Ident {
			p.next()
			nt.Qualifier = nt.Name
			nt.Name = p.next().Text
		}
		if p.at(token.Lt) {
			nt.Args = p.parseTypeArgs()
		}
		ty = nt
	default:
		p.unexpected("type")
		return &ast.NamedType{Pos: t.Pos, Name: "?"}
	}
	for {
		if p.at(token.Question) {
			q := p.next()
			ty = &ast.OptionalType{Pos: q.Pos, Elem: ty}
		} else if p.at(token.QQ) {
			q := p.next()
			ty = &ast.OptionalType{Pos: q.Pos, Elem: &ast.OptionalType{Pos: q.Pos, Elem: ty}}
		} else {
			break
		}
	}
	return ty
}

func (p *parser) parseTypeArgs() []ast.TypeExpr {
	p.rec(p.info.Generic, p.expect(token.Lt).Pos.Off)
	var args []ast.TypeExpr
	for !p.at(token.Gt) && !p.at(token.EOF) {
		args = append(args, p.parseType())
		if !p.accept(token.Comma) {
			break
		}
	}
	if p.at(token.Gt) {
		p.rec(p.info.Generic, p.next().Pos.Off)
	} else {
		p.unexpected("'>'")
	}
	return args
}

// ---------- statements ----------

func (p *parser) parseBlock() *ast.Block {
	lb := p.expect(token.LBrace)
	b := &ast.Block{Pos: lb.Pos}
	for {
		p.skipNL()
		if p.at(token.RBrace) || p.at(token.EOF) {
			break
		}
		start := p.i
		if s := p.parseStmt(); s != nil {
			b.Stmts = append(b.Stmts, s)
		}
		p.endItem(token.RBrace)
		if p.i == start {
			p.next()
		}
	}
	b.End = p.tok().Pos
	p.expect(token.RBrace)
	return b
}

func isAssignOp(k token.Kind) bool {
	switch k {
	case token.Assign, token.PlusAssign, token.MinusAssign, token.StarAssign, token.SlashAssign:
		return true
	}
	return false
}

func (p *parser) parseStmt() ast.Stmt {
	t := p.tok()
	switch t.Kind {
	case token.KwLet, token.KwVar:
		p.next()
		ls := &ast.LetStmt{Pos: t.Pos, Var: t.Kind == token.KwVar}
		ls.Pat = p.parsePattern()
		if p.accept(token.Colon) {
			ls.Type = p.parseType()
		}
		p.expect(token.Assign)
		ls.Value = p.parseExpr()
		if p.accept(token.KwElse) {
			if p.at(token.LBrace) {
				ls.Else = &ast.BlockExpr{Block: p.parseBlock()}
			} else {
				ls.Else = p.parseExpr()
			}
		}
		return ls
	case token.KwFor:
		p.next()
		fs := &ast.ForStmt{Pos: t.Pos}
		fs.Pat = p.parsePattern()
		p.expect(token.KwIn)
		fs.Iter = p.parseExpr()
		fs.Body = p.parseBlock()
		return fs
	case token.KwWhile:
		p.next()
		ws := &ast.WhileStmt{Pos: t.Pos}
		ws.Cond = p.parseExpr()
		ws.Body = p.parseBlock()
		return ws
	case token.KwWith:
		p.next()
		w := &ast.WithStmt{Pos: t.Pos}
		w.Effect, _ = p.ident(true)
		p.expect(token.Colon)
		w.Handler = p.parseExpr()
		w.Body = p.parseBlock()
		return w
	case token.Ident:
		if t.Text == "assert" {
			switch p.peek(1).Kind {
			case token.Dot, token.Newline, token.RBrace, token.EOF, token.Question:
			default:
				if !isAssignOp(p.peek(1).Kind) {
					p.next()
					return &ast.AssertStmt{Pos: t.Pos, Cond: p.parseExpr()}
				}
			}
		}
	}
	x := p.parseExpr()
	if isAssignOp(p.kind()) {
		op := p.next().Kind
		return &ast.AssignStmt{Pos: t.Pos, Target: x, Op: op, Value: p.parseExpr()}
	}
	return &ast.ExprStmt{Pos: t.Pos, X: x}
}

// ---------- expressions ----------

const (
	precOr = iota + 1
	precAnd
	precCmp
	precQQ
	precRange
	precAdd
	precMul
)

func binPrec(k token.Kind) int {
	switch k {
	case token.OrOr:
		return precOr
	case token.AndAnd:
		return precAnd
	case token.Eq, token.NotEq, token.Lt, token.Gt, token.LtEq, token.GtEq:
		return precCmp
	case token.QQ:
		return precQQ
	case token.DotDot, token.DotDotEq:
		return precRange
	case token.Plus, token.Minus:
		return precAdd
	case token.Star, token.Slash, token.Percent:
		return precMul
	}
	return 0
}

func isArithCmp(k token.Kind) bool {
	p := binPrec(k)
	return p == precCmp || p == precAdd || p == precMul
}

func (p *parser) parseExpr() ast.Expr { return p.parseBinary(1) }

func (p *parser) bareBinary(e ast.Expr) (token.Kind, bool) {
	if b, ok := e.(*ast.BinaryExpr); ok && !p.paren[e] {
		return b.Op, true
	}
	return 0, false
}

func (p *parser) parseBinary(minPrec int) ast.Expr {
	x := p.parseUnary()
	for {
		opTok := p.tok()
		prec := binPrec(opTok.Kind)
		if prec == 0 || prec < minPrec {
			return x
		}
		p.next()
		p.skipNL()
		y := p.parseBinary(prec + 1)
		if prec == precRange {
			x = &ast.RangeExpr{Pos: x.GetPos(), Lo: x, Hi: y, Inclusive: opTok.Kind == token.DotDotEq}
			continue
		}
		mixed := false
		for _, side := range []ast.Expr{x, y} {
			if k, ok := p.bareBinary(side); ok {
				if (opTok.Kind == token.QQ && isArithCmp(k)) || (isArithCmp(opTok.Kind) && k == token.QQ) {
					mixed = true
				}
			}
		}
		if mixed && p.silent == 0 {
			p.d.Errorf("E0102", opTok.Pos, "'??' mixed with arithmetic or comparison operators needs parentheses, e.g. (a ?? 0) + 1")
		}
		x = &ast.BinaryExpr{Pos: x.GetPos(), Op: opTok.Kind, X: x, Y: y}
	}
}

func (p *parser) parseUnary() ast.Expr {
	t := p.tok()
	if t.Kind == token.Minus || t.Kind == token.Bang {
		p.next()
		p.rec(p.info.Unary, t.Pos.Off)
		return &ast.UnaryExpr{Pos: t.Pos, Op: t.Kind, X: p.parseUnary()}
	}
	x := p.parsePrimary()
	if _, ok := x.(*ast.LambdaExpr); ok {
		return x
	}
	return p.parsePostfix(x)
}

func (p *parser) parseArgs() []*ast.Arg {
	p.expect(token.LParen)
	var args []*ast.Arg
	for {
		p.skipNL()
		if p.at(token.RParen) || p.at(token.EOF) {
			break
		}
		a := &ast.Arg{Pos: p.tok().Pos}
		if (p.at(token.Ident) || isKeyword(p.kind())) && p.peek(1).Kind == token.Colon {
			a.Name = p.next().Text
			p.next()
		}
		a.Value = p.parseExpr()
		args = append(args, a)
		p.skipNL()
		if !p.accept(token.Comma) {
			break
		}
	}
	p.skipNL()
	p.expect(token.RParen)
	return args
}

func isKeyword(k token.Kind) bool { return k >= token.KwFn && k <= token.KwSelfType }

// tryTypeArgsCall reports whether `<types>(` follows.
func (p *parser) tryTypeArgsCall() bool {
	save, saveFailed := p.i, p.failed
	p.silent++
	p.failed = false
	p.parseTypeArgs()
	ok := !p.failed && p.at(token.LParen)
	p.silent--
	p.i, p.failed = save, saveFailed
	return ok
}

func (p *parser) parsePostfix(x ast.Expr) ast.Expr {
	for {
		switch p.kind() {
		case token.Dot:
			p.next()
			t := p.tok()
			if t.Kind == token.Ident || isKeyword(t.Kind) || t.Kind == token.Int {
				p.next()
				x = &ast.MemberExpr{Pos: x.GetPos(), X: x, Name: t.Text}
			} else {
				p.unexpected("member name")
				return x
			}
		case token.LParen:
			x = &ast.CallExpr{Pos: x.GetPos(), Fn: x, Args: p.parseArgs()}
		case token.LBracket:
			p.next()
			idx := p.parseExpr()
			p.expect(token.RBracket)
			x = &ast.IndexExpr{Pos: x.GetPos(), X: x, Index: idx}
		case token.Question:
			p.next()
			x = &ast.TryExpr{Pos: x.GetPos(), X: x}
		case token.Lt:
			switch x.(type) {
			case *ast.Ident, *ast.MemberExpr:
			default:
				return x
			}
			if !p.tryTypeArgsCall() {
				return x
			}
			targs := p.parseTypeArgs()
			x = &ast.CallExpr{Pos: x.GetPos(), Fn: x, TypeArgs: targs, Args: p.parseArgs()}
		default:
			return x
		}
	}
}

// isLambdaParen reports whether the `(` at the current token starts lambda params.
func (p *parser) isLambdaParen() bool {
	depth := 0
	for j := p.i; j < len(p.toks); j++ {
		switch p.toks[j].Kind {
		case token.LParen, token.LBracket, token.LBrace:
			depth++
		case token.RParen, token.RBracket, token.RBrace:
			depth--
			if depth == 0 {
				return j+1 < len(p.toks) && p.toks[j+1].Kind == token.FatArrow
			}
		case token.EOF:
			return false
		}
	}
	return false
}

func (p *parser) parseLambdaBody() ast.Expr {
	p.skipNL()
	if p.at(token.LBrace) {
		return &ast.BlockExpr{Block: p.parseBlock()}
	}
	start := p.tok().Pos
	body := p.parseExpr()
	if isAssignOp(p.kind()) {
		op := p.next().Kind
		as := &ast.AssignStmt{Pos: start, Target: body, Op: op, Value: p.parseExpr()}
		return &ast.BlockExpr{Block: &ast.Block{Pos: start, End: start, Stmts: []ast.Stmt{as}}}
	}
	return body
}

func (p *parser) parsePrimary() ast.Expr {
	t := p.tok()
	switch t.Kind {
	case token.Int:
		p.next()
		return &ast.IntLit{Pos: t.Pos, Text: t.Text}
	case token.Float:
		p.next()
		return &ast.FloatLit{Pos: t.Pos, Text: t.Text}
	case token.Dec:
		p.next()
		return &ast.DecLit{Pos: t.Pos, Text: t.Text}
	case token.KwTrue, token.KwFalse:
		p.next()
		return &ast.BoolLit{Pos: t.Pos, Value: t.Kind == token.KwTrue}
	case token.String:
		p.next()
		return p.strLit(t)
	case token.Ident:
		p.next()
		// `if x => ...` in a match guard: x is the guard, not a lambda parameter.
		afterIf := p.i >= 2 && p.toks[p.i-2].Kind == token.KwIf
		if p.at(token.FatArrow) && !afterIf {
			p.next()
			return &ast.LambdaExpr{Pos: t.Pos, Params: []*ast.Param{{Pos: t.Pos, Name: t.Text}}, Body: p.parseLambdaBody()}
		}
		return &ast.Ident{Pos: t.Pos, Name: t.Text}
	case token.KwSelf:
		p.next()
		return &ast.Ident{Pos: t.Pos, Name: "self"}
	case token.KwSelfType:
		p.next()
		if !p.at(token.LBrace) {
			return &ast.Ident{Pos: t.Pos, Name: "Self"}
		}
		p.next()
		sl := &ast.SelfLit{Pos: t.Pos}
		for {
			p.skipNL()
			if p.at(token.RBrace) || p.at(token.EOF) {
				break
			}
			a := &ast.Arg{Pos: p.tok().Pos}
			a.Name, _ = p.ident(true)
			p.expect(token.Colon)
			a.Value = p.parseExpr()
			sl.Fields = append(sl.Fields, a)
			p.skipNL()
			if !p.accept(token.Comma) {
				break
			}
		}
		p.skipNL()
		p.expect(token.RBrace)
		return sl
	case token.LParen:
		if p.isLambdaParen() {
			p.next()
			lam := &ast.LambdaExpr{Pos: t.Pos}
			for !p.at(token.RParen) && !p.at(token.EOF) {
				name, pos := p.ident(false)
				prm := &ast.Param{Pos: pos, Name: name}
				if p.accept(token.Colon) {
					prm.Type = p.parseType()
				}
				lam.Params = append(lam.Params, prm)
				if !p.accept(token.Comma) {
					break
				}
			}
			p.expect(token.RParen)
			p.expect(token.FatArrow)
			lam.Body = p.parseLambdaBody()
			return lam
		}
		p.next()
		p.skipNL()
		if p.accept(token.RParen) {
			return &ast.TupleLit{Pos: t.Pos}
		}
		first := p.parseExpr()
		p.skipNL()
		if p.at(token.Comma) {
			tl := &ast.TupleLit{Pos: t.Pos, Elems: []ast.Expr{first}}
			for p.accept(token.Comma) {
				p.skipNL()
				if p.at(token.RParen) {
					break
				}
				tl.Elems = append(tl.Elems, p.parseExpr())
				p.skipNL()
			}
			p.expect(token.RParen)
			return tl
		}
		p.expect(token.RParen)
		p.paren[first] = true
		return first
	case token.LBracket:
		p.next()
		ll := &ast.ListLit{Pos: t.Pos}
		for {
			p.skipNL()
			if p.at(token.RBracket) || p.at(token.EOF) {
				break
			}
			ll.Elems = append(ll.Elems, p.parseExpr())
			p.skipNL()
			if !p.accept(token.Comma) {
				break
			}
		}
		p.skipNL()
		p.expect(token.RBracket)
		return ll
	case token.LBrace:
		if p.isMapLit() {
			return p.parseMapLit()
		}
		return &ast.BlockExpr{Block: p.parseBlock()}
	case token.KwIf:
		return p.parseIf()
	case token.KwMatch:
		return p.parseMatch()
	case token.KwReturn:
		p.next()
		r := &ast.ReturnExpr{Pos: t.Pos}
		switch p.kind() {
		case token.Newline, token.RBrace, token.RParen, token.RBracket, token.Comma, token.EOF:
		default:
			r.Value = p.parseExpr()
		}
		return r
	case token.KwBreak:
		p.next()
		return &ast.BreakExpr{Pos: t.Pos}
	case token.KwContinue:
		p.next()
		return &ast.ContinueExpr{Pos: t.Pos}
	}
	p.unexpected("expression")
	if !p.at(token.Newline) && !p.at(token.RBrace) && !p.at(token.EOF) && !p.at(token.RParen) {
		p.next()
	}
	return &ast.Ident{Pos: t.Pos, Name: "_"}
}

func (p *parser) isMapLit() bool {
	j := p.i + 1
	for j < len(p.toks) && p.toks[j].Kind == token.Newline {
		j++
	}
	if p.toks[j].Kind == token.RBrace {
		return true
	}
	save, saveFailed := p.i, p.failed
	p.silent++
	p.failed = false
	p.i = j
	p.parseExpr()
	ok := !p.failed && p.at(token.Colon)
	p.silent--
	p.i, p.failed = save, saveFailed
	return ok
}

func (p *parser) parseMapLit() ast.Expr {
	lb := p.next()
	p.rec(p.info.MapBrace, lb.Pos.Off)
	ml := &ast.MapLit{Pos: lb.Pos}
	for {
		p.skipNL()
		if p.at(token.RBrace) || p.at(token.EOF) {
			break
		}
		k := p.parseExpr()
		p.expect(token.Colon)
		v := p.parseExpr()
		ml.Entries = append(ml.Entries, ast.MapEntry{Key: k, Value: v})
		p.skipNL()
		if !p.accept(token.Comma) {
			break
		}
	}
	p.skipNL()
	if p.at(token.RBrace) {
		p.rec(p.info.MapBrace, p.tok().Pos.Off)
	}
	p.expect(token.RBrace)
	return ml
}

func (p *parser) parseIf() ast.Expr {
	t := p.next()
	ie := &ast.IfExpr{Pos: t.Pos}
	ie.Cond = p.parseExpr()
	ie.Then = p.parseBlock()
	j := p.i
	for p.toks[j].Kind == token.Newline {
		j++
	}
	if p.toks[j].Kind == token.KwElse {
		p.i = j
		p.next()
		if p.at(token.KwIf) {
			ie.Else = p.parseIf()
		} else {
			ie.Else = &ast.BlockExpr{Block: p.parseBlock()}
		}
	}
	return ie
}

func (p *parser) parseMatch() ast.Expr {
	t := p.next()
	me := &ast.MatchExpr{Pos: t.Pos}
	me.Subject = p.parseExpr()
	p.expect(token.LBrace)
	for {
		p.skipNL()
		if p.at(token.RBrace) || p.at(token.EOF) {
			break
		}
		start := p.i
		arm := &ast.MatchArm{Pos: p.tok().Pos}
		arm.Pat = p.parsePattern()
		if p.accept(token.KwIf) {
			arm.Guard = p.parseExpr()
		}
		p.expect(token.FatArrow)
		p.skipNL()
		if p.at(token.LBrace) && !p.isMapLit() {
			arm.Body = &ast.BlockExpr{Block: p.parseBlock()}
		} else {
			arm.Body = p.parseExpr()
		}
		me.Arms = append(me.Arms, arm)
		if !p.accept(token.Comma) {
			p.endItem(token.RBrace)
		}
		if p.i == start {
			p.next()
		}
	}
	p.expect(token.RBrace)
	return me
}

// strLit converts a string token, parsing interpolations.
func (p *parser) strLit(t token.Token) ast.Expr {
	sl := &ast.StrLit{Pos: t.Pos}
	for _, part := range t.Parts {
		if !part.IsExpr {
			sl.Parts = append(sl.Parts, ast.StrPart{Text: part.Text})
			continue
		}
		d := p.d
		if p.silent > 0 {
			d = &diag.List{}
		}
		lx := lexer.LexFrom(part.ExprPos, []byte(part.Expr), d)
		sub := &parser{toks: lx.Tokens, before: lx.Before, comments: lx.Comments, docUsed: make([]bool, len(lx.Comments)),
			d: d, info: newInfo(), paren: map[ast.Expr]bool{}, silent: p.silent}
		sub.skipNL()
		var e ast.Expr
		if sub.at(token.EOF) {
			sub.errAt(part.ExprPos, "empty interpolation")
			e = &ast.Ident{Pos: part.ExprPos, Name: "_"}
		} else {
			e = sub.parseExpr()
			sub.skipNL()
			if !sub.at(token.EOF) {
				sub.unexpected("'}'")
			}
		}
		if sub.failed {
			p.failed = true
		}
		sl.Parts = append(sl.Parts, ast.StrPart{Expr: e})
	}
	return sl
}

// ---------- patterns ----------

func (p *parser) parsePattern() ast.Pattern {
	first := p.parsePatAtom()
	if !p.at(token.Pipe) {
		return first
	}
	op := &ast.OrPat{Pos: first.GetPos(), Alts: []ast.Pattern{first}}
	for p.accept(token.Pipe) {
		p.skipNL()
		op.Alts = append(op.Alts, p.parsePatAtom())
	}
	return op
}

func isUpper(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsUpper(r)
}

func (p *parser) parsePatAtom() ast.Pattern {
	t := p.tok()
	switch t.Kind {
	case token.Ident:
		p.next()
		if t.Text == "_" {
			return &ast.WildcardPat{Pos: t.Pos}
		}
		if !isUpper(t.Text) {
			return &ast.BindPat{Pos: t.Pos, Name: t.Text}
		}
		vp := &ast.VariantPat{Pos: t.Pos, Name: t.Text}
		if p.at(token.Dot) && p.peek(1).Kind == token.Ident {
			p.next()
			vp.Qualifier = vp.Name
			vp.Name = p.next().Text
		}
		if p.accept(token.LParen) {
			vp.HasParens = true
			for !p.at(token.RParen) && !p.at(token.EOF) {
				vp.Args = append(vp.Args, p.parsePattern())
				if !p.accept(token.Comma) {
					break
				}
			}
			p.expect(token.RParen)
		}
		return vp
	case token.LParen:
		p.next()
		tp := &ast.TuplePat{Pos: t.Pos}
		trailing := false
		for !p.at(token.RParen) && !p.at(token.EOF) {
			tp.Elems = append(tp.Elems, p.parsePattern())
			trailing = false
			if !p.accept(token.Comma) {
				break
			}
			trailing = true
		}
		p.expect(token.RParen)
		if len(tp.Elems) == 1 && !trailing {
			return tp.Elems[0]
		}
		return tp
	case token.Int, token.Float, token.Dec, token.String, token.KwTrue, token.KwFalse:
		return &ast.LitPat{Pos: t.Pos, Value: p.parsePrimary()}
	case token.Minus:
		p.next()
		p.rec(p.info.Unary, t.Pos.Off)
		return &ast.LitPat{Pos: t.Pos, Value: &ast.UnaryExpr{Pos: t.Pos, Op: token.Minus, X: p.parsePrimary()}}
	}
	p.unexpected("pattern")
	return &ast.WildcardPat{Pos: t.Pos}
}
