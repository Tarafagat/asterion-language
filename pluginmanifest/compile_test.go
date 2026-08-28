package pluginmanifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tarafagat/asterion-language/ast"
	"github.com/Tarafagat/asterion-language/diagnostics"
	"github.com/Tarafagat/asterion-language/parser"
	"github.com/Tarafagat/asterion-plugin-contract/apc"
)

const happyPath = `
language "0.1"

Contract.define(
    name="asterion-mail-plugin-basic",
    version="0.1.0",
    description="Envía emails personalizados por SMTP real.",
    author="Asterion",
    license="Apache-2.0",
)

Contract.language(name="go", version="1.25")
Contract.start(command="./asterion-mail-plugin-basic", port=0)
Contract.health_path(path="/health")
Contract.api(base_path="/api/v1", openapi="api/openapi.yaml")

Contract.permissions(
    network=["smtp"],
    filesystem=["./data"],
    secrets=true,
)

Contract.config(key="smtp_host", label="Servidor SMTP", type="string", required=true)
Contract.config(key="use_tls", label="TLS implícito", type="bool", default="false")

Contract.resource(
    name="templates",
    endpoint="/templates",
    schema="resources/schemas/template.json",
    primary_key="id",
    crud=["create", "read", "update", "delete", "list"],
)

Contract.action(name="send_email", method="POST", endpoint="/send", description="Envía un email real")
`

// parseOK parsea src y falla el test si el lexer/parser encuentran algún
// error — separa "el .ast no es válido Asterion Language" (bug del test)
// de "el compilador de manifiestos rechaza esta forma" (lo que se prueba).
func parseOK(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, diags := parser.Parse([]byte(src), "test.ast")
	if diags.HasErrors() {
		t.Fatalf("el .ast de prueba no parsea (bug del test, no del compilador):\n%s", diags.String())
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
	manifest, diags := Compile(prog)
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores, tuve:\n%s", diags.String())
	}

	if manifest.Name != "asterion-mail-plugin-basic" {
		t.Errorf("Name = %q", manifest.Name)
	}
	if manifest.Version != "0.1.0" {
		t.Errorf("Version = %q", manifest.Version)
	}
	if manifest.ContractVersion != apc.ContractVersion {
		t.Errorf("ContractVersion = %q, quería %q", manifest.ContractVersion, apc.ContractVersion)
	}
	if manifest.Language == nil || manifest.Language.Name != "go" || manifest.Language.Version != "1.25" {
		t.Errorf("Language = %+v", manifest.Language)
	}
	if manifest.Start.Command != "./asterion-mail-plugin-basic" {
		t.Errorf("Start.Command = %q", manifest.Start.Command)
	}
	if manifest.HealthPath != "/health" {
		t.Errorf("HealthPath = %q", manifest.HealthPath)
	}
	if manifest.API == nil || manifest.API.BasePath != "/api/v1" {
		t.Errorf("API = %+v", manifest.API)
	}
	if manifest.Permissions == nil || !manifest.Permissions.Secrets || len(manifest.Permissions.Network) != 1 || manifest.Permissions.Network[0] != "smtp" {
		t.Errorf("Permissions = %+v", manifest.Permissions)
	}
	if len(manifest.ConfigSchema) != 2 {
		t.Fatalf("ConfigSchema tiene %d campos, quería 2", len(manifest.ConfigSchema))
	}
	if manifest.ConfigSchema[0].Key != "smtp_host" || !manifest.ConfigSchema[0].Required {
		t.Errorf("ConfigSchema[0] = %+v", manifest.ConfigSchema[0])
	}
	if len(manifest.Resources) != 1 || manifest.Resources[0].Name != "templates" || len(manifest.Resources[0].CRUD) != 5 {
		t.Errorf("Resources = %+v", manifest.Resources)
	}
	if len(manifest.Actions) != 1 || manifest.Actions[0].Method != "POST" || manifest.Actions[0].Endpoint != "/send" {
		t.Errorf("Actions = %+v", manifest.Actions)
	}

	// El resultado tiene que ser, además, un manifest realmente válido
	// según las reglas ya existentes — la prueba de fuego de que este
	// compilador no inventa su propio criterio de "válido".
	if err := manifest.Validate(); err != nil {
		t.Errorf("el manifest compilado no pasa apc.Manifest.Validate(): %v", err)
	}
}

func TestCompile_ASTR300_NotAContractCall(t *testing.T) {
	prog := parseOK(t, `
Contract.define(name="x", version="1.0.0")
Contract.start(command="./x")
def helper():
    return 1
`)
	_, diags := Compile(prog)
	if !hasCode(diags, "ASTR300") {
		t.Errorf("esperaba ASTR300, tuve: %v", codes(diags))
	}
}

