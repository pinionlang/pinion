// Package lexer turns Pinion source into tokens.
//
// Comments are not emitted as tokens; they are collected in Result.Comments and
// each token records (via Result.Before) which comments precede it. Newline
// tokens are emitted only where a line break is significant for the parser
// (see significantNewline).
package lexer

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pinionlang/pinion/internal/diag"
	"github.com/pinionlang/pinion/internal/token"
)

// Comment is a `//` or `///` comment.
type Comment struct {
	Pos      token.Pos
	End      token.Pos
	Text     string // without the leading // or ///
	Raw      string // full source text, right-trimmed
	Doc      bool
	Trailing bool // code precedes it on the same line
}

// Result is the output of Lex.
type Result struct {
	Tokens   []token.Token // ends with EOF
	Comments []Comment
	// Before[i] lists indices into Comments that appear between the previous
	// non-newline token and Tokens[i].
	Before [][]int
}

type scanner struct {
	file string
	src  []byte
	i    int
	line int
	col  int
	base int // byte offset of src[0] in the real file
	d    *diag.List

	res        *Result
	pending    []int
	stack      []token.Kind
	pendingNL  bool
	nlPos      token.Pos
	lineHasTok bool
}

// Lex tokenizes a whole file.
func Lex(file string, src []byte, d *diag.List) *Result {
	return LexFrom(token.Pos{File: file, Line: 1, Col: 1}, src, d)
}

// LexFrom tokenizes src as if it began at position start (used for string
// interpolation expressions).
func LexFrom(start token.Pos, src []byte, d *diag.List) *Result {
	s := &scanner{file: start.File, src: src, line: start.Line, col: start.Col, base: start.Off, d: d, res: &Result{}}
	s.run()
	return s.res
}

func (s *scanner) pos() token.Pos {
	return token.Pos{File: s.file, Line: s.line, Col: s.col, Off: s.base + s.i}
}

func (s *scanner) peekRune(k int) rune {
	j := s.i
	for ; k > 0 && j < len(s.src); k-- {
		_, n := utf8.DecodeRune(s.src[j:])
		j += n
	}
	if j >= len(s.src) {
		return 0
	}
	r, _ := utf8.DecodeRune(s.src[j:])
	return r
}

func (s *scanner) cur() rune { return s.peekRune(0) }

func (s *scanner) advance() rune {
	if s.i >= len(s.src) {
		return 0
	}
	r, n := utf8.DecodeRune(s.src[s.i:])
	s.i += n
	if r == '\n' {
		s.line++
		s.col = 1
	} else {
		s.col++
	}
	return r
}

func (s *scanner) emit(t token.Token) {
	if s.pendingNL {
		s.pendingNL = false
		n := len(s.res.Tokens)
		if n > 0 && t.Kind != token.EOF && !suppressAfter(s.res.Tokens[n-1].Kind) && !suppressBefore(t.Kind) && s.newlineSignificant() {
			s.res.Tokens = append(s.res.Tokens, token.Token{Kind: token.Newline, Text: "\n", Pos: s.nlPos, End: s.nlPos})
			s.res.Before = append(s.res.Before, nil)
		}
	}
	s.res.Tokens = append(s.res.Tokens, t)
	s.res.Before = append(s.res.Before, s.pending)
	s.pending = nil
	s.lineHasTok = true
	switch t.Kind {
	case token.LParen, token.LBracket, token.LBrace:
		s.stack = append(s.stack, t.Kind)
	case token.RParen, token.RBracket, token.RBrace:
		if len(s.stack) > 0 {
			s.stack = s.stack[:len(s.stack)-1]
		}
	}
}

func (s *scanner) newlineSignificant() bool {
	return len(s.stack) == 0 || s.stack[len(s.stack)-1] == token.LBrace
}

// suppressAfter: a newline after these tokens never ends a statement.
func suppressAfter(k token.Kind) bool {
	switch k {
	case token.Comma, token.FatArrow, token.Arrow, token.Assign, token.PlusAssign, token.MinusAssign,
		token.StarAssign, token.SlashAssign, token.Plus, token.Minus, token.Star, token.Slash, token.Percent,
		token.Eq, token.NotEq, token.LtEq, token.GtEq, token.AndAnd, token.OrOr, token.QQ, token.Pipe,
		token.Dot, token.DotDot, token.DotDotEq, token.LParen, token.LBracket, token.Colon, token.Newline:
		return true
	}
	return false
}

// suppressBefore: a line starting with these continues the previous line.
func suppressBefore(k token.Kind) bool {
	switch k {
	case token.Dot, token.AndAnd, token.OrOr, token.QQ, token.Pipe, token.Plus, token.Star, token.Slash,
		token.Percent, token.Eq, token.NotEq, token.LtEq, token.GtEq, token.FatArrow, token.Arrow:
		return true
	}
	return false
}

