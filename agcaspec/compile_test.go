package agcaspec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tarafagat/asterion-language/ast"
	"github.com/Tarafagat/asterion-language/diagnostics"
	"github.com/Tarafagat/asterion-language/parser"
)

const happyPath = `
language "0.1"

brain = AGCA.intelligence(name="CompanyBrain")

world = AGCA.graph(intelligence=brain, name="CompanyWorld", hierarchical=true, persistent=true, temporal=true, provenance=true)

local_fast = AGCA.neuron(
    intelligence=brain,
    name="LocalFast",
    runtime="gguf",
    model="./models/local-fast.gguf",
    capabilities=["classification", "extraction"],
    privacy="local",
)

remote_deep = AGCA.neuron(
    intelligence=brain,
    name="RemoteDeepReasoner",
    adapter="remote-llm",
    model="env:DEEP_MODEL",
    capabilities=["reasoning", "planning", "critic"],
    privacy="remote",
)

operations = AGCA.swarm(intelligence=brain, name="Operations", instances="adaptive")
finance = AGCA.swarm(intelligence=brain, name="Finance", instances=8)

executive = AGCA.agent(intelligence=brain, name="Executive", strategy="adaptive")

AGCA.requires_capability(intelligence=brain, capability="database.query")
AGCA.requires_capability(intelligence=brain, capability="inventory.read")

memory = AGCA.memory(intelligence=brain, name="LongTerm", type="hybrid", graph=world)

policy = AGCA.policy(
    intelligence=brain,
    name="ConfidentialRouting",
    when="data.sensitivity>=confidential",
    allow="privacy==local",
    deny="privacy==remote",
)

admin_bot = AGCA.bot(intelligence=brain, name="AdminAssistant", interface="terminal", permissions=["inventory.read"])

db_password = AGCA.secret(name="DatabasePassword", source="mycompany/prod/database_password", allow=["asterion-mercadopago"])
`

func parseOK(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, diags := parser.Parse([]byte(src), "test.asterion")
	if diags.HasErrors() {
		t.Fatalf("el .asterion de prueba no parsea (bug del test, no del compilador):\n%s", diags.String())
	}
	return prog
}

func codes(diags *diagnostics.Bag) []string {
	out := make([]string, 0, len(diags.Items()))
	for _, d := range diags.Items() {
		out = append(out, d.Code)
	}
	return out
}

func TestCompile_HappyPath(t *testing.T) {
	prog := parseOK(t, happyPath)
	spec, diags := Compile(prog, "")
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores, tuve:\n%s", diags.String())
	}

	if len(spec.Intelligences) != 1 || spec.Intelligences[0].Name != "CompanyBrain" {
		t.Fatalf("Intelligences = %+v", spec.Intelligences)
	}
	if len(spec.Graphs) != 1 || !spec.Graphs[0].Hierarchical || !spec.Graphs[0].Persistent || !spec.Graphs[0].Temporal || !spec.Graphs[0].Provenance {
		t.Fatalf("Graphs = %+v", spec.Graphs)
	}
	if spec.Graphs[0].Intelligence != "brain" {
		t.Errorf("Graphs[0].Intelligence = %q, want \"brain\"", spec.Graphs[0].Intelligence)
	}

	if len(spec.Neurons) != 2 {
		t.Fatalf("Neurons = %d, want 2", len(spec.Neurons))
	}
	n0 := spec.Neurons[0]
	if n0.Runtime != "gguf" || n0.Adapter != "" || n0.Privacy != "local" || len(n0.Capabilities) != 2 {
		t.Errorf("Neurons[0] = %+v", n0)
	}
	n1 := spec.Neurons[1]
	if n1.Adapter != "remote-llm" || n1.Runtime != "" || n1.Privacy != "remote" || n1.Model != "env:DEEP_MODEL" {
		t.Errorf("Neurons[1] = %+v", n1)
	}

	if len(spec.Swarms) != 2 {
		t.Fatalf("Swarms = %d, want 2", len(spec.Swarms))
	}
	if spec.Swarms[0].Instances != "adaptive" {
		t.Errorf("Swarms[0].Instances = %q, want \"adaptive\"", spec.Swarms[0].Instances)
	}
	if spec.Swarms[1].Instances != "8" {
		t.Errorf("Swarms[1].Instances = %q, want \"8\"", spec.Swarms[1].Instances)
	}

	if len(spec.Agents) != 1 || spec.Agents[0].Strategy != "adaptive" {
		t.Errorf("Agents = %+v", spec.Agents)
	}

	if len(spec.Capabilities) != 2 {
		t.Fatalf("Capabilities = %d, want 2", len(spec.Capabilities))
	}
	if spec.Capabilities[0].Capability != "database.query" || spec.Capabilities[0].Intelligence != "brain" {
		t.Errorf("Capabilities[0] = %+v", spec.Capabilities[0])
	}

	if len(spec.Memories) != 1 || spec.Memories[0].Graph != "world" || spec.Memories[0].Type != "hybrid" {
		t.Errorf("Memories = %+v", spec.Memories)
	}

	if len(spec.Policies) != 1 || spec.Policies[0].When == "" || spec.Policies[0].Allow == "" || spec.Policies[0].Deny == "" {
		t.Errorf("Policies = %+v", spec.Policies)
	}

	if len(spec.Bots) != 1 {
		t.Fatalf("Bots = %d, want 1", len(spec.Bots))
	}
	bot := spec.Bots[0]
	if bot.Name != "AdminAssistant" || bot.Intelligence != "brain" || bot.Interface != "terminal" || len(bot.Permissions) != 1 {
		t.Errorf("Bots[0] = %+v", bot)
	}

	if len(spec.Secrets) != 1 {
		t.Fatalf("Secrets = %d, want 1", len(spec.Secrets))
	}
	secret := spec.Secrets[0]
	if secret.Name != "DatabasePassword" || secret.Source != "mycompany/prod/database_password" || len(secret.AllowPlugins) != 1 {
		t.Errorf("Secrets[0] = %+v", secret)
	}
}

