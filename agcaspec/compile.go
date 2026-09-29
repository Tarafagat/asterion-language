// Package agcaspec compila un *ast.Program a la descripción de una
// INTELIGENCIA de Asterion Graph Cognitive Architecture (AGCA) — un cuarto
// uso del mismo lenguaje, namespace AGCA.*, distinto de infraestructura
// (Provider.*/Lab.*, semantic.Analyzer), manifiesto de plugin (Contract.*,
// pluginmanifest) y sistema de plugins (System.*, systemspec). Ver
// docs/TUTORIAL.md § 8 y spec/grammar.md § "DSL de inteligencia cognitiva"
// para la gramática completa y el porqué de esta capa.
//
// La sintaxis de bloques anidados con llaves que propone el documento de
// investigación original ("intelligence X { graph Y { ... } agents {
// swarm Z { instances = 128 } } }") NO es la gramática de Asterion
// Language — este lenguaje no tiene llaves ni bloques anónimos (ver
// spec/grammar.md § Léxico: los únicos símbolos son "( ) [ ] , . = : ?").
// agcaspec adapta esa misma idea a la forma que YA usan Contract.*/
// System.*: una secuencia de llamadas AGCA.<verbo>(clave=valor, ...),
// cada una asignada a un nombre de variable que las demás referencian
// después (mismo criterio de "declarar antes de usar" de siempre). Esto
// no es una simplificación cosmética: reusa el lexer/parser/diagnostics
// existentes tal cual, sin agregar un solo token nuevo.
//
// Es el segundo compilador de este repo (después de systemspec) que
// resuelve referencias reales entre variables del mismo archivo —
// AGCA.neuron(intelligence=brain, ...) exige que `brain` sea un
// AGCA.intelligence(...) ya declarado, no cualquier nombre: ver
// call.ref, que además valida el KIND de lo referenciado (a diferencia
// de systemspec.call.ref, que solo pedía "una referencia a algo
// declarado con System.plugin").
//
// Import(path) trae los plugins de OTRO archivo .asterion (uno que
// declare un sistema con System.plugin(...)/System.wire(...), ver
// systemspec) para poder referenciarlos desde este archivo sin copiar su
// contenido a mano — ver ImportDecl y compileImport. Es la única forma
// en que agcaspec conecta con systemspec: un archivo de inteligencia
// puede apoyarse en un sistema de plugins YA declarado en otro lado, en
// vez de que ambos vivan mezclados en el mismo archivo. Hoy es de un
// solo nivel (un archivo AGCA importa un archivo System — no hay
// import anidado ni un archivo System importando a otro): extenderlo es
// straightforward (mismo mecanismo, otro caller) pero deliberadamente no
// implementado hasta que haga falta de verdad.
package agcaspec

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Tarafagat/asterion-language/ast"
	"github.com/Tarafagat/asterion-language/diagnostics"
	langparser "github.com/Tarafagat/asterion-language/parser"
	"github.com/Tarafagat/asterion-language/systemspec"
)

// Kind identifica qué verbo AGCA.* declaró un nombre — usado para que
// call.ref pueda rechazar, por ejemplo, `intelligence=miGrafo` (un nombre
// real, pero del tipo equivocado).
type Kind string

const (
	KindIntelligence Kind = "intelligence"
	KindGraph        Kind = "graph"
	KindNeuron       Kind = "neuron"
	KindSwarm        Kind = "swarm"
	KindAgent        Kind = "agent"
	KindMemory       Kind = "memory"
	KindPolicy       Kind = "policy"
	KindBot          Kind = "bot"
	KindSecret       Kind = "secret"
	KindImport       Kind = "import"
	KindTool         Kind = "tool"
	KindCapability   Kind = "capability"
	KindRole         Kind = "role"
)

// RoleDecl es AGCA.role(...) — la autoridad de un USUARIO, distinta de la
// de un agente. Un agente es "quién actúa" dentro de la inteligencia; un
// rol es "en nombre de quién": el mismo agente, operado por un viewer o
// por un admin, no puede hacer las mismas cosas.
//
// Inherits compone roles (un analyst hereda lo del viewer y suma lo
// suyo). La regla de composición es la única segura: DENY GANA SIEMPRE,
// venga de donde venga — si cualquier rol de la cadena deniega una
// capability, ningún allow posterior la reabre. Allow vacío en toda la
// cadena significa "este rol no puede invocar nada" (deny-by-default).
//
// Users mapea identidades concretas a este rol, para que el CLI pueda
// resolver "qué puede hacer ESTE usuario" sin que nadie declare su rol a
// mano en cada invocación.
type RoleDecl struct {
	VarName      string
	Name         string
	Description  string
	Intelligence string
	Allow        []string
	Deny         []string
	Inherits     []string // VarNames de otros RoleDecl
	Users        []string
}

// ToolDecl es Tool.define(...) — una herramienta con la que una
// Intelligence puede ACTUAR sobre un World. Una Tool no es ejecutable por
// sí misma: solo agrupa capabilities declaradas con Tool.capability(...),
// y AGCA nunca puede invocar nada fuera de ese conjunto explícito (ver el
// doc comment de CapabilityContractDecl).
type ToolDecl struct {
	VarName  string
	Name     string
	Category string
	// Isolation es "sandbox" para una Tool que ejecuta código (ver § 9 del
	// pedido de Experience: SandboxedPython) — vacío para una Tool normal.
	// Este compilador solo lo traduce a datos; forzar el aislamiento de
	// verdad es responsabilidad del runtime que registre su handler.
	Isolation string
}