func (s *scanner) run() {
	for {
		s.skipSpace()
		if s.i >= len(s.src) {
			break
		}
		start := s.pos()
		r := s.cur()
		switch {
		case r == '\n':
			s.advance()
			if !s.pendingNL {
				s.pendingNL = true
				s.nlPos = start
			}
			s.lineHasTok = false
			continue
		case r == '/' && s.peekRune(1) == '/':
			s.comment(start)
			continue
		case r == '/' && s.peekRune(1) == '*':
			s.d.Errorf("E0001", start, "block comments /* */ are not supported; use //")
			s.advance()
			s.advance()
			for s.i < len(s.src) && !(s.cur() == '*' && s.peekRune(1) == '/') {
				if s.cur() == '\n' && !s.pendingNL {
					s.pendingNL = true
					s.nlPos = s.pos()
				}
				s.advance()
			}
			if s.i < len(s.src) {
				s.advance()
				s.advance()
			}
			continue
		case r == '"':
			s.str(start)
		case r == '_' || unicode.IsLetter(r):
			for s.i < len(s.src) && (s.cur() == '_' || unicode.IsLetter(s.cur()) || unicode.IsDigit(s.cur())) {
				s.advance()
			}
			text := string(s.src[start.Off-s.base : s.i])
			k := token.Ident
			if kw, ok := token.Keywords[text]; ok {
				k = kw
			}
			s.emit(token.Token{Kind: k, Text: text, Pos: start, End: s.pos()})
		case r >= '0' && r <= '9':
			s.number(start)
		default:
			s.punct(start)
		}
	}
	s.pendingNL = false
	s.res.Tokens = append(s.res.Tokens, token.Token{Kind: token.EOF, Pos: s.pos(), End: s.pos()})
	s.res.Before = append(s.res.Before, s.pending)
	s.pending = nil
}

func (s *scanner) skipSpace() {
	for s.i < len(s.src) {
		c := s.src[s.i]
		if c == ' ' || c == '\t' || c == '\r' {
			s.advance()
			continue
		}
		break
	}
}

func (s *scanner) comment(start token.Pos) {
	j := s.i
	for j < len(s.src) && s.src[j] != '\n' {
		j++
	}
	raw := strings.TrimRight(string(s.src[s.i:j]), " \t\r")
	for s.i < j {
		s.advance()
	}
	c := Comment{Pos: start, End: s.pos(), Raw: raw, Trailing: s.lineHasTok}
	if strings.HasPrefix(raw, "///") && !strings.HasPrefix(raw, "////") {
		c.Doc = true
		c.Text = raw[3:]
	} else {
		c.Text = raw[2:]
	}
	s.res.Comments = append(s.res.Comments, c)
	s.pending = append(s.pending, len(s.res.Comments)-1)
}

func (s *scanner) number(start token.Pos) {
	digits := func() {
		for s.i < len(s.src) && ((s.cur() >= '0' && s.cur() <= '9') || s.cur() == '_') {
			s.advance()
		}
	}
	digits()
	k := token.Int
	if s.cur() == '.' && s.peekRune(1) >= '0' && s.peekRune(1) <= '9' {
		s.advance()
		digits()
		k = token.Float
	}
	if (s.cur() == 'e' || s.cur() == 'E') && k == token.Float {
		n := 1
		if s.peekRune(1) == '+' || s.peekRune(1) == '-' {
			n = 2
		}
		if c := s.peekRune(n); c >= '0' && c <= '9' {
			for ; n > 0; n-- {
				s.advance()
			}
			digits()
		}
	}
	text := string(s.src[start.Off-s.base : s.i])
	if s.cur() == 'd' && !isIdentRune(s.peekRune(1)) {
		s.advance()
		k = token.Dec
	}
	if isIdentRune(s.cur()) {
		s.d.Errorf("E0001", s.pos(), "unexpected character %q after number", s.cur())
		for isIdentRune(s.cur()) {
			s.advance()
		}
	}
	s.emit(token.Token{Kind: k, Text: text, Pos: start, End: s.pos()})
}

func isIdentRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

var punct3 = map[string]token.Kind{"..=": token.DotDotEq}
var punct2 = map[string]token.Kind{
	"..": token.DotDot, "->": token.Arrow, "=>": token.FatArrow, "+=": token.PlusAssign, "-=": token.MinusAssign,
	"*=": token.StarAssign, "/=": token.SlashAssign, "==": token.Eq, "!=": token.NotEq, "<=": token.LtEq,
	">=": token.GtEq, "&&": token.AndAnd, "||": token.OrOr, "??": token.QQ,
}
var punct1 = map[byte]token.Kind{
	'(': token.LParen, ')': token.RParen, '{': token.LBrace, '}': token.RBrace, '[': token.LBracket,
	']': token.RBracket, ',': token.Comma, ':': token.Colon, '.': token.Dot, '=': token.Assign, '+': token.Plus,
	'-': token.Minus, '*': token.Star, '/': token.Slash, '%': token.Percent, '<': token.Lt, '>': token.Gt,
	'!': token.Bang, '?': token.Question, '|': token.Pipe, '\\': token.Backslash,
}

