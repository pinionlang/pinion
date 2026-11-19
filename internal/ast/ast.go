// Package ast defines the Pinion syntax tree shared by the parser, checker,
// formatter, outline tool and code generator.
//
// Ownership: this package is the contract between components. Add fields
// rather than renaming or removing them, and keep the comments accurate.
package ast

import "github.com/pinionlang/pinion/internal/token"

type Pos = token.Pos

// ---------- files ----------

// File is one source module.
type File struct {
	Path     string // path as given (e.g. "src/todo/store.pi")
	Module   string // module path (e.g. "todo/store"); "" for single-file programs
	Imports  []*Import
	Decls    []Decl
	Comments []*Comment // all comments (for the formatter), in source order
}

type Comment struct {
	Pos  Pos
	Text string // without the leading // or ///
	Doc  bool   // ///
}

type Import struct {
	Pos   Pos
	Path  string // "todo/store", "std/fs"
	Alias string // explicit alias after `as`, or ""
}

// Name returns the identifier used to refer to the import (alias or last segment).
func (i *Import) Name() string {
	if i.Alias != "" {
		return i.Alias
	}
	p := i.Path
	for j := len(p) - 1; j >= 0; j-- {
		if p[j] == '/' {
			return p[j+1:]
		}
	}
	return p
}

// ---------- declarations ----------

type Decl interface {
	declNode()
	GetPos() Pos
}

type TypeParam struct {
	Pos         Pos
	Name        string
	Constraints []string // "Eq", "Ord", "Hash"
}

type Param struct {
	Pos  Pos
	Name string
	Type TypeExpr // may be nil for lambda params
}

// FnDecl is a top-level function, a method, an interface method signature
// (Body == nil) or an init (IsInit).
type FnDecl struct {
	Pos        Pos
	Doc        string
	Pub        bool
	Mut        bool // `mut fn`
	IsInit     bool // `init(...)`
	Name       string
	TypeParams []*TypeParam
	Params     []*Param
	Ret        TypeExpr // nil => ()
	Effects    []Effect
	Body       *Block // nil for interface signatures
}

type Effect struct {
	Pos  Pos
	Name string // "io", "fs", "env", "time", "rand", "db", "net", ...
}

type Field struct {
	Pos     Pos
	Doc     string
	Pub     bool
	Mut     bool
	Name    string
	Type    TypeExpr
	Default Expr // may be nil
}

type TypeKind int

const (
	KindData TypeKind = iota
	KindClass
)

// TypeDecl is `data` or `class`.
type TypeDecl struct {
	Pos        Pos
	Doc        string
	Pub        bool
	Kind       TypeKind
	Name       string
	TypeParams []*TypeParam
	Implements []TypeExpr
	Fields     []*Field
	Methods    []*FnDecl
	Init       *FnDecl // may be nil
}

type InterfaceDecl struct {
	Pos        Pos
	Doc        string
	Pub        bool
	Name       string
	TypeParams []*TypeParam
	Methods    []*FnDecl // Body == nil
}

type VariantField struct {
	Pos  Pos
	Name string // "" for positional payload
	Type TypeExpr
}

type Variant struct {
	Pos    Pos
	Doc    string
	Name   string
	Fields []*VariantField
}

type EnumDecl struct {
	Pos        Pos
	Doc        string
	Pub        bool
	Name       string
	TypeParams []*TypeParam
	Variants   []*Variant
}

type AliasDecl struct {
	Pos        Pos
	Doc        string
	Pub        bool
	Name       string
	TypeParams []*TypeParam
	Type       TypeExpr
}

// LetDecl is a top-level `let` (Var == true for the forbidden top-level `var`).
type LetDecl struct {
	Pos   Pos
	Doc   string
	Pub   bool
	Var   bool
	Name  string
	Type  TypeExpr
	Value Expr
}

type TestDecl struct {
	Pos     Pos
	Name    string
	Effects []Effect // effects that use the real implementation
	Body    *Block
}

