// Copyright 2017 The Neugram Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package gengo implements a backend for the Neugram parser and
// typechecker that generates a Go package.
package gengo

import (
	"bytes"
	"fmt"
	"go/constant"
	goformat "go/format"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"neugram.io/ng/format"
	"neugram.io/ng/syntax"
	"neugram.io/ng/syntax/expr"
	"neugram.io/ng/syntax/stmt"
	"neugram.io/ng/syntax/tipe"
	"neugram.io/ng/syntax/token"
	"neugram.io/ng/typecheck"
)

func GenGo(filename, outGoPkgName string) (result []byte, err error) {
	p := &printer{
		buf:     new(bytes.Buffer),
		c:       typecheck.New(filename), // TODO: extract a pkg name
		imports: make(map[*tipe.Package]string),
		eliders: make(map[tipe.Type]string),
	}

	abspath, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}

	pkg, err := p.c.Check(abspath)
	if err != nil {
		return nil, err
	}

	if outGoPkgName == "" {
		outGoPkgName = "gengo_" + strings.TrimSuffix(filepath.Base(filename), ".ng")
	}
	p.printf(`// generated by ng, do not edit

package %s

`, outGoPkgName)

	usesShell := false
	builtins := make(map[string]bool)
	importPaths := []string{}
	preFn := func(c *syntax.Cursor) bool {
		switch node := c.Node.(type) {
		case *stmt.Import:
			importPaths = append(importPaths, node.Path)
		case *expr.Ident:
			// TODO: look up the typecheck.Obj for builtins
			switch node.Name {
			case "printf":
				builtins["printf"] = true
			case "print":
				builtins["print"] = true
			case "errorf":
				builtins["errorf"] = true
			}
		case *expr.ShellList:
			usesShell = true
		}
		return true
	}
	syntax.Walk(pkg.Syntax, preFn, nil)

	// Lift imports to the top-level.
	importSet := make(map[string]bool)
	for _, imp := range importPaths {
		importSet[imp] = true
	}
	// Name.
	namedImports := make(map[string]string) // name -> path
	for imp := range importSet {
		name := "gengoimp_" + path.Base(imp)
		i := 0
		for namedImports[name] != "" {
			i++
			name = fmt.Sprintf("gengoimp_%s_%d", path.Base(imp), i)
		}
		namedImports[name] = imp
		p.imports[p.c.Pkg(imp).Type] = name
	}

	p.printf("import (")
	p.indent++

	if builtins["printf"] || builtins["print"] || builtins["errorf"] {
		p.newline()
		p.printf(`"fmt"`)
	}
	if usesShell {
		p.newline()
		p.printf(`"fmt"`)
		p.newline()
		p.printf(`"os"`)
		p.newline()
		p.printf(`"reflect"`)
		p.newline()
		p.printf(`"neugram.io/ng/eval/environ"`)
		p.newline()
		p.printf(`"neugram.io/ng/eval/shell"`)
		p.newline()
		p.printf(`"neugram.io/ng/syntax/expr"`)
		p.newline()
		p.printf(`"neugram.io/ng/syntax/src"`)
		p.newline()
		p.printf(`"neugram.io/ng/syntax/token"`)
	}

	// Stable output is ensured by gofmt's sorting later.
	for name, imp := range namedImports {
		p.newline()
		p.printf("%s %q", name, imp)
	}

	p.indent--
	p.newline()
	p.print(")")
	p.newline()
	p.newline()

	if outGoPkgName == "main" {
		p.printf("func main() {}")
		p.newline()
		p.newline()
	}

	// Lift export declarations to the top-level.
	for _, obj := range pkg.Exported {
		switch obj.Kind {
		case typecheck.ObjType:
			p.printf("type %s %s", obj.Name, format.Type(obj.Type.(*tipe.Named).Type))
			p.newline()
			p.newline()
		case typecheck.ObjVar:
			p.printf("var %s %s", obj.Name, format.Type(obj.Type))
			p.newline()
			p.newline()
		case typecheck.ObjConst:
			p.printf("const %s %s = %s", obj.Name, format.Type(obj.Type), obj.Decl.(constant.Value))
			p.newline()
			p.newline()
		}
	}

	// TODO: Lift methodik declarations to the top-level.
	//       (Both of these are blocked on a visitor API.)

	p.print("func init() {")
	p.indent++
	for _, s := range pkg.Syntax.Stmts {
		p.newline()
		p.stmt(s)

		if s, isAssign := s.(*stmt.Assign); isAssign {
			// TODO: look to see if typecheck object is used,
			//       only emit this if it isn't.
			if s.Decl {
				for _, e := range s.Left {
					if ident, isIdent := e.(*expr.Ident); isIdent && ident.Name == "_" {
						continue
					}
					p.newline()
					p.print("_ = ")
					p.expr(e)
				}
			}
		}
	}
	p.indent--
	p.newline()
	p.print("}")

	p.printBuiltins(builtins)
	p.printEliders()
	if usesShell {
		p.printShell()
	}

	res, err := goformat.Source(p.buf.Bytes())
	if err != nil {
		lines := new(bytes.Buffer)
		for i, line := range strings.Split(p.buf.String(), "\n") {
			fmt.Fprintf(lines, "%3d: %s\n", i+1, line)
		}
		return nil, fmt.Errorf("gengo: bad generated source: %v\n%s", err, lines.String())
	}

	return res, nil
}

