// Package systemspec compila un *ast.Program (el mismo AST que usa
// 'asterion language check') a la descripción de un SISTEMA de varios
// plugins interconectados — distinto de pluginmanifest (que describe UN
// plugin, sus propios campos) y de providerspec (infraestructura de
// nube). Sintaxis reconocida, siempre bajo el namespace System.* (ver
// semantic/analyzer.go — System ya está reservado como builtin root, al
// lado de Provider/Lab/Plugin):
//
//	db  = System.plugin(route="./mi-plugin-db", principal=True)
//	api = System.plugin(route="github.com/user/api-plugin", ref="v1.2.3")
//	System.wire(to=api, key="DATABASE_URL", from=db, field="port")
//
// Mismo criterio que pluginmanifest/providerspec: paquete propio, su
// propio walker chico sobre prog.Statements, no se apoya en
// semantic.Analyzer (que solo devuelve un *diagnostics.Bag, nunca algo
// reusable — ver su propio doc comment).
//
// Es el primer compilador de este repo que resuelve de verdad una
// referencia a otra variable (System.wire(to=api, from=db, ...) —
// api/db son identificadores, no strings): pluginmanifest no tiene
// noción de referencias (todo son literales), y providerspec las
// rechaza explícitamente hoy (ver su exprKind — "todavía no soportado
// acá"). Acá sí hace falta, porque wire() no tiene sentido apuntando a
// otra cosa que un plugin ya declarado más arriba en el mismo archivo —
// ver call.ref más abajo.
package systemspec

import (
	"github.com/Tarafagat/asterion-language/ast"
	"github.com/Tarafagat/asterion-language/diagnostics"
)

// PluginDecl es un plugin declarado con System.plugin(...). Route es
// polimórfico (carpeta local o URL de git) — se resuelve por heurística
// en asterion-core (¿existe como carpeta en disco?), no acá: este
// paquete solo traduce sintaxis a datos, la interpretación vive afuera
// (mismo criterio que el resto de asterion-language).
type PluginDecl struct {
	Name      string // nombre de variable (db, api, principal, ...)
	Route     string
	Ref       string   // branch/tag/commit — vacío = HEAD del branch default
	Requires  []string // ej. ["node@20.11.0"] — ver internal/plugins/toolchains.go
	Principal bool
}

// WireDecl es una directiva System.wire(...) — antes de arrancar
// ToPlugin, su config Key debe setearse a partir de un campo de
// FromPlugin resuelto en runtime (Field: "port", o "env:<CLAVE>" para
// leer una config ya guardada de FromPlugin). Nunca lleva un valor
// literal: Key/Field son siempre nombres, jamás datos — un secreto de
// verdad no tiene ninguna forma de entrar por acá.
type WireDecl struct {
	ToPlugin   string
	Key        string
	FromPlugin string
	Field      string
}

// Compile recorre prog.Statements y arma la descripción del sistema. El
// Bag devuelto nunca es nil — puede estar vacío o traer uno o más
// diagnósticos; no corta en el primer error, mismo criterio que el
// resto del compilador.
func Compile(prog *ast.Program) ([]PluginDecl, []WireDecl, *diagnostics.Bag) {
	c := &compiler{diags: &diagnostics.Bag{}, declared: map[string]bool{}}
	c.walkStmts(prog.Statements)
	return c.plugins, c.wires, c.diags
}

type compiler struct {
	plugins  []PluginDecl
	wires    []WireDecl
	declared map[string]bool
	diags    *diagnostics.Bag
}

func (c *compiler) walkStmts(stmts []ast.Stmt) {
	for _, s := range stmts {
		c.walkStmt(s)
	}
}

// walkStmt solo reacciona a la forma `nombre = System.plugin(...)` y a
// `System.wire(...)` como statement suelto — cualquier otra cosa se
// ignora sin error (un archivo de sistema puede tener statements que no
// le importan a este compilador todavía), mismo criterio que
// providerspec.walkStmt.
func (c *compiler) walkStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.FuncDecl:
		c.walkStmts(s.Body)

	case *ast.AssignStmt:
		callExpr, ok := s.Value.(*ast.CallExpr)
		if !ok || !isSystemCall(callExpr.Callee, "plugin") {
			return
		}
		c.compilePluginDecl(s.Name, callExpr)

	case *ast.ExprStmt:
		callExpr, ok := s.X.(*ast.CallExpr)
		if !ok || !isSystemCall(callExpr.Callee, "wire") {
			return
		}
		c.compileWireDecl(callExpr)
	}
}

// isSystemCall reconoce la forma System.<method>(...) en el callee de un
// CallExpr.
func isSystemCall(callee ast.Expr, method string) bool {
	attr, ok := callee.(*ast.AttrExpr)
	if !ok || attr.Name != method {
		return false
	}
	root, ok := attr.X.(*ast.Ident)
	return ok && root.Name == "System"
}