// CapabilityContractDecl es Tool.capability(...) — el contrato explícito
// de UNA operación de una Tool. Es la unidad que AGCA puede seleccionar:
// nunca un string arbitrario, nunca código generado. Su ID estable es
// "<tool en minúsculas>.<name>" (ej. "statistics.search_series") y es lo
// único que viaja desde una Decision hasta el runtime, que lo resuelve
// contra un handler registrado de antemano.
//
// Effects/Requires/Guarantees son vocabulario declarativo que el runtime
// consulta ANTES de ejecutar: Effects dice qué puede pasarle al World
// (read_only, pure, mutating, external_write, destructive, más tags
// libres como "creates:Payment"), Requires son precondiciones
// verificables (ej. "network.available"), Guarantees es lo que el
// contrato promete devolver (ej. "returns:Dataset") — y es contra esas
// garantías que la Experience evalúa después si la ejecución cumplió.
type CapabilityContractDecl struct {
	VarName     string
	Tool        string // VarName del ToolDecl al que pertenece
	ID          string // "<tool>.<name>", estable, lo que selecciona una Decision
	Name        string
	Description string
	Input       []string
	Output      []string
	Effects     []string
	Requires    []string
	Guarantees  []string
}

// IntelligenceDecl es AGCA.intelligence(name=...) — el contenedor lógico
// al que se cuelgan graph/neuron/swarm/agent/memory/policy/bot vía su
// argumento intelligence=<ref>. No tiene campos propios más allá del
// nombre: es, deliberadamente, solo un ancla de agrupación — toda
// configuración real vive en los demás verbos.
type IntelligenceDecl struct {
	VarName string // nombre de variable en el .asterion (ej. "brain")
	Name    string // nombre lógico legible (ej. "CompanyBrain")
}

// GraphDecl es AGCA.graph(...) — un Cognitive Graph asociado a una
// Intelligence. Hierarchical/Persistent/Temporal/Provenance son flags de
// qué propiedades mantiene el runtime (ver Ct = (Vt, Eg_t, Θt) en el
// paper) — este compilador solo los traduce a datos, nunca los aplica.
type GraphDecl struct {
	VarName      string
	Name         string
	Intelligence string // ref a un IntelligenceDecl.VarName
	Hierarchical bool
	Persistent   bool
	Temporal     bool
	Provenance   bool
}

// NeuronDecl es AGCA.neuron(...) — una unidad cognitiva intercambiable
// (ver Ni: (x, mi) -> (y, mi') en el paper). Runtime XOR Adapter: Runtime
// es para ejecución local (ej. "gguf" vía llama.cpp), Adapter para un
// proveedor remoto (ej. "remote-llm", "openai-compatible") — ninguno de
// los dos es obligatorio a nivel de compilador (una neurona puramente
// determinista no necesita declarar ninguno), pero declarar los DOS a la
// vez es contradictorio y se rechaza (ASTR606).
type NeuronDecl struct {
	VarName      string
	Name         string
	Intelligence string
	Runtime      string // ej. "gguf" — ejecución local
	Adapter      string // ej. "remote-llm", "openai-compatible" — proveedor remoto
	Model        string // ruta local, o "env:VAR" para leerlo de una variable de entorno en runtime
	Capabilities []string
	Privacy      string // "local" | "remote" (ASTR607 si es otra cosa)
}

// SwarmDecl es AGCA.swarm(...) — instances es "adaptive" o un entero
// positivo en forma de string (validado, nunca negativo ni cero — ver
// call.instances) para que el runtime decida cuántas instancias del swarm
// activar bajo demanda, no para que el compilador imponga un número fijo.
type SwarmDecl struct {
	VarName      string
	Name         string
	Intelligence string
	Instances    string
}

// AgentDecl es AGCA.agent(...) — un agente ejecutivo o especializado que
// coordina neuronas/swarms/capabilities dentro de una Intelligence.
// AgentDecl es AGCA.agent(...). Allow/Deny son la frontera de autoridad
// de ESTE agente sobre las capabilities declaradas con Tool.capability(...)
// — "Tool instalada ≠ Tool accesible" (§ 22 del pedido de Experience):
// que una Tool declare 20 capabilities no significa que un agente pueda
// usar las 20. Deny gana siempre sobre Allow; Allow vacío significa
// "ninguna capability de Tool autorizada todavía" (deny-by-default, mismo
// criterio que Contract.permissions y AGCA.secret.allow) — el runtime lo
// verifica ANTES de resolver ningún handler, nunca después.
type AgentDecl struct {
	VarName      string
	Name         string
	Intelligence string
	Strategy     string
	Allow        []string
	Deny         []string
	// Role es el rol que este agente asume por default (VarName de un
	// RoleDecl). La autoridad efectiva es la INTERSECCIÓN de las dos
	// listas: lo que el rol permite Y el agente permite. Ninguna de las
	// dos puede ampliar a la otra.
	Role string
}

// CapabilityRequirement es AGCA.requires_capability(...) — una directiva
// suelta (nunca asignada a un nombre, igual que System.wire), nunca un
// recurso: declara qué capability necesita una Intelligence de Asterion
// Plugins, sin decir con qué plugin concreto se satisface (eso lo
// resuelve el Capability Router en runtime, no este compilador).
type CapabilityRequirement struct {
	Intelligence string
	Capability   string
}

// MemoryDecl es AGCA.memory(...) — Graph referencia un GraphDecl.VarName
// ya declarado: la memoria de una Intelligence se apoya sobre uno de sus
// propios Cognitive Graphs, nunca uno ajeno.
type MemoryDecl struct {
	VarName      string
	Name         string
	Intelligence string
	Type         string // ej. "hybrid" — sin validación cerrada: el runtime decide qué tipos soporta
	Graph        string // ref a un GraphDecl.VarName
}