type printer struct {
	buf    *bytes.Buffer
	indent int

	imports map[*tipe.Package]string // import package -> name
	c       *typecheck.Checker
	eliders map[tipe.Type]string
}

func (p *printer) printShell() {
	p.newline()
	p.newline()
	p.printf(`var _ = src.Pos{} // used in some expr.Shell prints`)
	p.newline()
	p.printf(`var _ = token.Token(0)`)
	p.newline()
	p.printf(`var shellState = &shell.State{
	Env:   environ.NewFrom(os.Environ()),
	Alias: environ.New(),
}`)

	p.newline()
	p.newline()
	p.printf(`func init() {
	wd, err := os.Getwd()
	if err == nil {
		shellState.Env.Set("PWD", wd)
	}
}`)

	p.newline()
	p.newline()
	p.printf(`func gengo_shell(e *expr.Shell, p gengo_shell_params) (string, error) {
	str, err := shell.Run(shellState, p, e)
	return str, err
}

func gengo_shell_elide(e *expr.Shell, p gengo_shell_params) string {
	str, err := gengo_shell(e, p)
	if err != nil {
		panic(err)
	}
	return str
}

type gengo_shell_params map[string]reflect.Value

func (p gengo_shell_params) Get(name string) string {
	if v, found := p[name]; found {
		vi := v.Interface()
		if s, ok := vi.(string); ok {
			return s
		}
		return fmt.Sprint(vi)
	}
	return shellState.Env.Get(name)
}

func (p gengo_shell_params) Set(name, value string) {
	v, found := p[name]
	if !found {
		v = reflect.ValueOf(&value).Elem()
		p[name] = v
	}
	if v.Kind() == reflect.String {
		v.SetString(value)
	} else {
		fmt.Sscan(value, v)
	}
}

func init() { shell.Init() }
`)
}

func (p *printer) printBuiltins(builtins map[string]bool) {
	if builtins["print"] {
		p.newline()
		p.newline()
		p.print(`func print(args ...interface{}) {
	for _, arg := range args {
		fmt.Printf("%v", arg)
	}
	fmt.Print("\n")
}`)
	}

	if builtins["printf"] {
		p.newline()
		p.newline()
		p.print("func printf(f string, args ...interface{}) { fmt.Printf(f, args...) }")
	}

	if builtins["errorf"] {
		p.newline()
		p.newline()
		p.print("func errorf(f string, args ...interface{}) error { return fmt.Errorf(f, args...) }")
	}
}