func (*FnDecl) declNode()        {}
func (*TypeDecl) declNode()      {}
func (*InterfaceDecl) declNode() {}
func (*EnumDecl) declNode()      {}
func (*AliasDecl) declNode()     {}
func (*LetDecl) declNode()       {}
func (*TestDecl) declNode()      {}

func (d *FnDecl) GetPos() Pos        { return d.Pos }
func (d *TypeDecl) GetPos() Pos      { return d.Pos }
func (d *InterfaceDecl) GetPos() Pos { return d.Pos }
func (d *EnumDecl) GetPos() Pos      { return d.Pos }
func (d *AliasDecl) GetPos() Pos     { return d.Pos }
func (d *LetDecl) GetPos() Pos       { return d.Pos }
func (d *TestDecl) GetPos() Pos      { return d.Pos }

// ---------- type expressions ----------

type TypeExpr interface {
	typeNode()
	GetPos() Pos
}

// NamedType is `Name`, `Name<Args>`, `mod.Name<Args>`.
type NamedType struct {
	Pos       Pos
	Qualifier string // module qualifier or ""
	Name      string // "Int", "List", "User", "Self"
	Args      []TypeExpr
}

type OptionalType struct {
	Pos  Pos
	Elem TypeExpr
}

type FnType struct {
	Pos     Pos
	Params  []TypeExpr
	Ret     TypeExpr // nil => ()
	Effects []Effect
}

// TupleType is `(A, B)`; zero elements is the unit type `()`.
type TupleType struct {
	Pos   Pos
	Elems []TypeExpr
}

func (*NamedType) typeNode()    {}
func (*OptionalType) typeNode() {}
func (*FnType) typeNode()       {}
func (*TupleType) typeNode()    {}

func (t *NamedType) GetPos() Pos    { return t.Pos }
func (t *OptionalType) GetPos() Pos { return t.Pos }
func (t *FnType) GetPos() Pos       { return t.Pos }
func (t *TupleType) GetPos() Pos    { return t.Pos }

// ---------- statements ----------

type Stmt interface {
	stmtNode()
	GetPos() Pos
}

type Block struct {
	Pos   Pos
	End   Pos // position of closing brace
	Stmts []Stmt
	// The value of a block is its last statement when that is an ExprStmt
	// (no trailing semicolons exist in Pinion).
}

// LetStmt: `let pat: T = value` or `let pat = value else <expr-or-block>`.
type LetStmt struct {
	Pos   Pos
	Var   bool // `var`
	Pat   Pattern
	Type  TypeExpr
	Value Expr
	Else  Expr // nil unless let-else; usually *BlockExpr or a Never-typed expr
}

type AssignStmt struct {
	Pos    Pos
	Target Expr       // Ident, MemberExpr, IndexExpr
	Op     token.Kind // Assign, PlusAssign, MinusAssign, StarAssign, SlashAssign
	Value  Expr
}

type ExprStmt struct {
	Pos Pos
	X   Expr
}

type ForStmt struct {
	Pos  Pos
	Pat  Pattern
	Iter Expr
	Body *Block
}

type WhileStmt struct {
	Pos  Pos
	Cond Expr
	Body *Block
}

// WithStmt: `with fs: fs.fake(...) { ... }`
type WithStmt struct {
	Pos     Pos
	Effect  string
	Handler Expr
	Body    *Block
}

// AssertStmt: `assert cond` (statement, no parentheses required).
type AssertStmt struct {
	Pos  Pos
	Cond Expr
}

func (*AssertStmt) stmtNode()     {}
func (s *AssertStmt) GetPos() Pos { return s.Pos }

func (*Block) stmtNode()      {}
func (*LetStmt) stmtNode()    {}
func (*AssignStmt) stmtNode() {}
func (*ExprStmt) stmtNode()   {}
func (*ForStmt) stmtNode()    {}
func (*WhileStmt) stmtNode()  {}
func (*WithStmt) stmtNode()   {}

func (s *Block) GetPos() Pos      { return s.Pos }
func (s *LetStmt) GetPos() Pos    { return s.Pos }
func (s *AssignStmt) GetPos() Pos { return s.Pos }
func (s *ExprStmt) GetPos() Pos   { return s.Pos }
func (s *ForStmt) GetPos() Pos    { return s.Pos }
func (s *WhileStmt) GetPos() Pos  { return s.Pos }
func (s *WithStmt) GetPos() Pos   { return s.Pos }

