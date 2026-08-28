package semantic

import (
	"testing"

	"github.com/Tarafagat/asterion-language/parser"
)

func analyze(t *testing.T, src string) []string {
	t.Helper()
	prog, parseDiags := parser.Parse([]byte(src), "t.ast")
	if parseDiags.HasErrors() {
		t.Fatalf("no esperaba errores de parseo: %s", parseDiags)
	}
	diags := NewAnalyzer(nil).Analyze(prog)
	codes := make([]string, 0, len(diags.Items()))
	for _, d := range diags.Items() {
		codes = append(codes, d.Code)
	}
	return codes
}

func TestValidProviderResourceHasNoErrors(t *testing.T) {
	codes := analyze(t, "web = Provider.aws.instance(image=\"ubuntu-24.04\", cpu=4, memory=8GB)\n")
	if len(codes) != 0 {
		t.Fatalf("no esperaba errores, dio %v", codes)
	}
}

func TestUndefinedReferenceIsReported(t *testing.T) {
	codes := analyze(t, "web = Provider.aws.instance(network=nunca_declarada)\n")
	if !contains(codes, "ASTR202") {
		t.Fatalf("esperaba ASTR202, dio %v", codes)
	}
}

func TestDuplicateResourceNameIsReported(t *testing.T) {
	src := "a = Provider.aws.instance(cpu=1)\na = Provider.aws.instance(cpu=1)\n"
	codes := analyze(t, src)
	if !contains(codes, "ASTR201") {
		t.Fatalf("esperaba ASTR201, dio %v", codes)
	}
}

func TestUnknownProviderIsReported(t *testing.T) {
	codes := analyze(t, "x = Provider.does_not_exist.instance(cpu=1)\n")
	if !contains(codes, "ASTR210") {
		t.Fatalf("esperaba ASTR210, dio %v", codes)
	}
}

func TestUnknownMethodIsReported(t *testing.T) {
	codes := analyze(t, "x = Provider.aws.load_balancer(cpu=1)\n")
	if !contains(codes, "ASTR211") {
		t.Fatalf("esperaba ASTR211, dio %v", codes)
	}
}

func TestMissingCapabilityIsReported(t *testing.T) {
	// resolver de prueba donde 'oci' declara 'compute' pero no 'database' —
	// no depende del mapa estático real (semantic/resolver.go), así el test
	// sigue siendo válido si ese mapa cambia.
	resolver := fakeResolver{
		providers: []string{"oci"},
		caps:      map[string]map[string]bool{"oci": {"compute": true}},
	}
	prog, parseDiags := parser.Parse([]byte("x = Provider.oci.database(engine=\"postgres\")\n"), "t.ast")
	if parseDiags.HasErrors() {
		t.Fatalf("no esperaba errores de parseo: %s", parseDiags)
	}
	diags := NewAnalyzer(resolver).Analyze(prog)
	found := false
	for _, d := range diags.Items() {
		if d.Code == "ASTR212" {
			found = true
			if d.Detail == "" {
				t.Fatal("ASTR212 debería traer el detalle de capabilities disponibles/faltantes")
			}
		}
	}
	if !found {
		t.Fatalf("esperaba ASTR212, dio %v", diags.Items())
	}
}

func TestDependenciesAreCollected(t *testing.T) {
	prog, parseDiags := parser.Parse([]byte(
		"network = Network(cidr=\"10.0.0.0/24\")\nweb = Provider.aws.instance(cpu=1, network=network)\n",
	), "t.ast")
	if parseDiags.HasErrors() {
		t.Fatalf("no esperaba errores de parseo: %s", parseDiags)
	}
	a := NewAnalyzer(nil)
	diags := a.Analyze(prog)
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores semánticos: %s", diags)
	}
	// no hay forma directa de inspeccionar el scope final desde afuera del
	// paquete (a propósito — es un detalle de implementación), así que este
	// test solo confirma que declarar y referenciar en orden compila limpio;
	// TestUndefinedReferenceIsReported ya cubre el caso contrario.
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

type fakeResolver struct {
	providers []string
	caps      map[string]map[string]bool
}

func (f fakeResolver) Providers() []string { return f.providers }
func (f fakeResolver) HasCapability(provider, capability string) bool {
	return f.caps[provider][capability]
}
