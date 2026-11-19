// Package check implements the Pinion static checker (DESIGN.md chapter 3).
package check

import (
	"fmt"
	"strings"

	"github.com/pinionlang/pinion/internal/ast"
	"github.com/pinionlang/pinion/internal/diag"
	"github.com/pinionlang/pinion/internal/token"
)

type module struct {
	file    *ast.File
	path    string
	types   map[string]*TypeInfo
	fns     map[string]*FnInfo
	lets    map[string]*letInfo
	imports map[string]*importRef
}

type importRef struct {
	std *stdModule
	mod *module
	imp *ast.Import
}

type letInfo struct {
	decl     *ast.LetDecl
	t        *Type
	mod      *module
	checking bool
	done     bool
}

type checker struct {
	diags   *diag.List
	std     *stdEnv
	modules []*module
	byPath  map[string]*module
	trail   []*TVar
	nextVar int

	// current position in the program
	mod *module
	fn  *fnCtx
	sc  *scope

	deferred  []func()
	bodyPhase bool

	lastWrapCandidate ast.Expr
}

// Check type-checks all modules of one program.
func Check(files []*ast.File) *diag.List {
	c := &checker{diags: &diag.List{}, std: getStd(), byPath: map[string]*module{}}
	for _, f := range files {
		m := &module{file: f, path: f.Module, types: map[string]*TypeInfo{}, fns: map[string]*FnInfo{},
			lets: map[string]*letInfo{}, imports: map[string]*importRef{}}
		c.modules = append(c.modules, m)
		c.byPath[f.Module] = m
	}
	c.resolveImports()
	for _, m := range c.modules {
		c.collect(m)
	}
	for _, m := range c.modules {
		c.resolveDecls(m)
	}
	for _, f := range c.deferred {
		f()
	}
	c.deferred = nil
	for _, m := range c.modules {
		c.declChecks(m)
	}
	c.bodyPhase = true
	for _, m := range c.modules {
		c.checkBodies(m)
	}
	c.diags.Sort()
	return c.diags
}

func (c *checker) errorf(code string, pos token.Pos, format string, args ...any) {
	if c.fn != nil {
		c.fn.markError()
	}
	c.diags.Errorf(code, pos, format, args...)
}

func (c *checker) errorFix(code string, pos token.Pos, fix diag.Fix, format string, args ...any) {
	if c.fn != nil {
		c.fn.markError()
	}
	c.diags.Add(diag.Diagnostic{Code: code, Severity: diag.Error, Pos: pos, Msg: fmt.Sprintf(format, args...),
		Fixes: []diag.Fix{fix}})
}

func (c *checker) filePath() string {
	if c.mod == nil {
		return ""
	}
	return c.mod.file.Path
}

// ---------- imports ----------

func (c *checker) resolveImports() {
	for _, m := range c.modules {
		for _, imp := range m.file.Imports {
			name := imp.Name()
			if strings.HasPrefix(imp.Path, "std/") {
				sm := c.std.modules[strings.TrimPrefix(imp.Path, "std/")]
				if sm == nil {
					c.diags.Errorf("E1101", imp.Pos, "unknown standard module %q", imp.Path)
					continue
				}
				m.imports[name] = &importRef{std: sm, imp: imp}
				continue
			}
			target := c.byPath[imp.Path]
			if target == nil {
				c.diags.Errorf("E1101", imp.Pos, "unknown module %q", imp.Path)
				continue
			}
			m.imports[name] = &importRef{mod: target, imp: imp}
		}
	}
	// cycle detection: an import edge u -> v is in a cycle when v reaches u.
	reach := func(from, to *module) bool {
		seen := map[*module]bool{}
		var dfs func(x *module) bool
		dfs = func(x *module) bool {
			if x == to {
				return true
			}
			if seen[x] {
				return false
			}
			seen[x] = true
			for _, r := range x.imports {
				if r.mod != nil && dfs(r.mod) {
					return true
				}
			}
			return false
		}
		return dfs(from)
	}
	for _, m := range c.modules {
		for _, imp := range m.file.Imports {
			r := m.imports[imp.Name()]
			if r == nil || r.mod == nil || r.imp != imp {
				continue
			}
			if reach(r.mod, m) {
				c.diags.Errorf("E1601", imp.Pos, "import cycle: %s imports %s, which imports it back", m.path, imp.Path)
			}
		}
	}
}