func (p *printer) printEliders() {
	for t, name := range p.eliders {
		p.newline()
		p.newline()
		if typecheck.IsError(t) {
			p.printf("func %s(err error) {", name)
			p.indent++
			p.newline()
			p.printf("if err != nil { panic(err) }")
			p.indent++
			p.newline()
			p.printf("}")
			continue
		}

		p.printf("func %s(", name)
		elems := t.(*tipe.Tuple).Elems
		for i, elem := range elems {
			if i == len(elems)-1 {
				p.printf("err error")
				continue
			}
			p.printf("arg%d ", i)
			p.tipe(elem)
			p.printf(", ")
		}
		p.printf(") (")
		for i, elem := range elems[:len(elems)-1] {
			if i > 0 {
				p.printf(", ")
			}
			p.tipe(elem)
		}
		p.printf(") {")
		p.indent++
		p.newline()
		p.printf("if err != nil { panic(err) }")
		p.newline()
		p.printf("return ")
		for i := range elems[:len(elems)-1] {
			if i > 0 {
				p.printf(", ")
			}
			p.printf("arg%d", i)
		}
		p.indent++
		p.newline()
		p.printf("}")
	}
}

func (p *printer) printf(format string, args ...interface{}) {
	fmt.Fprintf(p.buf, format, args...)
}

func (p *printer) print(str string) {
	p.buf.WriteString(str)
}

func (p *printer) newline() {
	p.buf.WriteByte('\n')
	for i := 0; i < p.indent; i++ {
		p.buf.WriteByte('\t')
	}
}

func (p *printer) expr(e expr.Expr) {
	switch e := e.(type) {
	case *expr.BasicLiteral:
		if str, isStr := e.Value.(string); isStr {
			p.printf("%q", str)
		} else {
			p.printf("%v", e.Value)
		}
	case *expr.Binary:
		p.expr(e.Left)
		p.printf(" %s ", e.Op)
		p.expr(e.Right)
	case *expr.Call:
		if e.ElideError {
			fnName := p.elider(p.c.Type(e))
			p.printf("%s(", fnName)
		}
		p.expr(e.Func)
		p.print("(")
		for i, arg := range e.Args {
			if i != 0 {
				p.print(", ")
			}
			p.expr(arg)
		}
		if e.Ellipsis {
			p.print("...")
		}
		p.print(")")
		if e.ElideError {
			p.print(")")
		}
	case *expr.CompLiteral:
		p.tipe(e.Type)
		p.print("{")
		if len(e.Keys) > 0 {
			p.indent++
			for i, key := range e.Keys {
				p.newline()
				p.expr(key)
				p.print(": ")
				p.expr(e.Elements[i])
				p.print(",")
			}
			p.indent--
			p.newline()
		} else if len(e.Elements) > 0 {
			for i, elem := range e.Elements {
				if i > 0 {
					p.print(", ")
				}
				p.expr(elem)
			}
		}
		p.print("}")
	case *expr.FuncLiteral:
		if e.Name != "" {
			p.printf("%s := ", e.Name)
		}
		p.print("func(")
		for i, name := range e.ParamNames {
			if i != 0 {
				p.print(", ")
			}
			p.print(name)
			p.print(" ")
			p.tipe(e.Type.Params.Elems[i])
		}
		p.print(") ")
		if len(e.ResultNames) != 0 {
			p.print("(")
			for i, name := range e.ResultNames {
				if i != 0 {
					p.print(", ")
				}
				p.print(name)
				p.print(" ")
				p.tipe(e.Type.Results.Elems[i])
			}
			p.print(")")
		}
		if e.Body != nil {
			p.print(" ")
			p.stmt(e.Body.(*stmt.Block))
		}
	case *expr.Ident:
		if pkgType, isPkg := p.c.Type(e).(*tipe.Package); isPkg {
			p.print(p.imports[pkgType])
		} else {
			p.print(e.Name)
		}
	case *expr.Index:
		p.expr(e.Left)
		p.print("[")
		for i, index := range e.Indicies {
			if i > 0 {
				p.print(", ")
			}
			p.expr(index)
		}
		p.print("]")
	case *expr.MapLiteral:
		p.tipe(e.Type)
		p.print("{")
		p.indent++
		for i, key := range e.Keys {
			p.newline()
			p.expr(key)
			p.print(": ")
			p.expr(e.Values[i])
			p.print(",")
		}
		p.indent--
		p.newline()
		p.print("}")
	case *expr.Selector:
		p.expr(e.Left)
		p.print(".")
		p.expr(e.Right)
	case *expr.Shell:
		if e.ElideError {
			p.printf("gengo_shell_elide(%s, gengo_shell_params{", format.Debug(e))
		} else {
			p.printf("gengo_shell(%s, gengo_shell_params{", format.Debug(e))
		}
		if len(e.FreeVars) > 0 {
			p.indent++
			for _, name := range e.FreeVars {
				p.newline()
				p.printf("%q: reflect.ValueOf(&%s).Elem(),", name, name)
			}
			p.indent--
			p.newline()
		}
		p.printf("})")
	case *expr.SliceLiteral:
		p.tipe(e.Type)
		p.print("{")
		for i, elem := range e.Elems {
			if i > 0 {
				p.print(", ")
			}
			p.expr(elem)
		}
		p.print("}")
	case *expr.Type:
		p.tipe(e.Type)
	case *expr.TypeAssert:
		p.expr(e.Left)
		p.print(".(")
		if e.Type == nil {
			p.print("type")
		} else {
			p.tipe(e.Type)
		}
		p.print(")")
	case *expr.Unary:
		p.print(e.Op.String())
		p.expr(e.Expr)
		if e.Op == token.LeftParen {
			p.print(")")
		}
	}
}