// ---------- expressions ----------

type Expr interface {
	exprNode()
	GetPos() Pos
}

type Ident struct {
	Pos  Pos
	Name string // includes "self"
}

type IntLit struct {
	Pos  Pos
	Text string
}

type FloatLit struct {
	Pos  Pos
	Text string
}

type DecLit struct {
	Pos  Pos
	Text string // without the trailing d
}

type BoolLit struct {
	Pos   Pos
	Value bool
}

// StrLit parts alternate literal text and interpolated expressions.
type StrLit struct {
	Pos   Pos
	Parts []StrPart
}

type StrPart struct {
	Text string // literal text (already unescaped) when Expr == nil
	Expr Expr
}

type ListLit struct {
	Pos   Pos
	Elems []Expr
}

type MapEntry struct {
	Key   Expr
	Value Expr
}

// MapLit is `{k: v, ...}`; `{}` is an empty map literal in expression position.
type MapLit struct {
	Pos     Pos
	Entries []MapEntry
}

// TupleLit with zero elements is the unit value `()`.
type TupleLit struct {
	Pos   Pos
	Elems []Expr
}

type BinaryExpr struct {
	Pos Pos
	Op  token.Kind // Plus..OrOr, QQ, Eq..GtEq
	X   Expr
	Y   Expr
}

type UnaryExpr struct {
	Pos Pos
	Op  token.Kind // Minus, Bang
	X   Expr
}

type Arg struct {
	Pos   Pos
	Name  string // "" for positional
	Value Expr
}

type CallExpr struct {
	Pos      Pos
	Fn       Expr
	TypeArgs []TypeExpr // `decode<T>(...)`, `MutMap<Str, Int>()`
	Args     []*Arg
}

type MemberExpr struct {
	Pos  Pos
	X    Expr
	Name string
	// OnOption is set by the checker when X has type T? and Name is an option
	// method (is_some, is_none, map). The dynamic backend represents Some(x)
	// as x, so it needs this to dispatch to the option method.
	OnOption bool
}

type IndexExpr struct {
	Pos   Pos
	X     Expr
	Index Expr
}

// TryExpr is postfix `x?`.
type TryExpr struct {
	Pos Pos
	X   Expr
}

type IfExpr struct {
	Pos  Pos
	Cond Expr
	Then *Block
	Else Expr // nil, *BlockExpr, or *IfExpr
}

type MatchArm struct {
	Pos   Pos
	Pat   Pattern
	Guard Expr // may be nil
	Body  Expr
}

type MatchExpr struct {
	Pos     Pos
	Subject Expr
	Arms    []*MatchArm
}

type BlockExpr struct {
	Block *Block
}

type LambdaExpr struct {
	Pos    Pos
	Params []*Param
	Body   Expr
}

type RangeExpr struct {
	Pos       Pos
	Lo        Expr
	Hi        Expr
	Inclusive bool
}

// SelfLit is `Self { field: v, ... }` (only valid inside init).
type SelfLit struct {
	Pos    Pos
	Fields []*Arg // Name always set
}

// ReturnExpr / BreakExpr / ContinueExpr are expressions of type Never so they
// can appear in `let ... else return`, match arms, etc.
type ReturnExpr struct {
	Pos   Pos
	Value Expr // may be nil
}

type BreakExpr struct{ Pos Pos }
type ContinueExpr struct{ Pos Pos }

