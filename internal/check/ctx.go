package check

import "github.com/pinionlang/pinion/internal/token"

// fnCtx is the checking context of one function, method, init, test,
// field default, top-level let or closure body.
type fnCtx struct {
	parent   *fnCtx // enclosing context (for closures)
	ret      *Type  // expected result (for return / ?)
	declared Effects
	used     Effects
	allowAll bool   // test body: every std effect is replaced
	effCode  string // code for an undeclared effect: E3001 (default) or E3005
	lambda   bool   // closure: effects are collected, not checked
	depth    int    // closure nesting depth

	owner    *TypeInfo // enclosing type for methods/init
	selfT    *Type
	method   bool
	mutFn    bool
	isInit   bool
	hadError bool
}

func (f *fnCtx) markError() {
	for x := f; x != nil; x = x.parent {
		x.hadError = true
	}
}

// useEffects records effects incurred at pos.
func (c *checker) useEffects(e Effects, pos token.Pos) {
	f := c.fn
	if f == nil || e == 0 {
		return
	}
	f.used |= e
	if f.lambda || f.allowAll {
		return
	}
	if missing := e &^ f.declared; missing != 0 {
		code := f.effCode
		if code == "" {
			code = "E3001"
		}
		switch code {
		case "E3005":
			c.errorf(code, pos, "field default value cannot have effects (%s)", missing)
		default:
			c.errorFix(code, pos, effectFix(missing), "effect %s is used here but not declared by the enclosing function", missing)
		}
	}
}

type binding struct {
	name    string
	t       *Type
	mutable bool // var
	depth   int  // closure depth where declared
	self    bool
}

type scope struct {
	parent *scope
	vars   map[string]*binding
}

func (c *checker) push() { c.sc = &scope{parent: c.sc, vars: map[string]*binding{}} }
func (c *checker) pop()  { c.sc = c.sc.parent }

func (c *checker) declare(name string, t *Type, mutable bool) *binding {
	if name == "_" || name == "" {
		return nil
	}
	depth := 0
	if c.fn != nil {
		depth = c.fn.depth
	}
	b := &binding{name: name, t: t, mutable: mutable, depth: depth}
	c.sc.vars[name] = b
	return b
}

func (c *checker) lookup(name string) *binding {
	for s := c.sc; s != nil; s = s.parent {
		if b := s.vars[name]; b != nil {
			return b
		}
	}
	return nil
}
