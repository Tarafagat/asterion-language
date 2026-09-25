// Package pluginmanifest compila un *ast.Program (el mismo AST que usa
// 'asterion language check') a un apc.Manifest — el equivalente, para
// definir un plugin nuevo, de lo que 'asterion plugin from-openapi' ya
// hace a partir de un openapi.yaml, pero sin heurística: acá el autor
// declara cada campo explícitamente con llamadas Contract.<verbo>(...),
// así que no hay nada que adivinar mal.
//
// Un archivo de definición de plugin es sintácticamente un .asterion
// normal (mismo lexer, mismo parser — nada de este paquete los toca) pero
// semánticamente distinto de un .asterion de infraestructura: es una
// secuencia plana de llamadas Contract.<verbo>(clave=valor, ...), nunca
// def, nunca asignaciones, nunca Provider.*/Lab.*. Por eso este compilador
// NO pasa por semantic.Analyzer (que asume el modelo de recursos/scope de
// infraestructura) — es su propio walker, chico, que solo entiende esa
// forma. Provider.* y Lab.* siguen reservados para .asterion de
// infraestructura, y Plugin.* sigue reservado para *usar* un plugin ya
// instalado (ver examples/plugin.asterion) — "Contract" es un builtin
// nuevo, sin colisión con ninguno de los dos.
//
// Deliberadamente NO llama apc.Manifest.Validate() acá — esa validación ya
// existe, ya está probada, y vive en asterion-plugin-contract/apc; correrla
// de nuevo acá duplicaría las reglas en dos lugares. Este paquete solo
// traduce sintaxis a datos; quien lo invoca (asterion-core, 'asterion
// plugin from-asterion') decide cuándo validar el resultado.
package pluginmanifest

import (
	"github.com/Tarafagat/asterion-language/ast"
	"github.com/Tarafagat/asterion-language/diagnostics"
	"github.com/Tarafagat/asterion-plugin-contract/apc"
)

// Compile recorre prog.Statements y arma un apc.Manifest. El Bag devuelto
// nunca es nil — puede estar vacío (sin errores) o tener uno o más
// diagnósticos; igual que el resto del compilador, no corta en el primer
// error, reporta todos los que encuentre en la misma corrida.
func Compile(prog *ast.Program) (apc.Manifest, *diagnostics.Bag) {
	c := &compiler{diags: &diagnostics.Bag{}}
	c.manifest.ContractVersion = apc.ContractVersion
	for _, stmt := range prog.Statements {
		c.walkStmt(stmt)
	}
	if !c.defined {
		c.diags.Errorf(prog.Pos, "ASTR305", "el archivo nunca llamó Contract.define(...) — no hay nada que compilar")
	}
	return c.manifest, c.diags
}

type compiler struct {
	manifest apc.Manifest
	diags    *diagnostics.Bag

	// Flags de "llamado una vez" — ver onceGuard.
	defined     bool
	languageSet bool
	startSet    bool
	healthSet   bool
	apiSet      bool
	permsSet    bool
	eventsSet   bool
}

func (c *compiler) walkStmt(stmt ast.Stmt) {
	exprStmt, ok := stmt.(*ast.ExprStmt)
	if !ok {
		c.diags.Errorf(stmt.Position(), "ASTR300",
			"se esperaba una llamada Contract.<verbo>(...) — un archivo de definición de plugin es una secuencia plana de esas llamadas, sin def ni asignaciones")
		return
	}
	callExpr, ok := exprStmt.X.(*ast.CallExpr)
	if !ok {
		c.diags.Errorf(exprStmt.Pos, "ASTR300", "se esperaba una llamada Contract.<verbo>(...)")
		return
	}
	verb, ok := contractVerb(callExpr.Callee)
	if !ok {
		c.diags.Errorf(callExpr.Pos, "ASTR300", "se esperaba una llamada con forma Contract.<verbo>(...) — esta no la tiene")
		return
	}
	cc := &call{verb: verb, args: callExpr.Args, pos: callExpr.Pos, diags: c.diags}
	c.dispatch(verb, cc, callExpr)
}

// contractVerb reconoce la forma Contract.<verbo> en el callee de un
// CallExpr — Contract.resource(...) es AttrExpr{X: Ident("Contract"), Name: "resource"}.
func contractVerb(callee ast.Expr) (verb string, ok bool) {
	attr, isAttr := callee.(*ast.AttrExpr)
	if !isAttr {
		return "", false
	}
	root, isIdent := attr.X.(*ast.Ident)
	if !isIdent || root.Name != "Contract" {
		return "", false
	}
	return attr.Name, true
}