func (p *printer) stmt(s stmt.Stmt) {
	switch s := s.(type) {
	case *stmt.ConstSet:
		p.print("const (")
		p.indent++
		for _, v := range s.Consts {
			p.newline()
			p.stmtConst(v)
		}
		p.indent--
		p.newline()
		p.print(")")
	case *stmt.Const:
		p.print("const ")
		p.stmtConst(s)
	case *stmt.VarSet:
		p.print("var (")
		p.indent++
		for _, v := range s.Vars {
			p.newline()
			p.stmtVar(v)
		}
		p.indent--
		p.newline()
		p.print(")")
	case *stmt.Var:
		p.print("var ")
		p.stmtVar(s)
	case *stmt.Assign:
		for i, e := range s.Left {
			if i != 0 {
				p.print(", ")
			}
			p.expr(e)
		}
		// TODO: A, b := ...
		if ident, isIdent := s.Left[0].(*expr.Ident); !isIdent || isExported(ident.Name) || !s.Decl {
			p.print(" = ")
		} else {
			p.print(" := ")
		}
		for i, e := range s.Right {
			if i != 0 {
				p.print(", ")
			}
			p.expr(e)
		}
	case *stmt.Block:
		p.print("{")
		p.indent++
		for _, s := range s.Stmts {
			p.newline()
			p.stmt(s)
		}
		p.indent--
		p.newline()
		p.print("}")
	case *stmt.For:
		p.print("for ")
		if s.Init != nil {
			p.stmt(s.Init)
			p.print("; ")
		}
		if s.Cond != nil {
			p.expr(s.Cond)
			p.print("; ")
		}
		if s.Post != nil {
			p.stmt(s.Post)
		}
		p.stmt(s.Body)
	case *stmt.Go:
		p.print("go ")
		p.expr(s.Call)
	case *stmt.If:
		p.print("if ")
		if s.Init != nil {
			p.stmt(s.Init)
			p.print("; ")
		}
		p.expr(s.Cond)
		p.print(" ")
		p.stmt(s.Body)
		if s.Else != nil {
			p.print(" else ")
			p.stmt(s.Else)
		}
	case *stmt.ImportSet:
		// lifted to top-level earlier
	case *stmt.Import:
		// lifted to top-level earlier
	case *stmt.Range:
		p.print("for ")
		if s.Key != nil {
			p.expr(s.Key)
		}
		if s.Val != nil {
			p.print(", ")
			p.expr(s.Val)
		}
		if s.Decl {
			p.print(":")
		}
		if s.Key != nil || s.Val != nil {
			p.print("= ")
		}
		p.print("range ")
		p.expr(s.Expr)
		p.stmt(s.Body)
	case *stmt.Return:
		p.print("return")
		for i, e := range s.Exprs {
			if i == 0 {
				p.print(" ")
			} else {
				p.print(", ")
			}
			p.expr(e)
		}
	case *stmt.Simple:
		p.expr(s.Expr)
	case *stmt.Send:
		p.expr(s.Chan)
		p.print(" <- ")
		p.expr(s.Value)
	case *stmt.TypeDecl:
		p.printf("type %s ", s.Name)
		p.tipe(s.Type.Type)
	case *stmt.MethodikDecl:
		// lifted to top-level earlier
	case *stmt.Labeled:
		p.indent--
		p.newline()
		p.printf("%s:", s.Label)
		p.indent++
		p.newline()
		p.stmt(s.Stmt)
	case *stmt.Branch:
		p.printf("%s", s.Type)
		if s.Label != "" {
			p.printf(" %s", s.Label)
		}
	case *stmt.Switch:
		p.print("switch ")
		if s.Init != nil {
			p.stmt(s.Init)
			p.print("; ")
		}
		if s.Cond != nil {
			p.expr(s.Cond)
		}
		p.print(" {")

		for _, c := range s.Cases {
			p.newline()
			if c.Default {
				p.print("default:")
			} else {
				p.print("case ")
				for i, e := range c.Conds {
					if i > 0 {
						p.print(", ")
					}
					p.expr(e)
				}
				p.print(":")
			}
			p.indent++
			for _, s := range c.Body.Stmts {
				p.newline()
				p.stmt(s)
			}
			p.indent--
		}

		p.newline()
		p.print("}")

	case *stmt.TypeSwitch:
		p.print("switch ")
		if s.Init != nil {
			p.stmt(s.Init)
			p.print("; ")
		}
		p.stmt(s.Assign)
		p.print(" {")

		for _, c := range s.Cases {
			p.newline()
			if c.Default {
				p.print("default:")
			} else {
				p.print("case ")
				for i, t := range c.Types {
					if i > 0 {
						p.print(", ")
					}
					p.tipe(t)
				}
				p.print(":")
			}
			p.indent++
			for _, s := range c.Body.Stmts {
				p.newline()
				p.stmt(s)
			}
			p.indent--
		}

		p.newline()
		p.print("}")
	case *stmt.Select:
		p.print("select {")
		for _, c := range s.Cases {
			p.newline()
			if c.Default {
				p.print("default:")
			} else {
				p.print("case ")
				p.stmt(c.Stmt)
				p.print(":")
			}
			p.indent++
			for _, s := range c.Body.Stmts {
				p.newline()
				p.stmt(s)
			}
			p.indent--
		}
		p.newline()
		p.print("}")
	}
}