func TestCompile_ASTR300_NonContractCall(t *testing.T) {
	prog := parseOK(t, `
Contract.define(name="x", version="1.0.0")
Provider.aws.instance(cpu=1)
`)
	_, diags := Compile(prog)
	if !hasCode(diags, "ASTR300") {
		t.Errorf("esperaba ASTR300, tuve: %v", codes(diags))
	}
}

func TestCompile_ASTR301_UnknownVerb(t *testing.T) {
	prog := parseOK(t, `
Contract.define(name="x", version="1.0.0")
Contract.nope(foo="bar")
`)
	_, diags := Compile(prog)
	if !hasCode(diags, "ASTR301") {
		t.Errorf("esperaba ASTR301, tuve: %v", codes(diags))
	}
}

func TestCompile_ASTR302_MissingRequiredArg(t *testing.T) {
	prog := parseOK(t, `
Contract.define(version="1.0.0")
`)
	_, diags := Compile(prog)
	if !hasCode(diags, "ASTR302") {
		t.Errorf("esperaba ASTR302 (falta 'name'), tuve: %v", codes(diags))
	}
}

func TestCompile_ASTR303_WrongArgType(t *testing.T) {
	prog := parseOK(t, `
Contract.define(name="x", version="1.0.0")
Contract.start(command="./x", port="no-es-un-numero")
`)
	_, diags := Compile(prog)
	if !hasCode(diags, "ASTR303") {
		t.Errorf("esperaba ASTR303 (port debe ser entero), tuve: %v", codes(diags))
	}
}

func TestCompile_ASTR304_DuplicateOnceVerb(t *testing.T) {
	prog := parseOK(t, `
Contract.define(name="x", version="1.0.0")
Contract.define(name="y", version="2.0.0")
`)
	_, diags := Compile(prog)
	if !hasCode(diags, "ASTR304") {
		t.Errorf("esperaba ASTR304 (define llamado dos veces), tuve: %v", codes(diags))
	}
}

func TestCompile_ASTR305_NeverDefined(t *testing.T) {
	prog := parseOK(t, `
Contract.start(command="./x")
`)
	_, diags := Compile(prog)
	if !hasCode(diags, "ASTR305") {
		t.Errorf("esperaba ASTR305 (nunca llamó Contract.define), tuve: %v", codes(diags))
	}
}

func TestCompile_RepeatableVerbsAccumulate(t *testing.T) {
	prog := parseOK(t, `
Contract.define(name="x", version="1.0.0")
Contract.start(command="./x")
Contract.resource(name="a", endpoint="/a", crud=["list"])
Contract.resource(name="b", endpoint="/b", crud=["list"])
Contract.action(name="one", method="GET", endpoint="/one")
Contract.action(name="two", method="POST", endpoint="/two")
`)
	manifest, diags := Compile(prog)
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores, tuve:\n%s", diags.String())
	}
	if len(manifest.Resources) != 2 {
		t.Errorf("Resources tiene %d, quería 2", len(manifest.Resources))
	}
	if len(manifest.Actions) != 2 {
		t.Errorf("Actions tiene %d, quería 2", len(manifest.Actions))
	}
}

// TestCompile_ExampleFile compila examples/plugin-manifest.ast (el mismo
// que linkea el README y el CLI 'asterion plugin from-ast --help') — si
// esto se rompe, la documentación también quedó mintiendo, mismo criterio
// que TestGoldenExamples en semantic/.
func TestCompile_ExampleFile(t *testing.T) {
	path := filepath.Join("..", "examples", "plugin-manifest.ast")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no pude leer %s: %v", path, err)
	}
	prog, parseDiags := parser.Parse(src, path)
	if parseDiags.HasErrors() {
		t.Fatalf("%s no debería tener errores de lexer/parser: %s", path, parseDiags)
	}
	manifest, diags := Compile(prog)
	if diags.HasErrors() {
		t.Fatalf("%s no debería tener errores de compilación: %s", path, diags)
	}
	if manifest.Name != "asterion-mail-plugin-basic" {
		t.Errorf("Name = %q", manifest.Name)
	}
	if err := manifest.Validate(); err != nil {
		t.Errorf("el manifest compilado desde el ejemplo no pasa Validate(): %v", err)
	}
}

func hasCode(diags *diagnostics.Bag, code string) bool {
	for _, d := range diags.Items() {
		if d.Code == code {
			return true
		}
	}
	return false
}