func (c *compiler) dispatch(verb string, cc *call, callExpr *ast.CallExpr) {
	switch verb {
	case "define":
		c.onceGuard(verb, &c.defined, cc)
		name, _ := cc.str("name", true)
		version, _ := cc.str("version", true)
		description, _ := cc.str("description", false)
		author, _ := cc.str("author", false)
		license, _ := cc.str("license", false)
		repo, _ := cc.str("repo", false)
		c.manifest.Name = name
		c.manifest.Version = version
		c.manifest.Description = description
		c.manifest.Author = author
		c.manifest.License = license
		c.manifest.Repo = repo
		c.defined = true

	case "language":
		c.onceGuard(verb, &c.languageSet, cc)
		name, _ := cc.str("name", true)
		version, _ := cc.str("version", false)
		venv, _ := cc.str("venv", false)
		requirements, _ := cc.str("requirements", false)
		c.manifest.Language = &apc.LanguageSpec{Name: name, Version: version, Venv: venv, Requirements: requirements}

	case "start":
		c.onceGuard(verb, &c.startSet, cc)
		command, _ := cc.str("command", true)
		port := cc.intVal("port", 0)
		args := cc.stringList("args")
		c.manifest.Start = apc.StartSpec{Command: command, Args: args}
		c.manifest.Port = port

	case "health_path":
		c.onceGuard(verb, &c.healthSet, cc)
		path, _ := cc.str("path", true)
		c.manifest.HealthPath = path

	case "api":
		c.onceGuard(verb, &c.apiSet, cc)
		basePath, _ := cc.str("base_path", false)
		openapi, _ := cc.str("openapi", false)
		c.manifest.API = &apc.APISpec{BasePath: basePath, OpenAPI: openapi}

	case "permissions":
		c.onceGuard(verb, &c.permsSet, cc)
		c.manifest.Permissions = &apc.PermissionsSpec{
			Network:    cc.stringList("network"),
			Filesystem: cc.stringList("filesystem"),
			Database:   cc.boolVal("database", false),
			Secrets:    cc.boolVal("secrets", false),
		}

	case "events":
		c.onceGuard(verb, &c.eventsSet, cc)
		c.manifest.Events = &apc.EventsSpec{
			Publishes:  cc.stringList("publishes"),
			Subscribes: cc.stringList("subscribes"),
		}

	case "config":
		key, _ := cc.str("key", true)
		label, _ := cc.str("label", false)
		typ, _ := cc.str("type", false)
		def, _ := cc.str("default", false)
		c.manifest.ConfigSchema = append(c.manifest.ConfigSchema, apc.ConfigField{
			Key: key, Label: label, Type: typ, Default: def,
			Secret:   cc.boolVal("secret", false),
			Required: cc.boolVal("required", false),
		})

	case "resource":
		name, _ := cc.str("name", true)
		endpoint, _ := cc.str("endpoint", true)
		schema, _ := cc.str("schema", false)
		primaryKey, _ := cc.str("primary_key", false)
		c.manifest.Resources = append(c.manifest.Resources, apc.ResourceSpec{
			Name: name, Endpoint: endpoint, Schema: schema, PrimaryKey: primaryKey,
			CRUD: cc.stringList("crud"),
		})

	case "action":
		name, _ := cc.str("name", true)
		method, _ := cc.str("method", true)
		endpoint, _ := cc.str("endpoint", true)
		description, _ := cc.str("description", false)
		c.manifest.Actions = append(c.manifest.Actions, apc.ActionSpec{
			Name: name, Method: method, Endpoint: endpoint, Description: description,
		})

	default:
		c.diags.Errorf(callExpr.Pos, "ASTR301",
			"Contract.%s no existe — verbos reconocidos: define, language, start, health_path, api, permissions, events, config, resource, action", verb)
	}
}

// onceGuard reporta ASTR304 si este verbo ya se vio antes en el archivo —
// define/language/start/health_path/api/permissions/events describen un
// solo aspecto del manifiesto cada uno, a diferencia de config/resource/
// action que son listas y se acumulan por diseño.
func (c *compiler) onceGuard(verb string, seen *bool, cc *call) {
	if *seen {
		c.diags.Errorf(cc.pos, "ASTR304", "Contract.%s ya se llamó antes en este archivo — solo puede aparecer una vez", verb)
	}
	*seen = true
}

// call agrupa lo que un handler de verbo necesita para leer sus
// argumentos con mensajes de error consistentes (siempre "Contract.<verbo>:
// ...", siempre anclados a la posición correcta).
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
			c.diags.Errorf(c.pos, "ASTR302", "Contract.%s: falta el argumento obligatorio %q", c.verb, key)
		}
		return "", false
	}
	lit, ok := v.(*ast.StringLit)
	if !ok {
		c.diags.Errorf(v.Position(), "ASTR303", "Contract.%s: %q debe ser un string, no %s", c.verb, key, exprKind(v))
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
		c.diags.Errorf(v.Position(), "ASTR303", "Contract.%s: %q debe ser True/False, no %s", c.verb, key, exprKind(v))
		return def
	}
	return lit.Value
}

func (c *call) intVal(key string, def int) int {
	v, ok := c.find(key)
	if !ok {
		return def
	}
	lit, ok := v.(*ast.IntLit)
	if !ok {
		c.diags.Errorf(v.Position(), "ASTR303", "Contract.%s: %q debe ser un entero, no %s", c.verb, key, exprKind(v))
		return def
	}
	return int(lit.Value)
}

func (c *call) stringList(key string) []string {
	v, ok := c.find(key)
	if !ok {
		return nil
	}
	list, ok := v.(*ast.ListLit)
	if !ok {
		c.diags.Errorf(v.Position(), "ASTR303", "Contract.%s: %q debe ser una lista, no %s", c.verb, key, exprKind(v))
		return nil
	}
	out := make([]string, 0, len(list.Elements))
	for _, el := range list.Elements {
		lit, ok := el.(*ast.StringLit)
		if !ok {
			c.diags.Errorf(el.Position(), "ASTR303", "Contract.%s: los elementos de %q deben ser strings, no %s", c.verb, key, exprKind(el))
			continue
		}
		out = append(out, lit.Value)
	}
	return out
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
	default:
		return "otra cosa"
	}
}