// PolicyDecl es AGCA.policy(...). When/Allow/Deny/Prefer/Unless son
// expresiones de política en TEXTO PLANO (ej. "privacy==local"), nunca
// evaluadas ni parseadas por este compilador — un lenguaje de políticas
// real es trabajo futuro del Policy Engine (runtime), no de este DSL.
// Igual que Contract.permissions, este verbo es declarativo puro.
type PolicyDecl struct {
	VarName      string
	Name         string
	Intelligence string
	When         string
	Allow        string
	Deny         string
	Prefer       string
	Unless       string
}

// BotDecl es AGCA.bot(...) — una interfaz hacia una Intelligence ya
// existente (nunca una inteligencia separada, ver § 16 del paper).
// Permissions es qué capabilities puede invocar ESTE bot puntual — puede
// ser un subconjunto de las que su Intelligence ya declaró con
// requires_capability, nunca una vía para pedir una capability nueva sin
// pasar por ahí (este compilador no lo fuerza todavía: queda para el
// Policy Engine del runtime).
type BotDecl struct {
	VarName      string
	Name         string
	Intelligence string
	Interface    string // ej. "terminal", "web", "api"
	Permissions  []string
}

// SecretDecl es AGCA.secret(...) — una REFERENCIA a un secreto (nunca un
// valor: ver el doc comment de Compile). Dos formas mutuamente
// excluyentes de decir DÓNDE vive esa referencia (ASTR609 si se
// declaran las dos, o ninguna):
//
//  1. Literal: Source es la ruta/clave dentro del gestor de secretos
//     (ej. "mycompany/prod/database_password"), escrita a mano.
//  2. Derivada de un plugin ya importado: From/FromPlugin apuntan a un
//     plugin de un Import(...) ya declarado (ej. `from=sys.db` — From
//     = "sys", FromPlugin = "db"), y Field nombra qué clave de SU
//     config_schema es este secreto (ej. `field="config:database_password"`,
//     mismo convenio que WireDecl.Field de systemspec — el string se
//     guarda tal cual, sin parsear el prefijo "config:" acá). Esta
//     forma existe para no tener que RE-DECLARAR a mano una ruta que el
//     plugin ya conoce de sobra: el plugin es quien declaró ese campo en
//     su propio config_schema, este archivo solo pide acceso a él.
//
// AllowPlugins es la lista de plugins autorizados a recibirlo
// inyectado — vacía significa "ninguno todavía", nunca "todos":
// deny-by-default, igual que el resto de Asterion (ver
// Contract.permissions).
type SecretDecl struct {
	Name         string
	Source       string
	From         string // nombre de variable de un Import(...) — vacío si se usó Source
	FromPlugin   string // nombre del plugin dentro de ese import — vacío si se usó Source
	Field        string // ej. "config:database_password" — vacío si se usó Source
	AllowPlugins []string
}

// ImportDecl es Import(path=...) — trae los nombres de plugin que OTRO
// archivo .asterion declaró con System.plugin(...), para que
// AGCA.secret(from=<var>.<plugin>, ...) (y, a futuro, otros verbos)
// pueda referenciarlos. No copia el wiring (System.wire(...)) del
// archivo importado — Import es sobre QUÉ PLUGINS EXISTEN, no sobre
// cómo se conectan entre sí (eso lo resuelve, de nuevo, el propio
// archivo importado vía `asterion plugin system apply` sobre SÍ MISMO).
//
// PluginRoutes guarda el Route tal cual lo declaró cada
// System.plugin(...) (carpeta local o URL de git, sin resolver acá —
// mismo criterio que systemspec.PluginDecl.Route) para que un consumidor
// (ej. asterion-graph-cognitive-architecture, derivando capabilities
// reales de un plugin.yaml) pueda ubicar el plugin sin tener que volver
// a compilar el archivo importado él mismo. ResolvedDir es el
// directorio DEL ARCHIVO IMPORTADO (no del que declara el Import) —
// contra él, no contra el baseDir del archivo AGCA, se resuelve una
// PluginRoutes[...] relativa (mismo criterio que resolveOrInstall en
// asterion-core: route es siempre relativo a dónde vive SU PROPIO
// System.plugin(...), sin importar desde dónde se lo importe).
type ImportDecl struct {
	VarName      string
	Path         string
	ResolvedDir  string
	PluginNames  []string
	PluginRoutes map[string]string
}

// Spec es el resultado completo de compilar un archivo AGCA — todas las
// declaraciones en el orden en que aparecieron en el archivo (el orden
// importa: es también el orden de inicialización correcto, mismo
// criterio que systemspec.PluginDecl).
type Spec struct {
	Intelligences []IntelligenceDecl
	Graphs        []GraphDecl
	Neurons       []NeuronDecl
	Swarms        []SwarmDecl
	Agents        []AgentDecl
	Capabilities  []CapabilityRequirement
	Memories      []MemoryDecl
	Policies      []PolicyDecl
	Bots          []BotDecl
	Secrets       []SecretDecl
	Imports       []ImportDecl
	Tools         []ToolDecl
	ToolCaps      []CapabilityContractDecl
	Roles         []RoleDecl
}