// ---------- declaration collection ----------

func (c *checker) collect(m *module) {
	path := m.file.Path
	for _, d := range m.file.Decls {
		switch d := d.(type) {
		case *ast.TypeDecl:
			k := DData
			if d.Kind == ast.KindClass {
				k = DClass
			}
			m.types[d.Name] = &TypeInfo{Name: d.Name, File: path, Module: m, Kind: k, Pub: d.Pub, Pos: d.Pos,
				TypeDecl: d, Methods: map[string]*FnInfo{}}
		case *ast.EnumDecl:
			m.types[d.Name] = &TypeInfo{Name: d.Name, File: path, Module: m, Kind: DEnum, Pub: d.Pub, Pos: d.Pos,
				EnumDecl: d, Methods: map[string]*FnInfo{}}
		case *ast.InterfaceDecl:
			m.types[d.Name] = &TypeInfo{Name: d.Name, File: path, Module: m, Kind: DInterface, Pub: d.Pub, Pos: d.Pos,
				IfaceDecl: d, Methods: map[string]*FnInfo{}}
		case *ast.AliasDecl:
			m.types[d.Name] = &TypeInfo{Name: d.Name, File: path, Module: m, Kind: DAlias, Pub: d.Pub, Pos: d.Pos,
				AliasDecl: d, Methods: map[string]*FnInfo{}}
		case *ast.FnDecl:
			m.fns[d.Name] = &FnInfo{Name: d.Name, Pub: d.Pub, File: path, Decl: d, Pos: d.Pos}
		case *ast.LetDecl:
			if d.Var {
				c.diags.Errorf("E1404", d.Pos, "top-level var is not allowed; module state must be immutable")
			}
			m.lets[d.Name] = &letInfo{decl: d, mod: m}
		}
	}
	for _, ti := range m.types {
		var tps []*ast.TypeParam
		switch {
		case ti.TypeDecl != nil:
			tps = ti.TypeDecl.TypeParams
		case ti.EnumDecl != nil:
			tps = ti.EnumDecl.TypeParams
		case ti.IfaceDecl != nil:
			tps = ti.IfaceDecl.TypeParams
		case ti.AliasDecl != nil:
			tps = ti.AliasDecl.TypeParams
		}
		for _, tp := range tps {
			ti.Params = append(ti.Params, &TypeParamInfo{Name: tp.Name, Cons: tp.Constraints, Frozen: ti.Kind == DData})
		}
	}
}

// ---------- type expressions ----------

type typeEnv struct {
	params map[string]*TypeParamInfo
	self   *TypeInfo
}

func (env *typeEnv) with(tps []*TypeParamInfo) *typeEnv {
	n := &typeEnv{params: map[string]*TypeParamInfo{}}
	if env != nil {
		for k, v := range env.params {
			n.params[k] = v
		}
		n.self = env.self
	}
	for _, p := range tps {
		n.params[p.Name] = p
	}
	return n
}

func (c *checker) lookupTypeName(m *module, qual, name string, pos token.Pos) *TypeInfo {
	if qual != "" {
		r := m.imports[qual]
		if r == nil {
			c.errorf("E1101", pos, "unknown module %q", qual)
			return nil
		}
		if r.std != nil {
			if ti := r.std.types[name]; ti != nil {
				return ti
			}
			c.errorf("E1101", pos, "unknown type %s.%s", qual, name)
			return nil
		}
		ti := r.mod.types[name]
		if ti == nil {
			c.errorf("E1101", pos, "unknown type %s.%s", qual, name)
			return nil
		}
		if !ti.Pub {
			c.errorf("E1602", pos, "type %s.%s is private to its module", qual, name)
		}
		return ti
	}
	if ti := m.types[name]; ti != nil {
		return ti
	}
	if ti := c.std.types[name]; ti != nil {
		return ti
	}
	return nil
}