func (c *compiler) compilePluginDecl(name string, callExpr *ast.CallExpr) {
	if c.declared[name] {
		c.diags.Errorf(callExpr.Pos, "ASTR500",
			"%q ya fue declarado antes en este archivo — los nombres de plugin del sistema deben ser únicos", name)
		return
	}
	cc := &call{verb: "plugin", args: callExpr.Args, pos: callExpr.Pos, diags: c.diags}
	route, _ := cc.str("route", true)
	ref, _ := cc.str("ref", false)
	requires := cc.stringList("requires")
	principal := cc.boolVal("principal", false)

	c.plugins = append(c.plugins, PluginDecl{
		Name: name, Route: route, Ref: ref, Requires: requires, Principal: principal,
	})
	c.declared[name] = true
}

func (c *compiler) compileWireDecl(callExpr *ast.CallExpr) {
	cc := &call{verb: "wire", args: callExpr.Args, pos: callExpr.Pos, diags: c.diags}
	to, toOK := cc.ref("to", true, c.declared)
	key, keyOK := cc.str("key", true)
	from, fromOK := cc.ref("from", true, c.declared)
	field, hasField := cc.str("field", false)
	if !hasField {
		field = "port"
	}
	if !toOK || !keyOK || !fromOK {
		return
	}
	c.wires = append(c.wires, WireDecl{ToPlugin: to, Key: key, FromPlugin: from, Field: field})
}

// call agrupa lo que un handler de verbo necesita para leer sus
// argumentos con mensajes de error consistentes — mismo patrón que el
// `call` de pluginmanifest/providerspec, con el agregado de ref() (ver
// doc comment del paquete).
type call struct {
	verb  string
	args  []ast.Arg
	pos   diagnostics.Position
	diags *diagnostics.Bag
}

func (c *call) find(key string) (ast.Expr, bool) {
	for _, a := range c.args {
		if a.Name == key {
			return a.Value, true
		}
	}
	return nil, false
}

func (c *call) str(key string, required bool) (string, bool) {
	v, ok := c.find(key)
	if !ok {
		if required {
			c.diags.Errorf(c.pos, "ASTR501", "System.%s: falta el argumento obligatorio %q", c.verb, key)
		}
		return "", false
	}
	lit, ok := v.(*ast.StringLit)
	if !ok {
		c.diags.Errorf(v.Position(), "ASTR502", "System.%s: %q debe ser un string, no %s", c.verb, key, exprKind(v))
		return "", false
	}
	return lit.Value, true
}

func (c *call) boolVal(key string, def bool) bool {
	v, ok := c.find(key)
	if !ok {
		return def
	}
	lit, ok := v.(*ast.BoolLit)
	if !ok {
		c.diags.Errorf(v.Position(), "ASTR502", "System.%s: %q debe ser True/False, no %s", c.verb, key, exprKind(v))
		return def
	}
	return lit.Value
}

func (c *call) stringList(key string) []string {
	v, ok := c.find(key)
	if !ok {
		return nil
	}
	list, ok := v.(*ast.ListLit)
	if !ok {
		c.diags.Errorf(v.Position(), "ASTR502", "System.%s: %q debe ser una lista, no %s", c.verb, key, exprKind(v))
		return nil
	}
	out := make([]string, 0, len(list.Elements))
	for _, el := range list.Elements {
		lit, ok := el.(*ast.StringLit)
		if !ok {
			c.diags.Errorf(el.Position(), "ASTR502", "System.%s: los elementos de %q deben ser strings, no %s", c.verb, key, exprKind(el))
			continue
		}
		out = append(out, lit.Value)
	}
	return out
}

// ref lee un argumento que debe ser una referencia (Ident) a un plugin
// ya declarado antes en el archivo con System.plugin(...) — ver el doc
// comment del paquete sobre por qué esto es nuevo en todo el repo.
func (c *call) ref(key string, required bool, declared map[string]bool) (string, bool) {
	v, ok := c.find(key)
	if !ok {
		if required {
			c.diags.Errorf(c.pos, "ASTR501", "System.%s: falta el argumento obligatorio %q", c.verb, key)
		}
		return "", false
	}
	ident, ok := v.(*ast.Ident)
	if !ok {
		c.diags.Errorf(v.Position(), "ASTR503",
			"System.%s: %q debe ser una referencia a un plugin declarado con System.plugin(...) más arriba, no %s",
			c.verb, key, exprKind(v))
		return "", false
	}
	if !declared[ident.Name] {
		c.diags.Errorf(v.Position(), "ASTR504",
			"System.%s: %q referencia a %q, que no fue declarado con System.plugin(...) antes de esta línea",
			c.verb, key, ident.Name)
		return "", false
	}
	return ident.Name, true
}

func exprKind(e ast.Expr) string {
	switch e.(type) {
	case *ast.StringLit:
		return "un string"
	case *ast.IntLit:
		return "un entero"
	case *ast.FloatLit:
		return "un float"
	case *ast.BoolLit:
		return "un booleano"
	case *ast.ListLit:
		return "una lista"
	case *ast.SizeLit:
		return "un tamaño (ej. 8GB)"
	case *ast.DurationLit:
		return "una duración (ej. 30s)"
	case *ast.Ident:
		return "una referencia a otra variable"
	default:
		return "otra cosa"
	}
}