func TestCompile_NeuronRuntimeAndAdapterMutuallyExclusive(t *testing.T) {
	prog := parseOK(t, `
brain = AGCA.intelligence(name="Brain")
bad = AGCA.neuron(intelligence=brain, name="Bad", runtime="gguf", adapter="remote-llm")
`)
	_, diags := Compile(prog, "")
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR606" {
		t.Errorf("codes = %v, want [ASTR606]", got)
	}
}

func TestCompile_InvalidPrivacy(t *testing.T) {
	prog := parseOK(t, `
brain = AGCA.intelligence(name="Brain")
n = AGCA.neuron(intelligence=brain, name="N", privacy="on-premise")
`)
	_, diags := Compile(prog, "")
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR607" {
		t.Errorf("codes = %v, want [ASTR607]", got)
	}
}

func TestCompile_SwarmInstancesZeroRejected(t *testing.T) {
	prog := parseOK(t, `
brain = AGCA.intelligence(name="Brain")
s = AGCA.swarm(intelligence=brain, name="S", instances=0)
`)
	_, diags := Compile(prog, "")
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR606" {
		t.Errorf("codes = %v, want [ASTR606]", got)
	}
}

func TestCompile_RefWrongKind(t *testing.T) {
	// world es un AGCA.graph(...), no un AGCA.intelligence(...) — pasarlo
	// como intelligence= tiene que rechazarse por el KIND, no aceptarse
	// solo porque "world" existe.
	prog := parseOK(t, `
brain = AGCA.intelligence(name="Brain")
world = AGCA.graph(intelligence=brain, name="World")
bad = AGCA.agent(intelligence=world, name="Bad")
`)
	_, diags := Compile(prog, "")
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR608" {
		t.Errorf("codes = %v, want [ASTR608]", got)
	}
}

func TestCompile_RefUndeclared(t *testing.T) {
	prog := parseOK(t, `agent = AGCA.agent(intelligence=nonexistent, name="X")`)
	_, diags := Compile(prog, "")
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR604" {
		t.Errorf("codes = %v, want [ASTR604]", got)
	}
}

func TestCompile_RefStringLiteralInsteadOfReference(t *testing.T) {
	prog := parseOK(t, `agent = AGCA.agent(intelligence="brain", name="X")`)
	_, diags := Compile(prog, "")
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR603" {
		t.Errorf("codes = %v, want [ASTR603]", got)
	}
}

func TestCompile_DuplicateName(t *testing.T) {
	prog := parseOK(t, `
brain = AGCA.intelligence(name="Brain")
brain = AGCA.intelligence(name="OtherBrain")
`)
	_, diags := Compile(prog, "")
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR600" {
		t.Errorf("codes = %v, want [ASTR600]", got)
	}
}

func TestCompile_MissingRequiredArg(t *testing.T) {
	prog := parseOK(t, `brain = AGCA.intelligence()`)
	_, diags := Compile(prog, "")
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR601" {
		t.Errorf("codes = %v, want [ASTR601]", got)
	}
}

func TestCompile_UnknownVerb(t *testing.T) {
	prog := parseOK(t, `x = AGCA.nonexistent(name="X")`)
	_, diags := Compile(prog, "")
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR605" {
		t.Errorf("codes = %v, want [ASTR605]", got)
	}
}

func TestCompile_IgnoresUnrelatedStatements(t *testing.T) {
	prog := parseOK(t, `
otro = SomeOtherThing(x=1)
brain = AGCA.intelligence(name="Brain")
`)
	spec, diags := Compile(prog, "")
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores:\n%s", diags.String())
	}
	if len(spec.Intelligences) != 1 {
		t.Errorf("Intelligences = %d, want 1", len(spec.Intelligences))
	}
}

