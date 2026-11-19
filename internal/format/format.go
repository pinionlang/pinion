// Package format implements `pinion fmt`.
//
// The formatter works on the token stream (plus parser hints) rather than
// re-printing the AST, so every comment keeps its place and doc comments stay
// attached to the declaration that follows them. Line breaks chosen by the
// author are kept (consecutive blank lines collapse to one); everything else —
// indentation (2 spaces per open bracket line), spacing around tokens,
// trailing whitespace — has exactly one canonical form.
package format

import (
	"strings"

	"github.com/pinionlang/pinion/internal/diag"
	"github.com/pinionlang/pinion/internal/lexer"
	"github.com/pinionlang/pinion/internal/parser"
	"github.com/pinionlang/pinion/internal/token"
)

// Format returns the canonical form of src. If src has syntax errors, src is
// returned unchanged together with the diagnostics.
func Format(path string, src []byte) ([]byte, *diag.List) {
	_, d, info := parser.ParseWithInfo(path, src)
	if d.HasErrors() {
		return src, d
	}
	lx := lexer.Lex(path, src, &diag.List{})
	return []byte(render(lx, info)), d
}

type item struct {
	tok     *token.Token
	comment *lexer.Comment
	line    int // start line
	endLine int
	off     int
	member  bool // token follows `.` (keywords used as member names)
}

type opener struct {
	kind   token.Kind
	indent int
}

func render(lx *lexer.Result, info *parser.Info) string {
	// Merge tokens and comments in source order.
	var items []item
	ci := 0
	for ti := range lx.Tokens {
		t := &lx.Tokens[ti]
		for ci < len(lx.Comments) && lx.Comments[ci].Pos.Off < t.Pos.Off {
			c := &lx.Comments[ci]
			items = append(items, item{comment: c, line: c.Pos.Line, endLine: c.Pos.Line, off: c.Pos.Off})
			ci++
		}
		if t.Kind == token.Newline || t.Kind == token.EOF {
			continue
		}
		member := ti > 0 && lx.Tokens[ti-1].Kind == token.Dot
		items = append(items, item{tok: t, line: t.Pos.Line, endLine: t.End.Line, off: t.Pos.Off, member: member})
	}
	for ; ci < len(lx.Comments); ci++ {
		c := &lx.Comments[ci]
		items = append(items, item{comment: c, line: c.Pos.Line, endLine: c.Pos.Line, off: c.Pos.Off})
	}

	// Group into lines.
	var lines [][]item
	var gaps []bool // blank line before this line in source
	lastEnd := 0
	for _, it := range items {
		if len(lines) == 0 || it.line > lastEnd {
			gaps = append(gaps, len(lines) > 0 && it.line > lastEnd+1)
			lines = append(lines, nil)
		}
		lines[len(lines)-1] = append(lines[len(lines)-1], it)
		if it.endLine > lastEnd {
			lastEnd = it.endLine
		}
	}

	var out strings.Builder
	var stack []opener
	var prevLast *item
	for li, ln := range lines {
		// Leading closers pop before computing indentation.
		k := 0
		for k < len(ln) && ln[k].tok != nil && isCloser(ln[k].tok.Kind) {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			k++
		}
		indent := 0
		if len(stack) > 0 {
			indent = stack[len(stack)-1].indent + 1
		}
		if continues(prevLast, ln[0]) {
			indent++
		}
		if li > 0 && gaps[li] && !(prevLast != nil && prevLast.tok != nil && isOpener(prevLast.tok.Kind)) &&
			!(ln[0].tok != nil && isCloser(ln[0].tok.Kind)) {
			out.WriteByte('\n')
		}
		out.WriteString(strings.Repeat("  ", indent))
		var prev *item
		for j := range ln {
			it := &ln[j]
			if prev != nil && space(prev, it, info) {
				out.WriteByte(' ')
			}
			if it.comment != nil {
				out.WriteString(it.comment.Raw)
			} else {
				out.WriteString(it.tok.Text)
				if j >= k {
					switch {
					case isOpener(it.tok.Kind):
						stack = append(stack, opener{it.tok.Kind, indent})
					case isCloser(it.tok.Kind):
						if len(stack) > 0 {
							stack = stack[:len(stack)-1]
						}
					}
				}
			}
			prev = it
		}
		out.WriteByte('\n')
		// The last code item decides continuation; a trailing comment does not.
		prevLast = nil
		for j := len(ln) - 1; j >= 0; j-- {
			if ln[j].tok != nil {
				prevLast = &ln[j]
				break
			}
		}
		if prevLast == nil {
			prevLast = &ln[len(ln)-1]
		}
	}
	return out.String()
}

func isOpener(k token.Kind) bool {
	return k == token.LParen || k == token.LBracket || k == token.LBrace
}

func isCloser(k token.Kind) bool {
	return k == token.RParen || k == token.RBracket || k == token.RBrace
}

// continues reports whether a line is a continuation of the previous one
// (method chain or a dangling binary operator), which gets one extra indent.
func continues(prev *item, first item) bool {
	if first.tok != nil {
		switch first.tok.Kind {
		case token.Dot, token.AndAnd, token.OrOr, token.QQ, token.Pipe, token.Plus, token.Star, token.Slash,
			token.Percent, token.Eq, token.NotEq, token.LtEq, token.GtEq:
			return true
		}
	}
	if prev == nil || prev.tok == nil || first.tok == nil {
		return false
	}
	switch prev.tok.Kind {
	case token.FatArrow, token.Arrow, token.Assign, token.PlusAssign, token.MinusAssign, token.StarAssign,
		token.SlashAssign, token.Plus, token.Minus, token.Star, token.Slash, token.Percent, token.Eq, token.NotEq,
		token.LtEq, token.GtEq, token.AndAnd, token.OrOr, token.QQ, token.Pipe:
		return true
	}
	return false
}

func space(a, b *item, info *parser.Info) bool {
	if b.comment != nil {
		return true
	}
	if a.comment != nil {
		return true
	}
	x, y := a.tok, b.tok
	if info.PathSlash[x.Pos.Off] || info.PathSlash[y.Pos.Off] {
		return false
	}
	// no space after
	switch x.Kind {
	case token.LParen, token.LBracket, token.Dot, token.DotDot, token.DotDotEq:
		return false
	case token.Bang, token.Minus:
		if info.Unary[x.Pos.Off] || info.Effect[x.Pos.Off] {
			return false
		}
	case token.Lt:
		if info.Generic[x.Pos.Off] {
			return false
		}
	case token.LBrace:
		if info.MapBrace[x.Pos.Off] {
			return false
		}
	}
	// no space before
	switch y.Kind {
	case token.RParen, token.RBracket, token.Comma, token.Dot, token.Question, token.DotDot, token.DotDotEq:
		return false
	case token.Colon:
		return info.ImplColon[y.Pos.Off]
	case token.Lt, token.Gt:
		if info.Generic[y.Pos.Off] {
			return false
		}
	case token.RBrace:
		if info.MapBrace[y.Pos.Off] {
			return false
		}
	case token.LParen:
		if a.member {
			return false
		}
		switch x.Kind {
		case token.Ident, token.RParen, token.RBracket, token.KwFn, token.KwInit, token.String, token.KwSelf, token.KwSelfType:
			return false
		case token.Gt:
			return !info.Generic[x.Pos.Off]
		}
	case token.LBracket:
		switch x.Kind {
		case token.Ident, token.RParen, token.RBracket, token.String, token.KwSelf:
			return false
		}
	}
	return true
}
