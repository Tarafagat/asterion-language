package systemspec

import (
	"testing"

	"github.com/Tarafagat/asterion-language/ast"
	"github.com/Tarafagat/asterion-language/diagnostics"
	"github.com/Tarafagat/asterion-language/parser"
)

const happyPath = `
language "0.1"

db = System.plugin(route="./mi-plugin-db", principal=true)
api = System.plugin(route="github.com/user/api-plugin", ref="v1.2.3", requires=["node@20.11.0"])

System.wire(to=api, key="DATABASE_URL", from=db)
System.wire(to=api, key="DB_NAME", from=db, field="env:database_name")
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
	plugins, wires, diags := Compile(prog)
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores, tuve:\n%s", diags.String())
	}

	if len(plugins) != 2 {
		t.Fatalf("plugins = %d, want 2", len(plugins))
	}
	want0 := PluginDecl{Name: "db", Route: "./mi-plugin-db", Principal: true}
	if plugins[0].Name != want0.Name || plugins[0].Route != want0.Route || plugins[0].Ref != want0.Ref ||
		len(plugins[0].Requires) != 0 || plugins[0].Principal != want0.Principal {
		t.Errorf("plugins[0] = %+v, want %+v", plugins[0], want0)
	}
	want1 := PluginDecl{Name: "api", Route: "github.com/user/api-plugin", Ref: "v1.2.3", Requires: []string{"node@20.11.0"}}
	if plugins[1].Name != want1.Name || plugins[1].Route != want1.Route || plugins[1].Ref != want1.Ref ||
		len(plugins[1].Requires) != 1 || plugins[1].Requires[0] != "node@20.11.0" || plugins[1].Principal {
		t.Errorf("plugins[1] = %+v, want %+v", plugins[1], want1)
	}

	if len(wires) != 2 {
		t.Fatalf("wires = %d, want 2", len(wires))
	}
	if wires[0] != (WireDecl{ToPlugin: "api", Key: "DATABASE_URL", FromPlugin: "db", Field: "port"}) {
		t.Errorf("wires[0] = %+v (field debería defaultear a \"port\")", wires[0])
	}
	if wires[1] != (WireDecl{ToPlugin: "api", Key: "DB_NAME", FromPlugin: "db", Field: "env:database_name"}) {
		t.Errorf("wires[1] = %+v", wires[1])
	}
}

func TestCompile_WireToUndeclaredPlugin(t *testing.T) {
	prog := parseOK(t, `
db = System.plugin(route="./db")
System.wire(to=nonexistent, key="X", from=db)
`)
	_, _, diags := Compile(prog)
	if !diags.HasErrors() {
		t.Fatal("esperaba un error por referenciar un plugin no declarado")
	}
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR504" {
		t.Errorf("codes = %v, want [ASTR504]", got)
	}
}

func TestCompile_WireToStringLiteralInsteadOfReference(t *testing.T) {
	prog := parseOK(t, `
db = System.plugin(route="./db")
System.wire(to="db", key="X", from=db)
`)
	_, _, diags := Compile(prog)
	if !diags.HasErrors() {
		t.Fatal("esperaba un error: 'to' fue un string literal, no una referencia")
	}
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR503" {
		t.Errorf("codes = %v, want [ASTR503]", got)
	}
}

func TestCompile_DuplicatePluginName(t *testing.T) {
	prog := parseOK(t, `
db = System.plugin(route="./db")
db = System.plugin(route="./otra-carpeta")
`)
	plugins, _, diags := Compile(prog)
	if !diags.HasErrors() {
		t.Fatal("esperaba un error por nombre de plugin duplicado")
	}
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR500" {
		t.Errorf("codes = %v, want [ASTR500]", got)
	}
	if len(plugins) != 1 {
		t.Errorf("plugins = %d, want 1 (la segunda declaración se rechaza, no se agrega)", len(plugins))
	}
}

func TestCompile_MissingRoute(t *testing.T) {
	prog := parseOK(t, `db = System.plugin(principal=true)`)
	_, _, diags := Compile(prog)
	if !diags.HasErrors() {
		t.Fatal("esperaba un error por falta de 'route'")
	}
	if got := codes(diags); len(got) != 1 || got[0] != "ASTR501" {
		t.Errorf("codes = %v, want [ASTR501]", got)
	}
}

func TestCompile_ForwardReferenceRejected(t *testing.T) {
	// api se referencia antes de declararse — el parser/lexer lo aceptan
	// (no hay validación de orden ahí), pero systemspec.Compile procesa
	// en orden de aparición, así que 'api' todavía no está en `declared`
	// cuando se llega a wire().
	prog := parseOK(t, `
System.wire(to=api, key="X", from=db)
db = System.plugin(route="./db")
api = System.plugin(route="./api")
`)
	_, _, diags := Compile(prog)
	if !diags.HasErrors() {
		t.Fatal("esperaba un error: 'api'/'db' todavía no estaban declarados en esta línea")
	}
}

func TestCompile_IgnoresUnrelatedStatements(t *testing.T) {
	// Un archivo de sistema puede tener otras cosas (comentarios ya los
	// saca el lexer; acá probamos una llamada común que no es
	// System.plugin/System.wire) — no debería producir ni plugins ni
	// wires ni error.
	prog := parseOK(t, `
otro = SomeOtherThing(x=1)
db = System.plugin(route="./db")
`)
	plugins, wires, diags := Compile(prog)
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores:\n%s", diags.String())
	}
	if len(plugins) != 1 || len(wires) != 0 {
		t.Errorf("plugins=%d wires=%d, want 1/0", len(plugins), len(wires))
	}
}
