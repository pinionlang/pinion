// Package outline implements `pinion outline`: a signature-level view of
// modules with doc comments, effects and mut markers, without bodies.
package outline

import (
	"fmt"
	"strings"

	"github.com/pinionlang/pinion/internal/ast"
)

// Outline renders the outline of the given files, one module after another.
func Outline(files []*ast.File) string {
	var b strings.Builder
	for i, f := range files {
		if i > 0 {
			b.WriteByte('\n')
		}
		name := f.Module
		if name == "" {
			name = f.Path
		}
		fmt.Fprintf(&b, "module %s\n", name)
		for _, imp := range f.Imports {
			if imp.Alias != "" {
				fmt.Fprintf(&b, "  import %s as %s\n", imp.Path, imp.Alias)
			} else {
				fmt.Fprintf(&b, "  import %s\n", imp.Path)
			}
		}
		for _, d := range f.Decls {
			writeDecl(&b, d)
		}
	}
	return b.String()
}

func doc(b *strings.Builder, indent, text string) {
	if text == "" {
		return
	}
	for _, l := range strings.Split(text, "\n") {
		fmt.Fprintf(b, "%s/// %s\n", indent, l)
	}
}

func pub(p bool) string {
	if p {
		return "pub "
	}
	return ""
}

func writeDecl(b *strings.Builder, d ast.Decl) {
	const in = "  "
	switch d := d.(type) {
	case *ast.FnDecl:
		doc(b, in, d.Doc)
		fmt.Fprintf(b, "%s%s\n", in, FnSig(d))
		if d.Name == "main" && d.Body != nil {
			bound := "(none)"
			if len(d.Effects) > 0 {
				bound = effects(d.Effects)[1:]
			}
			fmt.Fprintf(b, "%s  effect upper bound: %s\n", in, bound)
			if d.Script {
				fmt.Fprintf(b, "%s  script: implicit main from top-level statements (effects inferred)\n", in)
			}
		}
	case *ast.TypeDecl:
		doc(b, in, d.Doc)
		kw := "data"
		if d.Kind == ast.KindClass {
			kw = "class"
		}
		head := fmt.Sprintf("%s%s %s%s", pub(d.Pub), kw, d.Name, typeParams(d.TypeParams))
		if len(d.Implements) > 0 {
			var is []string
			for _, t := range d.Implements {
				is = append(is, Type(t))
			}
			head += " : " + strings.Join(is, ", ")
		}
		fmt.Fprintf(b, "%s%s\n", in, head)
		for _, f := range d.Fields {
			doc(b, in+"  ", f.Doc)
			mut := ""
			if f.Mut {
				mut = "mut "
			}
			def := ""
			if f.Default != nil {
				def = " = …"
			}
			fmt.Fprintf(b, "%s  %s%s%s: %s%s\n", in, pub(f.Pub), mut, f.Name, Type(f.Type), def)
		}
		if d.Init != nil {
			doc(b, in+"  ", d.Init.Doc)
			fmt.Fprintf(b, "%s  %s\n", in, FnSig(d.Init))
		}
		for _, m := range d.Methods {
			doc(b, in+"  ", m.Doc)
			fmt.Fprintf(b, "%s  %s\n", in, FnSig(m))
		}
	case *ast.InterfaceDecl:
		doc(b, in, d.Doc)
		fmt.Fprintf(b, "%s%sinterface %s%s\n", in, pub(d.Pub), d.Name, typeParams(d.TypeParams))
		for _, m := range d.Methods {
			doc(b, in+"  ", m.Doc)
			fmt.Fprintf(b, "%s  %s\n", in, FnSig(m))
		}
	case *ast.EnumDecl:
		doc(b, in, d.Doc)
		fmt.Fprintf(b, "%s%senum %s%s\n", in, pub(d.Pub), d.Name, typeParams(d.TypeParams))
		for _, v := range d.Variants {
			doc(b, in+"  ", v.Doc)
			s := v.Name
			if len(v.Fields) > 0 {
				var fs []string
				for _, f := range v.Fields {
					if f.Name != "" {
						fs = append(fs, f.Name+": "+Type(f.Type))
					} else {
						fs = append(fs, Type(f.Type))
					}
				}
				s += "(" + strings.Join(fs, ", ") + ")"
			}
			fmt.Fprintf(b, "%s  %s\n", in, s)
		}
	case *ast.AliasDecl:
		doc(b, in, d.Doc)
		fmt.Fprintf(b, "%s%stype %s%s = %s\n", in, pub(d.Pub), d.Name, typeParams(d.TypeParams), Type(d.Type))
	case *ast.LetDecl:
		doc(b, in, d.Doc)
		kw := "let"
		if d.Var {
			kw = "var"
		}
		ty := ""
		if d.Type != nil {
			ty = ": " + Type(d.Type)
		}
		fmt.Fprintf(b, "%s%s%s %s%s\n", in, pub(d.Pub), kw, d.Name, ty)
	case *ast.TestDecl:
		fmt.Fprintf(b, "%stest %q%s\n", in, d.Name, effects(d.Effects))
	}
}

// FnSig renders a function/method/init signature without its body.
func FnSig(d *ast.FnDecl) string {
	var s strings.Builder
	s.WriteString(pub(d.Pub))
	if d.IsInit {
		s.WriteString("init")
	} else {
		if d.Mut {
			s.WriteString("mut ")
		}
		s.WriteString("fn " + d.Name + typeParams(d.TypeParams))
	}
	var ps []string
	for _, p := range d.Params {
		ps = append(ps, p.Name+": "+Type(p.Type))
	}
	s.WriteString("(" + strings.Join(ps, ", ") + ")")
	if d.Ret != nil {
		s.WriteString(" -> " + Type(d.Ret))
	}
	s.WriteString(effects(d.Effects))
	return s.String()
}

func effects(es []ast.Effect) string {
	var s strings.Builder
	for _, e := range es {
		s.WriteString(" !" + e.Name)
	}
	return s.String()
}

func typeParams(tps []*ast.TypeParam) string {
	if len(tps) == 0 {
		return ""
	}
	var ss []string
	for _, tp := range tps {
		s := tp.Name
		if len(tp.Constraints) > 0 {
			s += ": " + strings.Join(tp.Constraints, " + ")
		}
		ss = append(ss, s)
	}
	return "<" + strings.Join(ss, ", ") + ">"
}

// Type renders a type expression in source syntax.
func Type(t ast.TypeExpr) string {
	switch t := t.(type) {
	case nil:
		return "()"
	case *ast.NamedType:
		s := t.Name
		if t.Qualifier != "" {
			s = t.Qualifier + "." + s
		}
		if len(t.Args) > 0 {
			var as []string
			for _, a := range t.Args {
				as = append(as, Type(a))
			}
			s += "<" + strings.Join(as, ", ") + ">"
		}
		return s
	case *ast.OptionalType:
		return Type(t.Elem) + "?"
	case *ast.TupleType:
		var es []string
		for _, e := range t.Elems {
			es = append(es, Type(e))
		}
		if len(es) == 1 {
			return "(" + es[0] + ",)"
		}
		return "(" + strings.Join(es, ", ") + ")"
	case *ast.FnType:
		var ps []string
		for _, p := range t.Params {
			ps = append(ps, Type(p))
		}
		s := "fn(" + strings.Join(ps, ", ") + ")"
		if t.Ret != nil {
			s += " -> " + Type(t.Ret)
		}
		return s + effects(t.Effects)
	}
	return "?"
}