// Compile recorre prog.Statements y arma la descripción de la
// inteligencia. El Bag devuelto nunca es nil y nunca corta en el primer
// error — mismo criterio que systemspec.Compile/pluginmanifest.Compile.
// baseDir es el directorio del propio archivo que se está compilando —
// contra él se resuelve un `Import(path=...)` relativo (mismo criterio
// que resolveOrInstall en asterion-core/cmd/asterion/plugin_system.go
// para System.plugin(route=...)); "" cuando no hay un archivo real
// detrás (ej. tests con fuente en memoria) — un Import ahí falla con
// ASTR611 en vez de resolver contra un cwd arbitrario.
//
// Ningún valor secreto de verdad puede pasar por acá: AGCA.secret(...)
// solo acepta Source (una referencia, ej. "store/prod/db_password" — el
// PATH dentro del gestor de secretos, nunca el secreto en sí) o
// From/FromPlugin/Field (derivarlo de un plugin ya importado — ver
// SecretDecl) — el valor real se sigue incorporando por el canal seguro
// de siempre (`asterion secret set`), nunca por un argumento de este DSL.
func Compile(prog *ast.Program, baseDir string) (*Spec, *diagnostics.Bag) {
	c := &compiler{
		spec:        &Spec{},
		diags:       &diagnostics.Bag{},
		kinds:       map[string]Kind{},
		importByVar: map[string]ImportDecl{},
		toolByVar:   map[string]string{},
		baseDir:     baseDir,
	}
	c.walkStmts(prog.Statements)
	return c.spec, c.diags
}

type compiler struct {
	spec        *Spec
	diags       *diagnostics.Bag
	kinds       map[string]Kind       // nombre de variable declarado -> qué verbo lo declaró
	importByVar map[string]ImportDecl // nombre de variable de un Import(...) ya resuelto -> su ImportDecl
	toolByVar   map[string]string     // nombre de variable de un Tool.define(...) -> su Name declarado
	baseDir     string
}

func (c *compiler) walkStmts(stmts []ast.Stmt) {
	for _, s := range stmts {
		c.walkStmt(s)
	}
}

func (c *compiler) walkStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.FuncDecl:
		c.walkStmts(s.Body)

	case *ast.AssignStmt:
		callExpr, ok := s.Value.(*ast.CallExpr)
		if !ok {
			return
		}
		if isImportCall(callExpr.Callee) {
			c.compileImport(s.Name, callExpr)
			return
		}
		if verb, ok := namespacedVerb(callExpr.Callee, "Tool"); ok {
			c.compileTool(s.Name, verb, callExpr)
			return
		}
		verb, ok := agcaVerb(callExpr.Callee)
		if !ok {
			return
		}
		c.compileAssigned(s.Name, verb, callExpr)

	case *ast.ExprStmt:
		callExpr, ok := s.X.(*ast.CallExpr)
		if !ok {
			return
		}
		verb, ok := agcaVerb(callExpr.Callee)
		if !ok || verb != "requires_capability" {
			return
		}
		c.compileCapabilityRequirement(callExpr)
	}
}

// agcaVerb reconoce la forma AGCA.<verbo>(...) en el callee de un
// CallExpr.
func agcaVerb(callee ast.Expr) (string, bool) { return namespacedVerb(callee, "AGCA") }

// namespacedVerb reconoce la forma <Namespace>.<verbo>(...) en el callee
// de un CallExpr — AGCA.* y Tool.* comparten este paquete (una Tool solo
// tiene sentido como algo que una Intelligence puede invocar) pero son
// namespaces distintos a propósito: Tool.* describe QUÉ SABE HACER una
// herramienta, AGCA.* describe QUIÉN la usa y bajo qué políticas.
func namespacedVerb(callee ast.Expr, namespace string) (string, bool) {
	attr, ok := callee.(*ast.AttrExpr)
	if !ok {
		return "", false
	}
	root, ok := attr.X.(*ast.Ident)
	if !ok || root.Name != namespace {
		return "", false
	}
	return attr.Name, true
}

// nonMutatingEffects/mutatingEffects son el vocabulario CERRADO de
// efectos que este compilador entiende y puede contradecir entre sí —
// cualquier otro effect (ej. "creates:Payment",
// "modifies:World.Infrastructure", "financial_operation") pasa como tag
// libre, sin validación: el contrato de una Tool puede describir más de
// lo que este compilador sabe juzgar, pero NUNCA puede declararse
// read_only y destructiva a la vez (ASTR622).
var nonMutatingEffects = map[string]bool{"read_only": true, "pure": true}
var mutatingEffects = map[string]bool{"mutating": true, "external_write": true, "destructive": true}

// compileTool resuelve `name = Tool.define(...)` y
// `name = Tool.capability(...)`.
func (c *compiler) compileTool(name, verb string, callExpr *ast.CallExpr) {
	if existing, redeclared := c.kinds[name]; redeclared {
		c.diags.Errorf(callExpr.Pos, "ASTR600",
			"%q ya fue declarado antes en este archivo (como %s) — los nombres deben ser únicos", name, existing)
		return
	}

	cc := &call{verb: verb, args: callExpr.Args, pos: callExpr.Pos, diags: c.diags, kinds: c.kinds, importByVar: c.importByVar}

	switch verb {
	case "define":
		dispName, _ := cc.str("name", true)
		c.spec.Tools = append(c.spec.Tools, ToolDecl{
			VarName:   name,
			Name:      dispName,
			Category:  mustStr(cc, "category"),
			Isolation: mustStr(cc, "isolation"),
		})
		c.toolByVar[name] = dispName
		c.declare(name, KindTool)

	case "capability":
		toolVar, _ := cc.ref("tool", true, KindTool)
		capName, _ := cc.str("name", true)
		effects := cc.stringList("effects")
		c.checkEffectConflict(callExpr, capName, effects)

		id := ""
		if toolVar != "" && capName != "" {
			id = strings.ToLower(c.toolByVar[toolVar]) + "." + capName
		}
		c.spec.ToolCaps = append(c.spec.ToolCaps, CapabilityContractDecl{
			VarName:     name,
			Tool:        toolVar,
			ID:          id,
			Name:        capName,
			Description: mustStr(cc, "description"),
			Input:       cc.stringList("input"),
			Output:      cc.stringList("output"),
			Effects:     effects,
			Requires:    cc.stringList("requires"),
			Guarantees:  cc.stringList("guarantees"),
		})
		c.declare(name, KindCapability)

	default:
		c.diags.Errorf(callExpr.Pos, "ASTR621",
			"Tool.%s no existe — verbos válidos: define, capability", verb)
	}
}

