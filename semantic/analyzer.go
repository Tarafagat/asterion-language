// Package semantic valida un *ast.Program: que toda referencia exista,
// que los recursos declarados con Provider.<code>.<método>(...) usen un
// provider y una capability reales, y que no haya dos recursos con el
// mismo nombre lógico en el mismo scope. No ejecuta nada — es exactamente
// el paso "check" de `asterion language check`, nunca "plan" ni "apply".
//
// Restricción explícita de v0.1: un nombre solo puede usarse DESPUÉS de
// haber sido declarado (una sola pasada, de arriba a abajo, sin
// referencias hacia adelante). Esto es una simplificación deliberada, no
// un descuido: mientras esa regla valga, el grafo de dependencias que se
// arma (resourceInfo.DependsOn) es un DAG por construcción — no hace falta
// un detector de ciclos aparte. Permitir declarar en cualquier orden (como
// Terraform) es una extensión futura documentable, no parte de v0.1.
package semantic

import (
	"strings"

	"github.com/Tarafagat/asterion-language/ast"
	"github.com/Tarafagat/asterion-language/diagnostics"
)

// resourceKind identifica qué clase de recurso declaró un CallExpr, para
// que el futuro compilador hacia AIR sepa hacia qué dominio traducirlo
// (Lab, Plugin, Cloud — ver la propuesta de integración). Vacío significa
// "no es un recurso, es una llamada/valor común".
type resourceKind struct {
	Domain string // "provider" | "lab" | "generic" | ""
	Detail string // ej. "aws.instance", "vm", "Network"
}

type binding struct {
	IsResource bool
	Kind       resourceKind
	DeclPos    diagnostics.Position
	DependsOn  []string
}

type scope struct {
	names  map[string]*binding
	parent *scope
}

func newScope(parent *scope) *scope {
	return &scope{names: map[string]*binding{}, parent: parent}
}

func (s *scope) declare(name string, b *binding) { s.names[name] = b }

func (s *scope) lookup(name string) (*binding, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if b, ok := cur.names[name]; ok {
			return b, true
		}
	}
	return nil, false
}

func (s *scope) lookupLocal(name string) (*binding, bool) {
	b, ok := s.names[name]
	return b, ok
}

// genericResourceTypes son los constructores "bare" que spec/resources.md
// reconoce sin pasar por Provider.<code> — ej. `Network(cidr=...)`. No
// llevan chequeo de capability propio: a qué provider terminan asociados
// es una pregunta abierta, marcada PLANNED en la documentación (ver
// README), no resuelta unilateralmente acá.
var genericResourceTypes = map[string]bool{
	"Network":   true,
	"Instance":  true,
	"Storage":   true,
	"Image":     true,
	"Container": true,
}

// builtinRoots son los identificadores que el lenguaje reserva como raíz
// de un namespace — nunca se resuelven contra el scope de variables
// porque no son valores, son espacios de nombres (Provider.aws.*,
// Lab.*, Plugin.* — esta última con sintaxis todavía PLANNED, ver
// examples/plugin.asterion) o constructores de tipo genéricos.
func isBuiltinRoot(name string) bool {
	switch name {
	case "Provider", "Lab", "Plugin":
		return true
	}
	return genericResourceTypes[name]
}

type Analyzer struct {
	resolver CapabilityResolver
	diags    *diagnostics.Bag
}

// NewAnalyzer crea un analyzer. resolver nil usa StaticCapabilityResolver
// (el snapshot de referencia) — asterion-core, al invocar esto, pasa un
// resolver respaldado por su Registry real en cambio.
func NewAnalyzer(resolver CapabilityResolver) *Analyzer {
	if resolver == nil {
		resolver = StaticCapabilityResolver{}
	}
	return &Analyzer{resolver: resolver, diags: &diagnostics.Bag{}}
}

func (a *Analyzer) Analyze(prog *ast.Program) *diagnostics.Bag {
	root := newScope(nil)
	a.walkStmts(prog.Statements, root)
	return a.diags
}

func (a *Analyzer) walkStmts(stmts []ast.Stmt, sc *scope) {
	for _, s := range stmts {
		a.walkStmt(s, sc)
	}
}

func (a *Analyzer) walkStmt(s ast.Stmt, sc *scope) {
	switch n := s.(type) {
	case *ast.FuncDecl:
		if _, exists := sc.lookupLocal(n.Name); exists {
			a.diags.Errorf(n.Pos, "ASTR200", "%q ya está declarado en este scope", n.Name)
		}
		sc.declare(n.Name, &binding{DeclPos: n.Pos})
		fscope := newScope(sc)
		for _, param := range n.Params {
			fscope.declare(param, &binding{DeclPos: n.Pos})
		}
		a.walkStmts(n.Body, fscope)

	case *ast.AssignStmt:
		a.walkExpr(n.Value, sc)
		kind := a.classify(n.Value, sc)
		if existing, ok := sc.lookupLocal(n.Name); ok && existing.IsResource {
			a.diags.Errorf(n.Pos, "ASTR201", "%q ya fue declarado como recurso (línea %d) — los nombres de recurso deben ser únicos dentro de su scope", n.Name, existing.DeclPos.Line)
		}
		sc.declare(n.Name, &binding{
			IsResource: kind.Domain != "",
			Kind:       kind,
			DeclPos:    n.Pos,
			DependsOn:  a.collectRefs(n.Value, sc),
		})

	case *ast.ReturnStmt:
		for _, v := range n.Values {
			a.walkExpr(v, sc)
		}

	case *ast.ExprStmt:
		a.walkExpr(n.X, sc)
	}
}

