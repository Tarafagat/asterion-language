// Package providerspec compila un *ast.Program (el mismo AST que usa
// 'asterion language check') hacia specs de recursos de proveedor listos
// para aplicar de verdad — hoy, solo instancias de cómputo
// (Provider.<code>.instance(...)), el único recurso con un adapter real
// del otro lado (GCP.CreateInstance, en asterion-core). Es el traductor
// que el propio README de este repo marcaba como pendiente para que
// 'apply' dejara de ser aspiracional.
//
// Igual que pluginmanifest, este paquete NO se apoya en semantic.Analyzer
// (que solo devuelve un *diagnostics.Bag, nunca un AST anotado ni una
// tabla de símbolos reusable — ver su propio código) ni en su helper
// interno providerCall (privado del paquete semantic, no importable):
// vuelve a recorrer prog.Statements con su propio walker chico, mismo
// criterio que pluginmanifest/compile.go.
//
// InstanceSpec es un tipo propio, no adapters.InstanceSpec de
// asterion-core — este repo es un módulo Go independiente y ese tipo
// vive bajo internal/ del otro, ni siquiera sería importable. Quien
// llame a Compile (asterion-core, que sí puede importar ambos) hace la
// conversión final campo a campo.
package providerspec

import (
	"github.com/Tarafagat/asterion-language/ast"
	"github.com/Tarafagat/asterion-language/diagnostics"
)

// InstanceSpec es una instancia de cómputo lista para aplicar. Region,
// para GCP, es en realidad una ZONA (ej. "us-central1-a") — mismo
// convenio ya documentado del lado del adapter real (InstanceSpec de
// asterion-core no tiene un campo Zone dedicado).
type InstanceSpec struct {
	Name           string `json:"name"`
	Provider       string `json:"provider"`
	Region         string `json:"region"`
	ShapeCode      string `json:"shape_code"`
	Image          string `json:"image"`
	Network        string `json:"network,omitempty"`
	Subnet         string `json:"subnet,omitempty"`
	AssignPublicIP bool   `json:"assign_public_ip"`
}

// supportedProviders son los proveedores con CreateInstance real del
// otro lado — hoy, solo GCP. Pedir Provider.aws.instance(...) (o
// azure/oci) da un diagnóstico claro, nunca se ignora en silencio: esos
// tres siguen ErrNotImplemented en asterion-core.
var supportedProviders = map[string]bool{"gcp": true}

// CompileInstances recorre prog.Statements buscando
// `nombre = Provider.<code>.instance(...)` y devuelve un InstanceSpec por
// cada uno encontrado, en el orden en que aparecen. El Bag devuelto nunca
// es nil — puede estar vacío (sin errores) o traer uno o más
// diagnósticos; igual que el resto del compilador, no corta en el primer
// error.
func CompileInstances(prog *ast.Program) ([]InstanceSpec, *diagnostics.Bag) {
	c := &compiler{diags: &diagnostics.Bag{}}
	for _, stmt := range prog.Statements {
		c.walkStmt(stmt)
	}
	return c.specs, c.diags
}

type compiler struct {
	specs []InstanceSpec
	diags *diagnostics.Bag
}

// walkStmt solo reacciona a AssignStmt cuyo Value sea una llamada
// Provider.<code>.instance(...) — cualquier otro statement (def, return,
// una llamada a otra cosa) se ignora sin error: un programa de
// Asterion Language puede tener statements que no le importan a apply
// todavía (ej. Network(...), Lab.vm(...)), y no es este paso el que
// decide si esos son válidos — eso ya lo hizo (o lo hará)
// semantic.Analyze.
func (c *compiler) walkStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		callExpr, ok := s.Value.(*ast.CallExpr)
		if !ok {
			return
		}
		provider, method, ok := providerCall(callExpr.Callee)
		if !ok {
			// No es una llamada Provider.<code>.<method> — puede ser
			// cualquier otra cosa (Lab.vm(...), una expresión normal);
			// no es este paso el que decide si eso es válido.
			return
		}
		if method != "instance" {
			c.diags.Errorf(callExpr.Pos, "ASTR402",
				"Provider.%s.%s(...) todavía no está soportado para apply — hoy solo .instance() tiene un traductor", provider, method)
			return
		}
		c.compileInstance(s.Name, provider, callExpr)
	case *ast.FuncDecl:
		for _, inner := range s.Body {
			c.walkStmt(inner)
		}
	}
}

// providerCall reconoce la forma Provider.<code>.<method> — copia chica
// de la función homónima (privada) de semantic.Analyzer, mismo criterio
// que pluginmanifest.contractVerb ya usa para Contract.<verbo>: el
// original no se puede importar de otro paquete.
func providerCall(callee ast.Expr) (provider, method string, ok bool) {
	methodAttr, isAttr := callee.(*ast.AttrExpr)
	if !isAttr {
		return "", "", false
	}
	providerAttr, isAttr := methodAttr.X.(*ast.AttrExpr)
	if !isAttr {
		return "", "", false
	}
	root, isIdent := providerAttr.X.(*ast.Ident)
	if !isIdent || root.Name != "Provider" {
		return "", "", false
	}
	return providerAttr.Name, methodAttr.Name, true
}

func (c *compiler) compileInstance(name, provider string, callExpr *ast.CallExpr) {
	if !supportedProviders[provider] {
		c.diags.Errorf(callExpr.Pos, "ASTR403",
			"Provider.%s.instance(...) todavía no está soportado para apply — hoy solo gcp tiene un adapter real del otro lado", provider)
		return
	}

	cc := &call{provider: provider, args: callExpr.Args, pos: callExpr.Pos, diags: c.diags}
	region, _ := cc.str("region", true)
	shapeCode, _ := cc.str("shape_code", true)
	image, _ := cc.str("image", true)
	network, _ := cc.str("network", false)
	subnet, _ := cc.str("subnet", false)

	c.specs = append(c.specs, InstanceSpec{
		Name:           name,
		Provider:       provider,
		Region:         region,
		ShapeCode:      shapeCode,
		Image:          image,
		Network:        network,
		Subnet:         subnet,
		AssignPublicIP: cc.boolVal("assign_public_ip", false),
	})
}

// call agrupa lo que compileInstance necesita para leer sus argumentos
// con mensajes de error consistentes — mismo patrón que el `call` de
// pluginmanifest/compile.go, adaptado a "Provider.<code>.instance" en
// vez de "Contract.<verbo>".
type call struct {
	provider string
	args     []ast.Arg
	pos      diagnostics.Position
	diags    *diagnostics.Bag
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
			c.diags.Errorf(c.pos, "ASTR400", "Provider.%s.instance: falta el argumento obligatorio %q", c.provider, key)
		}
		return "", false
	}
	lit, ok := v.(*ast.StringLit)
	if !ok {
		c.diags.Errorf(v.Position(), "ASTR401", "Provider.%s.instance: %q debe ser un string, no %s", c.provider, key, exprKind(v))
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
		c.diags.Errorf(v.Position(), "ASTR401", "Provider.%s.instance: %q debe ser True/False, no %s", c.provider, key, exprKind(v))
		return def
	}
	return lit.Value
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
		return "una referencia a otra variable (todavía no soportado acá)"
	default:
		return "otra cosa"
	}
}