// checkEffectConflict rechaza una capability que se declare a la vez
// no-mutante (read_only/pure) y mutante (mutating/external_write/
// destructive): el runtime consulta effects ANTES de ejecutar para
// decidir si una política la permite, así que un contrato que se
// contradice a sí mismo haría esa decisión indefinida.
func (c *compiler) checkEffectConflict(callExpr *ast.CallExpr, capName string, effects []string) {
	var nonMutating, mutating string
	for _, e := range effects {
		if nonMutatingEffects[e] && nonMutating == "" {
			nonMutating = e
		}
		if mutatingEffects[e] && mutating == "" {
			mutating = e
		}
	}
	if nonMutating != "" && mutating != "" {
		c.diags.Errorf(callExpr.Pos, "ASTR622",
			"Tool.capability %q declara %q y %q a la vez — una capability no puede ser no-mutante y mutante al mismo tiempo",
			capName, nonMutating, mutating)
	}
}

// mustStr lee un string opcional descartando el bool de presencia — solo
// para campos donde "" y "no declarado" significan lo mismo.
func mustStr(cc *call, key string) string {
	v, _ := cc.str(key, false)
	return v
}

// isImportCall reconoce Import(...) — a diferencia de AGCA.<verbo>, es
// un builtin de llamada DESNUDA (mismo estilo que Network(cidr=...) en
// semantic/analyzer.go: genericResourceTypes), no un verbo bajo un
// namespace — Import no pertenece más a AGCA que a System, es un
// mecanismo del lenguaje para conectar CUALQUIER par de archivos
// .asterion, del que este DSL es hoy el único consumidor.
func isImportCall(callee ast.Expr) bool {
	ident, ok := callee.(*ast.Ident)
	return ok && ident.Name == "Import"
}

// compileImport resuelve `name = Import(path="...")`: lee, parsea y
// compila el archivo referenciado con systemspec.Compile (Import solo
// trae QUÉ PLUGINS declaró — ver doc comment de ImportDecl), y registra
// sus nombres para que AGCA.secret(from=<name>.<plugin>, ...) (ver
// call.pluginRef) pueda validarlos.
func (c *compiler) compileImport(name string, callExpr *ast.CallExpr) {
	if existing, redeclared := c.kinds[name]; redeclared {
		c.diags.Errorf(callExpr.Pos, "ASTR600",
			"%q ya fue declarado antes en este archivo (como %s) — los nombres deben ser únicos", name, existing)
		return
	}

	cc := &call{verb: "Import", args: callExpr.Args, pos: callExpr.Pos, diags: c.diags, kinds: c.kinds, importByVar: c.importByVar}
	path, ok := cc.str("path", true)
	if !ok {
		return
	}

	resolvedPath, plugins, err := c.loadImportedPlugins(path)
	if err != nil {
		c.diags.Errorf(callExpr.Pos, "ASTR611", "Import(path=%q): %s", path, err)
		return
	}

	names := make([]string, len(plugins))
	routes := make(map[string]string, len(plugins))
	for i, p := range plugins {
		names[i] = p.Name
		routes[p.Name] = p.Route
	}

	decl := ImportDecl{VarName: name, Path: path, ResolvedDir: filepath.Dir(resolvedPath), PluginNames: names, PluginRoutes: routes}
	c.spec.Imports = append(c.spec.Imports, decl)
	c.importByVar[name] = decl
	c.declare(name, KindImport)
}

// loadImportedPlugins lee+parsea+compila (como sistema de plugins, vía
// systemspec — ver el doc comment del paquete sobre por qué solo esto y
// no también AGCA.*) el archivo referenciado por un Import(...),
// resolviendo path relativo contra c.baseDir (el directorio del propio
// archivo que se está compilando, nunca el cwd de quien corre el
// comando — mismo criterio que resolveOrInstall en asterion-core para
// System.plugin(route=...)). Devuelve los PluginDecl completos (no solo
// nombres) para que ImportDecl pueda guardar también su Route — ver
// PluginRoutes.
func (c *compiler) loadImportedPlugins(path string) (resolvedPath string, plugins []systemspec.PluginDecl, err error) {
	resolved := path
	if c.baseDir != "" && !filepath.IsAbs(path) {
		resolved = filepath.Join(c.baseDir, path)
	}

	src, err := os.ReadFile(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("no pude leer %s: %w", resolved, err)
	}

	prog, parseDiags := langparser.Parse(src, resolved)
	if parseDiags.HasErrors() {
		return "", nil, fmt.Errorf("%s no compila:\n%s", resolved, parseDiags.String())
	}

	plugins, _, compileDiags := systemspec.Compile(prog)
	if compileDiags.HasErrors() {
		return "", nil, fmt.Errorf("%s no se pudo compilar como sistema de plugins:\n%s", resolved, compileDiags.String())
	}
	return resolved, plugins, nil
}