// writeSystemFile escribe un archivo de sistema de plugins real
// (System.plugin(...)) a un directorio temporal — usado por los tests de
// Import(...) de abajo, que necesitan un archivo DE VERDAD en disco (a
// diferencia del resto de este paquete, que solo parsea fuente en
// memoria): Import lee y compila el archivo referenciado, no puede
// probarse sin uno real.
func writeSystemFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("no pude escribir %s: %v", path, err)
	}
	return path
}

const importedSystem = `
language "0.1"

db = System.plugin(route="./tutorial-db-plugin", principal=true)
api = System.plugin(route="./tutorial-web-plugin")
`

func TestCompile_ImportBringsInPluginNames(t *testing.T) {
	dir := t.TempDir()
	writeSystemFile(t, dir, "system.asterion", importedSystem)

	prog := parseOK(t, `sys = Import(path="./system.asterion")`)
	spec, diags := Compile(prog, dir)
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores:\n%s", diags.String())
	}
	if len(spec.Imports) != 1 {
		t.Fatalf("Imports = %d, want 1", len(spec.Imports))
	}
	imp := spec.Imports[0]
	if imp.VarName != "sys" || len(imp.PluginNames) != 2 || imp.PluginNames[0] != "db" || imp.PluginNames[1] != "api" {
		t.Errorf("Imports[0] = %+v", imp)
	}
}

func TestCompile_ImportCapturesPluginRoutesAndResolvedDir(t *testing.T) {
	dir := t.TempDir()
	writeSystemFile(t, dir, "system.asterion", importedSystem)

	prog := parseOK(t, `sys = Import(path="./system.asterion")`)
	spec, diags := Compile(prog, dir)
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores:\n%s", diags.String())
	}
	imp := spec.Imports[0]
	if imp.ResolvedDir != dir {
		t.Errorf("ResolvedDir = %q, want %q", imp.ResolvedDir, dir)
	}
	if imp.PluginRoutes["db"] != "./tutorial-db-plugin" || imp.PluginRoutes["api"] != "./tutorial-web-plugin" {
		t.Errorf("PluginRoutes = %+v", imp.PluginRoutes)
	}
}

func TestCompile_ImportMissingFile(t *testing.T) {
	dir := t.TempDir()
	prog := parseOK(t, `sys = Import(path="./nonexistent.asterion")`)
	_, diags := Compile(prog, dir)
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR611" {
		t.Errorf("codes = %v, want [ASTR611]", got)
	}
}

func TestCompile_SecretDerivedFromImportedPlugin(t *testing.T) {
	dir := t.TempDir()
	writeSystemFile(t, dir, "system.asterion", importedSystem)

	prog := parseOK(t, `
sys = Import(path="./system.asterion")
db_password = AGCA.secret(name="DatabasePassword", from=sys.db, field="config:database_password")
`)
	spec, diags := Compile(prog, dir)
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores:\n%s", diags.String())
	}
	if len(spec.Secrets) != 1 {
		t.Fatalf("Secrets = %d, want 1", len(spec.Secrets))
	}
	s := spec.Secrets[0]
	if s.Source != "" || s.From != "sys" || s.FromPlugin != "db" || s.Field != "config:database_password" {
		t.Errorf("Secrets[0] = %+v", s)
	}
}

func TestCompile_SecretFromReferencesUndeclaredPlugin(t *testing.T) {
	dir := t.TempDir()
	writeSystemFile(t, dir, "system.asterion", importedSystem)

	prog := parseOK(t, `
sys = Import(path="./system.asterion")
bad = AGCA.secret(name="X", from=sys.nonexistent, field="config:x")
`)
	_, diags := Compile(prog, dir)
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR613" {
		t.Errorf("codes = %v, want [ASTR613]", got)
	}
}

func TestCompile_SecretBothSourceAndFromRejected(t *testing.T) {
	dir := t.TempDir()
	writeSystemFile(t, dir, "system.asterion", importedSystem)

	prog := parseOK(t, `
sys = Import(path="./system.asterion")
bad = AGCA.secret(name="X", source="a/b/c", from=sys.db, field="config:x")
`)
	_, diags := Compile(prog, dir)
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR609" {
		t.Errorf("codes = %v, want [ASTR609]", got)
	}
}

func TestCompile_SecretNeitherSourceNorFromRejected(t *testing.T) {
	prog := parseOK(t, `bad = AGCA.secret(name="X")`)
	_, diags := Compile(prog, "")
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR609" {
		t.Errorf("codes = %v, want [ASTR609]", got)
	}
}

func TestCompile_SecretFromWrongKind(t *testing.T) {
	prog := parseOK(t, `
brain = AGCA.intelligence(name="Brain")
bad = AGCA.secret(name="X", from=brain.db, field="config:x")
`)
	_, diags := Compile(prog, "")
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR608" {
		t.Errorf("codes = %v, want [ASTR608]", got)
	}
}