func (*Ident) exprNode()        {}
func (*IntLit) exprNode()       {}
func (*FloatLit) exprNode()     {}
func (*DecLit) exprNode()       {}
func (*BoolLit) exprNode()      {}
func (*StrLit) exprNode()       {}
func (*ListLit) exprNode()      {}
func (*MapLit) exprNode()       {}
func (*TupleLit) exprNode()     {}
func (*BinaryExpr) exprNode()   {}
func (*UnaryExpr) exprNode()    {}
func (*CallExpr) exprNode()     {}
func (*MemberExpr) exprNode()   {}
func (*IndexExpr) exprNode()    {}
func (*TryExpr) exprNode()      {}
func (*IfExpr) exprNode()       {}
func (*MatchExpr) exprNode()    {}
func (*BlockExpr) exprNode()    {}
func (*LambdaExpr) exprNode()   {}
func (*RangeExpr) exprNode()    {}
func (*SelfLit) exprNode()      {}
func (*ReturnExpr) exprNode()   {}
func (*BreakExpr) exprNode()    {}
func (*ContinueExpr) exprNode() {}

func (e *Ident) GetPos() Pos        { return e.Pos }
func (e *IntLit) GetPos() Pos       { return e.Pos }
func (e *FloatLit) GetPos() Pos     { return e.Pos }
func (e *DecLit) GetPos() Pos       { return e.Pos }
func (e *BoolLit) GetPos() Pos      { return e.Pos }
func (e *StrLit) GetPos() Pos       { return e.Pos }
func (e *ListLit) GetPos() Pos      { return e.Pos }
func (e *MapLit) GetPos() Pos       { return e.Pos }
func (e *TupleLit) GetPos() Pos     { return e.Pos }
func (e *BinaryExpr) GetPos() Pos   { return e.Pos }
func (e *UnaryExpr) GetPos() Pos    { return e.Pos }
func (e *CallExpr) GetPos() Pos     { return e.Pos }
func (e *MemberExpr) GetPos() Pos   { return e.Pos }
func (e *IndexExpr) GetPos() Pos    { return e.Pos }
func (e *TryExpr) GetPos() Pos      { return e.Pos }
func (e *IfExpr) GetPos() Pos       { return e.Pos }
func (e *MatchExpr) GetPos() Pos    { return e.Pos }
func (e *BlockExpr) GetPos() Pos    { return e.Block.Pos }
func (e *LambdaExpr) GetPos() Pos   { return e.Pos }
func (e *RangeExpr) GetPos() Pos    { return e.Pos }
func (e *SelfLit) GetPos() Pos      { return e.Pos }
func (e *ReturnExpr) GetPos() Pos   { return e.Pos }
func (e *BreakExpr) GetPos() Pos    { return e.Pos }
func (e *ContinueExpr) GetPos() Pos { return e.Pos }

// ---------- patterns ----------

type Pattern interface {
	patNode()
	GetPos() Pos
}

type WildcardPat struct{ Pos Pos }

// BindPat binds a name. (In match arms, a bare identifier that names a
// payload-less enum variant is resolved by the checker, not the parser:
// the parser emits VariantPat for capitalised identifiers and BindPat for
// lowercase ones.)
type BindPat struct {
	Pos  Pos
	Name string
}

type LitPat struct {
	Pos   Pos
	Value Expr // IntLit, StrLit, BoolLit, FloatLit, or UnaryExpr(-, IntLit)
}

// VariantPat is `Name`, `Name(p, ...)`, `Type.Name(...)`, `Ok(p)`, `Err(p)`,
// `Some(p)`, `None`.
type VariantPat struct {
	Pos       Pos
	Qualifier string // optional enum type name
	Name      string
	Args      []Pattern
	HasParens bool
}

type TuplePat struct {
	Pos   Pos
	Elems []Pattern
}

type OrPat struct {
	Pos  Pos
	Alts []Pattern
}

func (*WildcardPat) patNode() {}
func (*BindPat) patNode()     {}
func (*LitPat) patNode()      {}
func (*VariantPat) patNode()  {}
func (*TuplePat) patNode()    {}
func (*OrPat) patNode()       {}

func (p *WildcardPat) GetPos() Pos { return p.Pos }
func (p *BindPat) GetPos() Pos     { return p.Pos }
func (p *LitPat) GetPos() Pos      { return p.Pos }
func (p *VariantPat) GetPos() Pos  { return p.Pos }
func (p *TuplePat) GetPos() Pos    { return p.Pos }
func (p *OrPat) GetPos() Pos       { return p.Pos }