func (c *compiler) compileAssigned(name, verb string, callExpr *ast.CallExpr) {
	if existing, redeclared := c.kinds[name]; redeclared {
		c.diags.Errorf(callExpr.Pos, "ASTR600",
			"%q ya fue declarado antes en este archivo (como AGCA.%s) — los nombres deben ser únicos", name, existing)
		return
	}

	cc := &call{verb: verb, args: callExpr.Args, pos: callExpr.Pos, diags: c.diags, kinds: c.kinds, importByVar: c.importByVar}

	switch verb {
	case "intelligence":
		dispName, _ := cc.str("name", true)
		c.spec.Intelligences = append(c.spec.Intelligences, IntelligenceDecl{VarName: name, Name: dispName})
		c.declare(name, KindIntelligence)

	case "graph":
		intelligence, _ := cc.ref("intelligence", true, KindIntelligence)
		dispName, hasName := cc.str("name", false)
		if !hasName {
			dispName = name
		}
		c.spec.Graphs = append(c.spec.Graphs, GraphDecl{
			VarName: name, Name: dispName, Intelligence: intelligence,
			Hierarchical: cc.boolVal("hierarchical", false),
			Persistent:   cc.boolVal("persistent", false),
			Temporal:     cc.boolVal("temporal", false),
			Provenance:   cc.boolVal("provenance", false),
		})
		c.declare(name, KindGraph)

	case "neuron":
		intelligence, _ := cc.ref("intelligence", true, KindIntelligence)
		dispName, hasName := cc.str("name", false)
		if !hasName {
			dispName = name
		}
		runtime, _ := cc.str("runtime", false)
		adapter, _ := cc.str("adapter", false)
		if runtime != "" && adapter != "" {
			c.diags.Errorf(callExpr.Pos, "ASTR606",
				"AGCA.neuron: 'runtime' (%q) y 'adapter' (%q) son mutuamente excluyentes — una neurona es local (runtime) o remota (adapter), no las dos", runtime, adapter)
		}
		model, _ := cc.str("model", false)
		capabilities := cc.stringList("capabilities")
		privacy := cc.enumStr("privacy", []string{"local", "remote"}, "local")
		c.spec.Neurons = append(c.spec.Neurons, NeuronDecl{
			VarName: name, Name: dispName, Intelligence: intelligence,
			Runtime: runtime, Adapter: adapter, Model: model,
			Capabilities: capabilities, Privacy: privacy,
		})
		c.declare(name, KindNeuron)

	case "swarm":
		intelligence, _ := cc.ref("intelligence", true, KindIntelligence)
		dispName, hasName := cc.str("name", false)
		if !hasName {
			dispName = name
		}
		instances := cc.instances()
		c.spec.Swarms = append(c.spec.Swarms, SwarmDecl{VarName: name, Name: dispName, Intelligence: intelligence, Instances: instances})
		c.declare(name, KindSwarm)

	case "agent":
		intelligence, _ := cc.ref("intelligence", true, KindIntelligence)
		dispName, hasName := cc.str("name", false)
		if !hasName {
			dispName = name
		}
		strategy, _ := cc.str("strategy", false)
		role, _ := cc.ref("role", false, KindRole)
		c.spec.Agents = append(c.spec.Agents, AgentDecl{
			VarName: name, Name: dispName, Intelligence: intelligence, Strategy: strategy,
			Allow: cc.stringList("allow"), Deny: cc.stringList("deny"), Role: role,
		})
		c.declare(name, KindAgent)

	case "memory":
		intelligence, _ := cc.ref("intelligence", true, KindIntelligence)
		dispName, hasName := cc.str("name", false)
		if !hasName {
			dispName = name
		}
		memType, _ := cc.str("type", false)
		graph, _ := cc.ref("graph", true, KindGraph)
		c.spec.Memories = append(c.spec.Memories, MemoryDecl{VarName: name, Name: dispName, Intelligence: intelligence, Type: memType, Graph: graph})
		c.declare(name, KindMemory)

	case "policy":
		intelligence, _ := cc.ref("intelligence", true, KindIntelligence)
		dispName, hasName := cc.str("name", false)
		if !hasName {
			dispName = name
		}
		when, _ := cc.str("when", false)
		allow, _ := cc.str("allow", false)
		deny, _ := cc.str("deny", false)
		prefer, _ := cc.str("prefer", false)
		unless, _ := cc.str("unless", false)
		c.spec.Policies = append(c.spec.Policies, PolicyDecl{
			VarName: name, Name: dispName, Intelligence: intelligence,
			When: when, Allow: allow, Deny: deny, Prefer: prefer, Unless: unless,
		})
		c.declare(name, KindPolicy)

	case "bot":
		intelligence, _ := cc.ref("intelligence", true, KindIntelligence)
		dispName, hasName := cc.str("name", false)
		if !hasName {
			dispName = name
		}
		iface, _ := cc.str("interface", true)
		permissions := cc.stringList("permissions")
		c.spec.Bots = append(c.spec.Bots, BotDecl{VarName: name, Name: dispName, Intelligence: intelligence, Interface: iface, Permissions: permissions})
		c.declare(name, KindBot)

	case "role":
		intelligence, _ := cc.ref("intelligence", true, KindIntelligence)
		dispName, hasName := cc.str("name", false)
		if !hasName {
			dispName = name
		}
		inherits := cc.refList("inherits", KindRole)
		c.spec.Roles = append(c.spec.Roles, RoleDecl{
			VarName: name, Name: dispName, Intelligence: intelligence,
			Description: mustStr(cc, "description"),
			Allow:       cc.stringList("allow"),
			Deny:        cc.stringList("deny"),
			Inherits:    inherits,
			Users:       cc.stringList("users"),
		})
		c.declare(name, KindRole)

	case "secret":
		dispName, _ := cc.str("name", true)
		source, hasSource := cc.str("source", false)
		_, fromPresent := cc.find("from") // presencia del ARGUMENTO, distinto de si pluginRef lo resolvió bien
		fromRef, fromOK := cc.pluginRef("from", false)
		field, hasField := cc.str("field", false)
		allowPlugins := cc.stringList("allow")

		switch {
		case hasSource && (fromPresent || hasField):
			c.diags.Errorf(callExpr.Pos, "ASTR609",
				"AGCA.secret: declarar 'source' (un valor literal) O 'from'+'field' (derivarlo de un plugin importado), no los dos")
		case !hasSource && !fromPresent:
			c.diags.Errorf(callExpr.Pos, "ASTR609",
				"AGCA.secret: falta 'source' (un valor literal) o 'from'+'field' (derivarlo de un plugin ya declarado con Import(...))")
		case fromPresent && fromOK && !hasField:
			c.diags.Errorf(callExpr.Pos, "ASTR601", "AGCA.secret: 'from' requiere también 'field' (ej. field=\"config:database_password\")")
		}

		c.spec.Secrets = append(c.spec.Secrets, SecretDecl{
			Name: dispName, Source: source,
			From: fromRef.ImportVar, FromPlugin: fromRef.Plugin, Field: field,
			AllowPlugins: allowPlugins,
		})
		c.declare(name, KindSecret)

	default:
		c.diags.Errorf(callExpr.Pos, "ASTR605", "AGCA.%s no existe — verbos válidos: intelligence, graph, neuron, swarm, agent, role, requires_capability, memory, policy, bot, secret", verb)
	}
}