func (c *checker) resolveType(te ast.TypeExpr, env *typeEnv) *Type {
	if te == nil {
		return unitT
	}
	m := c.mod
	switch te := te.(type) {
	case *ast.OptionalType:
		inner := c.resolveType(te.Elem, env)
		if p := prune(inner); p.K == KOption {
			c.errorf("E1204", te.Pos, "nested optional %s? is not allowed", typeString(inner))
			return inner
		}
		return optOf(inner)
	case *ast.TupleType:
		if len(te.Elems) == 0 {
			return unitT
		}
		var ts []*Type
		for _, e := range te.Elems {
			ts = append(ts, c.resolveType(e, env))
		}
		return tupleOf(ts...)
	case *ast.FnType:
		var ps []*Type
		for _, p := range te.Params {
			ps = append(ps, c.resolveType(p, env))
		}
		ret := unitT
		if te.Ret != nil {
			ret = c.resolveType(te.Ret, env)
		}
		return &Type{K: KFn, Args: ps, Ret: ret, Eff: effectsOf(te.Effects)}
	case *ast.NamedType:
		var args []*Type
		for _, a := range te.Args {
			args = append(args, c.resolveType(a, env))
		}
		if te.Qualifier == "" {
			if te.Name == "Self" && env != nil && env.self != nil {
				return env.self.selfType()
			}
			if env != nil {
				if tp := env.params[te.Name]; tp != nil {
					return &Type{K: KParam, Param: tp}
				}
			}
			if n := builtinArity(te.Name); n >= 0 {
				if len(args) != n {
					c.errorf("E1201", te.Pos, "%s takes %d type arguments", te.Name, n)
					return errT
				}
				t := builtinType(te.Name, args)
				switch t.K {
				case KSet, KMutSet:
					c.deferKeyCheck(args[0], te.Pos, te.Name+" element")
				case KMap, KMutMap:
					c.deferKeyCheck(args[0], te.Pos, te.Name+" key")
				}
				return t
			}
		}
		ti := c.lookupTypeName(m, te.Qualifier, te.Name, te.Pos)
		if ti == nil {
			if te.Qualifier == "" {
				c.errorf("E1101", te.Pos, "unknown type %s", te.Name)
			}
			return errT
		}
		if len(args) != len(ti.Params) {
			c.errorf("E1201", te.Pos, "%s takes %d type arguments, got %d", ti.Name, len(ti.Params), len(args))
			return errT
		}
		if ti.Kind == DAlias {
			c.resolveAlias(ti)
			mm := map[*TypeParamInfo]*Type{}
			for i, p := range ti.Params {
				mm[p] = args[i]
			}
			return subst(ti.Alias, mm)
		}
		if ti.Kind == DData && len(args) > 0 {
			pos := te.Pos
			for _, a := range args {
				a := a
				c.later(func() {
					if prune(a).K != KParam && !c.has(a, PFrozen) {
						c.errorf("E1402", pos, "type argument %s of data %s is not deeply immutable", typeString(a), ti.Name)
					}
				})
			}
		}
		return named(ti, args...)
	}
	return errT
}

func (c *checker) deferKeyCheck(t *Type, pos token.Pos, what string) {
	c.later(func() {
		if !c.has(t, PEq) || !c.has(t, PHash) {
			c.errorf("E1207", pos, "%s type %s must have Eq and Hash", what, typeString(t))
		}
	})
}

// later runs f after all declarations are resolved (immediately during body checking).
func (c *checker) later(f func()) {
	if c.bodyPhase {
		f()
		return
	}
	c.deferred = append(c.deferred, f)
}

func (c *checker) resolveAlias(ti *TypeInfo) {
	if ti.Alias != nil {
		return
	}
	if ti.resolving {
		c.errorf("E1406", ti.Pos, "type alias %s refers to itself", ti.Name)
		ti.Alias = errT
		return
	}
	ti.resolving = true
	saved := c.mod
	c.mod = ti.Module
	ti.Alias = c.resolveType(ti.AliasDecl.Type, (&typeEnv{}).with(ti.Params))
	c.mod = saved
	ti.resolving = false
}