// walkExpr resuelve toda referencia (Ident) contenida en expr, y si expr
// es una llamada con forma de recurso, valida provider/capability.
func (a *Analyzer) walkExpr(expr ast.Expr, sc *scope) {
	switch e := expr.(type) {
	case *ast.Ident:
		if isBuiltinRoot(e.Name) {
			return // builtins del lenguaje, no se resuelven contra el scope
		}
		if _, ok := sc.lookup(e.Name); !ok {
			a.diags.Errorf(e.Pos, "ASTR202", "%q no está definido — ¿falta declararlo antes de esta línea?", e.Name)
		}
	case *ast.AttrExpr:
		a.walkExpr(e.X, sc)
	case *ast.CallExpr:
		a.walkCall(e, sc)
	case *ast.ListLit:
		for _, el := range e.Elements {
			a.walkExpr(el, sc)
		}
	}
	// StringLit/IntLit/FloatLit/SizeLit/DurationLit/BoolLit: nada que resolver.
}

func (a *Analyzer) walkCall(call *ast.CallExpr, sc *scope) {
	a.walkExpr(call.Callee, sc)
	for _, arg := range call.Args {
		a.walkExpr(arg.Value, sc)
	}
	if provider, method, ok := providerCall(call.Callee); ok {
		a.checkProviderCall(call, provider, method)
	}
}

// providerCall reconoce la forma Provider.<code>.<método> en el callee de
// un CallExpr — Provider.aws.instance(...) es AttrExpr{ X: AttrExpr{ X:
// Ident("Provider"), Name: "aws" }, Name: "instance" }.
func providerCall(callee ast.Expr) (provider, method string, ok bool) {
	outer, isAttr := callee.(*ast.AttrExpr)
	if !isAttr {
		return "", "", false
	}
	inner, isAttr := outer.X.(*ast.AttrExpr)
	if !isAttr {
		return "", "", false
	}
	root, isIdent := inner.X.(*ast.Ident)
	if !isIdent || root.Name != "Provider" {
		return "", "", false
	}
	return inner.Name, outer.Name, true
}

func (a *Analyzer) checkProviderCall(call *ast.CallExpr, provider, method string) {
	known := false
	for _, p := range a.resolver.Providers() {
		if p == provider {
			known = true
			break
		}
	}
	if !known {
		a.diags.Errorf(call.Pos, "ASTR210", "provider %q no reconocido — providers disponibles: %s", provider, strings.Join(a.resolver.Providers(), ", "))
		return
	}

	capability, recognized := ResourceCapability[method]
	if !recognized {
		known := make([]string, 0, len(ResourceCapability))
		for m := range ResourceCapability {
			known = append(known, m)
		}
		a.diags.Errorf(call.Pos, "ASTR211", "Provider.%s.%s no existe — Asterion Core hoy solo expone: %s", provider, method, strings.Join(known, ", "))
		return
	}

	if !a.resolver.HasCapability(provider, capability) {
		available := []string{}
		for m, c := range ResourceCapability {
			if a.resolver.HasCapability(provider, c) {
				available = append(available, m)
			}
		}
		detail := "Required capability:\n    " + capability + "\n\nAvailable operations:\n    " + strings.Join(available, "\n    ") +
			"\n\nMissing:\n    " + capability + "\n\nNo infrastructure was modified."
		a.diags.Add(diagnostics.Diagnostic{
			Severity: diagnostics.SeverityError,
			Code:     "ASTR212",
			Pos:      call.Pos,
			Message:  "provider " + provider + " no declara la capability requerida por " + method,
			Detail:   detail,
		})
	}
}

// classify decide si expr es una declaración de recurso y de qué tipo —
// ver resourceKind. No es un error que NO lo sea: la mayoría de las
// expresiones (literales, llamadas comunes) legítimamente no declaran
// nada.
func (a *Analyzer) classify(expr ast.Expr, sc *scope) resourceKind {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return resourceKind{}
	}
	if provider, method, ok := providerCall(call.Callee); ok {
		return resourceKind{Domain: "provider", Detail: provider + "." + method}
	}
	if outer, ok := call.Callee.(*ast.AttrExpr); ok {
		if root, ok := outer.X.(*ast.Ident); ok && root.Name == "Lab" {
			return resourceKind{Domain: "lab", Detail: outer.Name}
		}
	}
	if ident, ok := call.Callee.(*ast.Ident); ok && genericResourceTypes[ident.Name] {
		return resourceKind{Domain: "generic", Detail: ident.Name}
	}
	return resourceKind{}
}

// collectRefs junta los nombres de recursos ya declarados que expr
// referencia — es la lista de dependencias que un futuro compilador hacia
// AIR necesitaría para decidir orden de creación.
func (a *Analyzer) collectRefs(expr ast.Expr, sc *scope) []string {
	var out []string
	var walk func(ast.Expr)
	walk = func(e ast.Expr) {
		switch n := e.(type) {
		case *ast.Ident:
			if b, ok := sc.lookup(n.Name); ok && b.IsResource {
				out = append(out, n.Name)
			}
		case *ast.AttrExpr:
			walk(n.X)
		case *ast.CallExpr:
			walk(n.Callee)
			for _, arg := range n.Args {
				walk(arg.Value)
			}
		case *ast.ListLit:
			for _, el := range n.Elements {
				walk(el)
			}
		}
	}
	walk(expr)
	return out
}