func (c *compiler) compileCapabilityRequirement(callExpr *ast.CallExpr) {
	cc := &call{verb: "requires_capability", args: callExpr.Args, pos: callExpr.Pos, diags: c.diags, kinds: c.kinds, importByVar: c.importByVar}
	intelligence, okI := cc.ref("intelligence", true, KindIntelligence)
	capability, okC := cc.str("capability", true)
	if !okI || !okC {
		return
	}
	c.spec.Capabilities = append(c.spec.Capabilities, CapabilityRequirement{Intelligence: intelligence, Capability: capability})
}

func (c *compiler) declare(name string, kind Kind) { c.kinds[name] = kind }

// call agrupa lo que un handler de verbo necesita para leer sus
// argumentos con mensajes de error consistentes — mismo patrón que
// systemspec.call/pluginmanifest.call, con el agregado de enumStr()
// (validar contra un conjunto cerrado de valores) e instances() (entero
// positivo o "adaptive").
type call struct {
	verb        string
	args        []ast.Arg
	pos         diagnostics.Position
	diags       *diagnostics.Bag
	kinds       map[string]Kind
	importByVar map[string]ImportDecl
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
			c.diags.Errorf(c.pos, "ASTR601", "AGCA.%s: falta el argumento obligatorio %q", c.verb, key)
		}
		return "", false
	}
	lit, ok := v.(*ast.StringLit)
	if !ok {
		c.diags.Errorf(v.Position(), "ASTR602", "AGCA.%s: %q debe ser un string, no %s", c.verb, key, exprKind(v))
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
		c.diags.Errorf(v.Position(), "ASTR602", "AGCA.%s: %q debe ser true/false, no %s", c.verb, key, exprKind(v))
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
		c.diags.Errorf(v.Position(), "ASTR602", "AGCA.%s: %q debe ser una lista, no %s", c.verb, key, exprKind(v))
		return nil
	}
	out := make([]string, 0, len(list.Elements))
	for _, el := range list.Elements {
		lit, ok := el.(*ast.StringLit)
		if !ok {
			c.diags.Errorf(el.Position(), "ASTR602", "AGCA.%s: los elementos de %q deben ser strings, no %s", c.verb, key, exprKind(el))
			continue
		}
		out = append(out, lit.Value)
	}
	return out
}

// enumStr lee un string y lo valida contra un conjunto cerrado de valores
// válidos (ej. privacy: "local"|"remote") — a diferencia de str(), un
// valor fuera del conjunto es un error (ASTR607), no "lo que sea".
func (c *call) enumStr(key string, valid []string, def string) string {
	v, ok := c.find(key)
	if !ok {
		return def
	}
	lit, ok := v.(*ast.StringLit)
	if !ok {
		c.diags.Errorf(v.Position(), "ASTR602", "AGCA.%s: %q debe ser un string, no %s", c.verb, key, exprKind(v))
		return def
	}
	for _, allowed := range valid {
		if lit.Value == allowed {
			return lit.Value
		}
	}
	c.diags.Errorf(v.Position(), "ASTR607", "AGCA.%s: %q %q no reconocido — válidos: %s", c.verb, key, lit.Value, joinQuoted(valid))
	return def
}

// instances lee `instances=` — "adaptive" (StringLit) o un entero
// positivo (IntLit, > 0). Cualquier otra cosa (incluido un entero <= 0)
// es ASTR606: un swarm de 0 o -3 instancias no tiene sentido operativo.
func (c *call) instances() string {
	v, ok := c.find("instances")
	if !ok {
		c.diags.Errorf(c.pos, "ASTR601", "AGCA.%s: falta el argumento obligatorio %q", c.verb, "instances")
		return ""
	}
	switch lit := v.(type) {
	case *ast.StringLit:
		if lit.Value != "adaptive" {
			c.diags.Errorf(v.Position(), "ASTR606", "AGCA.%s: %q como string solo acepta \"adaptive\", no %q", c.verb, "instances", lit.Value)
			return ""
		}
		return lit.Value
	case *ast.IntLit:
		if lit.Value <= 0 {
			c.diags.Errorf(v.Position(), "ASTR606", "AGCA.%s: %q debe ser un entero positivo (recibí %d)", c.verb, "instances", lit.Value)
			return ""
		}
		return strconv.FormatInt(lit.Value, 10)
	default:
		c.diags.Errorf(v.Position(), "ASTR602", "AGCA.%s: %q debe ser un entero o \"adaptive\", no %s", c.verb, "instances", exprKind(v))
		return ""
	}
}