func (s *scanner) punct(start token.Pos) {
	rest := s.src[s.i:]
	try := func(n int, m map[string]token.Kind) bool {
		if len(rest) < n {
			return false
		}
		if k, ok := m[string(rest[:n])]; ok {
			for j := 0; j < n; j++ {
				s.advance()
			}
			s.emit(token.Token{Kind: k, Text: string(rest[:n]), Pos: start, End: s.pos()})
			return true
		}
		return false
	}
	if try(3, punct3) || try(2, punct2) {
		return
	}
	if k, ok := punct1[rest[0]]; ok {
		s.advance()
		s.emit(token.Token{Kind: k, Text: string(rest[:1]), Pos: start, End: s.pos()})
		return
	}
	r := s.advance()
	s.d.Errorf("E0001", start, "unexpected character %q", r)
}

// str scans a string literal starting at the opening quote.
func (s *scanner) str(start token.Pos) {
	s.advance() // "
	var parts []token.StrPart
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			parts = append(parts, token.StrPart{Text: text.String()})
			text.Reset()
		}
	}
	closed := false
	for s.i < len(s.src) {
		r := s.cur()
		if r == '"' {
			s.advance()
			closed = true
			break
		}
		if r == '\n' {
			break
		}
		if r == '\\' {
			epos := s.pos()
			s.advance()
			e := s.advance()
			switch e {
			case 'n':
				text.WriteByte('\n')
			case 't':
				text.WriteByte('\t')
			case 'r':
				text.WriteByte('\r')
			case '0':
				text.WriteByte(0)
			case '"', '\\', '{', '}':
				text.WriteRune(e)
			case 'u':
				if s.cur() == '{' {
					s.advance()
					v := 0
					for s.i < len(s.src) && s.cur() != '}' && s.cur() != '"' {
						c := s.advance()
						switch {
						case c >= '0' && c <= '9':
							v = v*16 + int(c-'0')
						case c >= 'a' && c <= 'f':
							v = v*16 + int(c-'a'+10)
						case c >= 'A' && c <= 'F':
							v = v*16 + int(c-'A'+10)
						}
					}
					if s.cur() == '}' {
						s.advance()
					}
					text.WriteRune(rune(v))
				} else {
					s.d.Errorf("E0001", epos, "invalid escape \\u")
				}
			default:
				s.d.Errorf("E0001", epos, "invalid escape \\%c", e)
			}
			continue
		}
		if r == '{' {
			saveI, saveLine, saveCol := s.i, s.line, s.col
			s.advance()
			epos := s.pos()
			startI := s.i
			depth := 0
			literal := false
			for s.i < len(s.src) {
				c := s.cur()
				if c == '\n' {
					break
				}
				if c == '\\' {
					// No special case: a literal brace must be written as \{ (DESIGN 3.15.2).
					s.d.Errorf("E0001", epos, "escape sequence inside string interpolation; write \\{ for a literal brace")
					literal = true
					break
				}
				if c == '"' {
					s.skipNestedString()
					continue
				}
				if c == '{' {
					depth++
				} else if c == '}' {
					if depth == 0 {
						break
					}
					depth--
				}
				s.advance()
			}
			if literal {
				// `{` followed by an escape (e.g. JSON text "{\"a\":1}") is literal text.
				s.i, s.line, s.col = saveI, saveLine, saveCol
				text.WriteRune(s.advance())
				continue
			}
			flush()
			expr := string(s.src[startI:s.i])
			if s.cur() == '}' {
				s.advance()
			} else {
				s.d.Errorf("E0001", epos, "unterminated interpolation")
			}
			parts = append(parts, token.StrPart{IsExpr: true, Expr: expr, ExprPos: epos})
			continue
		}
		text.WriteRune(s.advance())
	}
	flush()
	if !closed {
		s.d.Errorf("E0001", start, "unterminated string literal")
	}
	s.emit(token.Token{Kind: token.String, Text: string(s.src[start.Off-s.base : s.i]), Pos: start, End: s.pos(), Parts: parts})
}

// skipNestedString skips a string literal inside an interpolation, including
// its own interpolations.
func (s *scanner) skipNestedString() {
	s.advance()
	for s.i < len(s.src) {
		c := s.cur()
		switch c {
		case '\n':
			return
		case '"':
			s.advance()
			return
		case '\\':
			s.advance()
			s.advance()
		case '{':
			s.advance()
			depth := 0
			for s.i < len(s.src) && s.cur() != '\n' {
				if s.cur() == '"' {
					s.skipNestedString()
					continue
				}
				if s.cur() == '{' {
					depth++
				} else if s.cur() == '}' {
					if depth == 0 {
						break
					}
					depth--
				}
				s.advance()
			}
			if s.cur() == '}' {
				s.advance()
			}
		default:
			s.advance()
		}
	}
}
