// Package token defines source positions and lexical tokens for Pinion.
package token

import "fmt"

// Pos is a source position. Line and Col are 1-based; Col counts Unicode code points.
// Off is the 0-based byte offset.
type Pos struct {
	File string
	Line int
	Col  int
	Off  int
}

func (p Pos) String() string { return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Col) }

// IsValid reports whether the position has been set.
func (p Pos) IsValid() bool { return p.Line > 0 }

// Span is a half-open source range.
type Span struct {
	Start Pos
	End   Pos
}

type Kind int

const (
	EOF Kind = iota
	Illegal
	Newline // significant line break (statement separator)

	Ident
	Int    // 123
	Float  // 1.5
	Dec    // 1.50d
	String // "..." (raw contents incl. interpolation; see Token.Parts)
	DocComment

	// keywords
	KwFn
	KwData
	KwClass
	KwInterface
	KwEnum
	KwType
	KwTest
	KwLet
	KwVar
	KwMut
	KwPub
	KwInit
	KwIf
	KwElse
	KwMatch
	KwFor
	KwIn
	KwWhile
	KwReturn
	KwBreak
	KwContinue
	KwTrue
	KwFalse
	KwImport
	KwAs
	KwWith
	KwSelf     // self
	KwSelfType // Self

	// punctuation / operators
	LParen
	RParen
	LBrace
	RBrace
	LBracket
	RBracket
	Comma
	Colon
	Dot
	DotDot   // ..
	DotDotEq // ..=
	Arrow    // ->
	FatArrow // =>
	Assign   // =
	PlusAssign
	MinusAssign
	StarAssign
	SlashAssign
	Plus
	Minus
	Star
	Slash
	Percent
	Eq    // ==
	NotEq // !=
	Lt
	Gt
	LtEq
	GtEq
	AndAnd
	OrOr
	Bang     // ! (not, or effect marker)
	Question // ? (postfix try, or optional type)
	QQ       // ??
	Pipe     // | (or-pattern)
	Backslash
)

var names = map[Kind]string{
	EOF: "EOF", Illegal: "illegal", Newline: "newline", Ident: "identifier", Int: "integer",
	Float: "float", Dec: "decimal", String: "string", DocComment: "doc comment",
	KwFn: "fn", KwData: "data", KwClass: "class", KwInterface: "interface", KwEnum: "enum",
	KwType: "type", KwTest: "test", KwLet: "let", KwVar: "var", KwMut: "mut", KwPub: "pub",
	KwInit: "init", KwIf: "if", KwElse: "else", KwMatch: "match", KwFor: "for", KwIn: "in",
	KwWhile: "while", KwReturn: "return", KwBreak: "break", KwContinue: "continue",
	KwTrue: "true", KwFalse: "false", KwImport: "import", KwAs: "as", KwWith: "with",
	KwSelf: "self", KwSelfType: "Self",
	LParen: "(", RParen: ")", LBrace: "{", RBrace: "}", LBracket: "[", RBracket: "]",
	Comma: ",", Colon: ":", Dot: ".", DotDot: "..", DotDotEq: "..=", Arrow: "->", FatArrow: "=>",
	Assign: "=", PlusAssign: "+=", MinusAssign: "-=", StarAssign: "*=", SlashAssign: "/=",
	Plus: "+", Minus: "-", Star: "*", Slash: "/", Percent: "%", Eq: "==", NotEq: "!=",
	Lt: "<", Gt: ">", LtEq: "<=", GtEq: ">=", AndAnd: "&&", OrOr: "||", Bang: "!",
	Question: "?", QQ: "??", Pipe: "|", Backslash: "\\",
}

func (k Kind) String() string {
	if s, ok := names[k]; ok {
		return s
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

var Keywords = map[string]Kind{
	"fn": KwFn, "data": KwData, "class": KwClass, "interface": KwInterface, "enum": KwEnum,
	"type": KwType, "test": KwTest, "let": KwLet, "var": KwVar, "mut": KwMut, "pub": KwPub,
	"init": KwInit, "if": KwIf, "else": KwElse, "match": KwMatch, "for": KwFor, "in": KwIn,
	"while": KwWhile, "return": KwReturn, "break": KwBreak, "continue": KwContinue,
	"true": KwTrue, "false": KwFalse, "import": KwImport, "as": KwAs, "with": KwWith,
	"self": KwSelf, "Self": KwSelfType,
}

// StrPart is one piece of a string literal: either literal text or an interpolated
// expression source (Expr != "" means interpolation; ExprPos is where the expr starts).
type StrPart struct {
	Text    string
	IsExpr  bool
	Expr    string
	ExprPos Pos
}

type Token struct {
	Kind  Kind
	Text  string // source text (for idents/numbers) or decoded doc comment text
	Pos   Pos
	End   Pos
	Parts []StrPart // for String
	// SpaceBefore reports whether whitespace/newline preceded this token.
	SpaceBefore bool
}