// ref lee un argumento que debe ser una referencia (Ident) a un nombre ya
// declarado por el verbo AGCA.<wantKind> correspondiente — a diferencia
// de systemspec.call.ref (que solo pedía "declarado con System.plugin",
// un único kind posible), acá hay varios kinds (intelligence/graph/...) y
// referenciar el correcto importa: pasar un GraphDecl donde se espera una
// Intelligence es un error real, no solo un nombre mal escrito.
func (c *call) ref(key string, required bool, wantKind Kind) (string, bool) {
	v, ok := c.find(key)
	if !ok {
		if required {
			c.diags.Errorf(c.pos, "ASTR601", "AGCA.%s: falta el argumento obligatorio %q", c.verb, key)
		}
		return "", false
	}
	ident, ok := v.(*ast.Ident)
	if !ok {
		c.diags.Errorf(v.Position(), "ASTR603",
			"AGCA.%s: %q debe ser una referencia a un AGCA.%s(...) declarado más arriba, no %s",
			c.verb, key, wantKind, exprKind(v))
		return "", false
	}
	gotKind, declared := c.kinds[ident.Name]
	if !declared {
		c.diags.Errorf(v.Position(), "ASTR604",
			"AGCA.%s: %q referencia a %q, que no fue declarado antes de esta línea", c.verb, key, ident.Name)
		return "", false
	}
	if gotKind != wantKind {
		c.diags.Errorf(v.Position(), "ASTR608",
			"AGCA.%s: %q referencia a %q, que es un AGCA.%s(...), no un AGCA.%s(...)", c.verb, key, ident.Name, gotKind, wantKind)
		return "", false
	}
	return ident.Name, true
}

// PluginRef es una referencia de la forma `<import>.<plugin>` (ej.
// `sys.db`) — ver call.pluginRef.
type PluginRef struct {
	ImportVar string
	Plugin    string
}

// pluginRef lee un argumento que debe ser una referencia de la forma
// `<import>.<plugin>` — un AttrExpr cuya raíz es un Import(...) ya
// declarado (ASTR612 si no tiene esa forma, ASTR604 si el import no fue
// declarado, ASTR608 si el nombre existe pero no es un Import, ASTR613
// si el import existe pero no declaró ESE plugin).
func (c *call) pluginRef(key string, required bool) (PluginRef, bool) {
	v, ok := c.find(key)
	if !ok {
		if required {
			c.diags.Errorf(c.pos, "ASTR601", "AGCA.%s: falta el argumento obligatorio %q", c.verb, key)
		}
		return PluginRef{}, false
	}
	attr, ok := v.(*ast.AttrExpr)
	if !ok {
		c.diags.Errorf(v.Position(), "ASTR612",
			"AGCA.%s: %q debe ser una referencia de la forma <import>.<plugin> (ej. sys.db), no %s", c.verb, key, exprKind(v))
		return PluginRef{}, false
	}
	root, ok := attr.X.(*ast.Ident)
	if !ok {
		c.diags.Errorf(v.Position(), "ASTR612",
			"AGCA.%s: %q debe ser una referencia de la forma <import>.<plugin> (ej. sys.db)", c.verb, key)
		return PluginRef{}, false
	}
	gotKind, declared := c.kinds[root.Name]
	if !declared {
		c.diags.Errorf(v.Position(), "ASTR604",
			"AGCA.%s: %q referencia a %q, que no fue declarado antes de esta línea", c.verb, key, root.Name)
		return PluginRef{}, false
	}
	if gotKind != KindImport {
		c.diags.Errorf(v.Position(), "ASTR608",
			"AGCA.%s: %q referencia a %q, que es un AGCA.%s(...), no un Import(...)", c.verb, key, root.Name, gotKind)
		return PluginRef{}, false
	}
	imp := c.importByVar[root.Name]
	if !containsString(imp.PluginNames, attr.Name) {
		c.diags.Errorf(v.Position(), "ASTR613",
			"AGCA.%s: %q no declaró ningún plugin %q — declarados: %v", c.verb, root.Name, attr.Name, imp.PluginNames)
		return PluginRef{}, false
	}
	return PluginRef{ImportVar: root.Name, Plugin: attr.Name}, true
}

// refList lee una lista de REFERENCIAS (Idents) a nombres ya declarados
// con el verbo wantKind — ej. inherits=[viewer, analyst]. A diferencia
// de stringList, cada elemento se valida contra lo realmente declarado:
// heredar de un nombre que no es un rol es un error, no un string suelto
// que nadie mira.
func (c *call) refList(key string, wantKind Kind) []string {
	v, ok := c.find(key)
	if !ok {
		return nil
	}
	list, ok := v.(*ast.ListLit)
	if !ok {
		c.diags.Errorf(v.Position(), "ASTR602", "AGCA.%s: %q debe ser una lista de referencias, no %s", c.verb, key, exprKind(v))
		return nil
	}
	out := make([]string, 0, len(list.Elements))
	for _, el := range list.Elements {
		ident, ok := el.(*ast.Ident)
		if !ok {
			c.diags.Errorf(el.Position(), "ASTR603",
				"AGCA.%s: los elementos de %q deben ser referencias a un AGCA.%s(...) declarado más arriba, no %s",
				c.verb, key, wantKind, exprKind(el))
			continue
		}
		gotKind, declared := c.kinds[ident.Name]
		if !declared {
			c.diags.Errorf(el.Position(), "ASTR604",
				"AGCA.%s: %q referencia a %q, que no fue declarado antes de esta línea", c.verb, key, ident.Name)
			continue
		}
		if gotKind != wantKind {
			c.diags.Errorf(el.Position(), "ASTR608",
				"AGCA.%s: %q referencia a %q, que es un AGCA.%s(...), no un AGCA.%s(...)", c.verb, key, ident.Name, gotKind, wantKind)
			continue
		}
		out = append(out, ident.Name)
	}
	return out
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
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

func joinQuoted(vals []string) string {
	out := ""
	for i, v := range vals {
		if i > 0 {
			out += ", "
		}
		out += "\"" + v + "\""
	}
	return out
}