func (p *printer) stmtConst(s *stmt.Const) {
	for i, n := range s.NameList {
		if i != 0 {
			p.print(", ")
		}
		p.print(n)
	}
	if s.Type != nil {
		p.print(" ")
		p.tipe(s.Type)
	}
	if len(s.Values) == 0 {
		return
	}
	p.print(" = ")
	for i, e := range s.Values {
		if i != 0 {
			p.print(", ")
		}
		p.expr(e)
	}
}

func (p *printer) stmtVar(s *stmt.Var) {
	for i, n := range s.NameList {
		if i != 0 {
			p.print(", ")
		}
		p.print(n)
	}
	if s.Type != nil {
		p.print(" ")
		p.tipe(s.Type)
	}
	if len(s.Values) == 0 {
		return
	}
	p.print(" = ")
	for i, e := range s.Values {
		if i != 0 {
			p.print(", ")
		}
		p.expr(e)
	}
}

// TODO there is a huge amount of overlap here with the format package.
//      deduplicate somehow.
func (p *printer) tipe(t tipe.Type) {
	switch t := t.(type) {
	case tipe.Basic:
		p.print(string(t))
	case *tipe.Struct:
		if len(t.Fields) == 0 {
			p.print("struct{}")
			return
		}
		p.print("struct {")
		p.indent++
		maxlen := 0
		for _, sf := range t.Fields {
			if len(sf.Name) > maxlen {
				maxlen = len(sf.Name)
			}
		}
		for _, sf := range t.Fields {
			p.newline()
			name := sf.Name
			if name == "" {
				name = "*ERROR*No*Name*"
			}
			p.print(name)
			for i := len(name); i <= maxlen; i++ {
				p.print(" ")
			}
			p.tipe(sf.Type)
		}
		p.indent--
		p.newline()
		p.print("}")
	case *tipe.Named:
		if t.PkgPath != "" {
			pkg := p.c.Pkg(t.PkgPath)
			p.print(p.imports[pkg.Type])
			p.print(".")
		}
		p.print(t.Name)
	case *tipe.Pointer:
		p.print("*")
		p.tipe(t.Elem)
	case *tipe.Unresolved:
		if t.Package != "" {
			p.print(t.Package)
			p.print(".")
		}
		p.print(t.Name)
	case *tipe.Array:
		if t.Ellipsis {
			p.print("[...]")
		} else {
			p.printf("[%d]", t.Len)
		}
		p.tipe(t.Elem)
	case *tipe.Slice:
		p.print("[]")
		p.tipe(t.Elem)
	case *tipe.Interface:
		if len(t.Methods) == 0 {
			p.print("interface{}")
			return
		}
		p.print("interface {")
		p.indent++
		names := make([]string, 0, len(t.Methods))
		for name := range t.Methods {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			p.newline()
			p.print(name)
			p.tipeFuncSig(t.Methods[name])
		}
		p.indent--
		p.newline()
		p.print("}")
	case *tipe.Map:
		p.print("map[")
		p.tipe(t.Key)
		p.print("]")
		p.tipe(t.Value)
	case *tipe.Chan:
		if t.Direction == tipe.ChanRecv {
			p.print("<-")
		}
		p.print("chan")
		if t.Direction == tipe.ChanSend {
			p.print("<-")
		}
		p.print(" ")
		p.tipe(t.Elem)
	case *tipe.Func:
		p.print("func")
		p.tipeFuncSig(t)
	case *tipe.Alias:
		p.print(t.Name)
	case *tipe.Tuple:
		p.print("(")
		for i, elt := range t.Elems {
			if i > 0 {
				p.print(", ")
			}
			p.tipe(elt)
		}
		p.print(")")
	case *tipe.Ellipsis:
		p.print("...")
		p.tipe(t.Elem)
	default:
		panic(fmt.Sprintf("unknown type: %T", t))
	}
}

func (p *printer) tipeFuncSig(t *tipe.Func) {
	p.print("(")
	if t.Params != nil {
		for i, elem := range t.Params.Elems {
			if i > 0 {
				p.print(", ")
			}
			p.tipe(elem)
		}
	}
	p.print(")")
	if t.Results != nil && len(t.Results.Elems) > 0 {
		p.print(" ")
		if len(t.Results.Elems) > 1 {
			p.print("(")
		}
		for i, elem := range t.Results.Elems {
			if i > 0 {
				p.print(", ")
			}
			p.tipe(elem)
		}
		if len(t.Results.Elems) > 1 {
			p.print(")")
		}
	}
}

func (p *printer) elider(t tipe.Type) string {
	name := p.eliders[t]
	if name == "" {
		name = fmt.Sprintf("gengo_elider%d", len(p.eliders))
		p.eliders[t] = name
	}
	return name
}

func isExported(name string) bool {
	ch, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(ch)
}
