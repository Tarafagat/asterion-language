package providerspec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tarafagat/asterion-language/ast"
	"github.com/Tarafagat/asterion-language/diagnostics"
	"github.com/Tarafagat/asterion-language/parser"
)

func parseOK(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, diags := parser.Parse([]byte(src), "test.asterion")
	if diags.HasErrors() {
		t.Fatalf("el .asterion de prueba no parsea (bug del test, no del compilador):\n%s", diags.String())
	}
	return prog
}

func hasCode(diags *diagnostics.Bag, code string) bool {
	for _, d := range diags.Items() {
		if d.Code == code {
			return true
		}
	}
	return false
}

func codes(diags *diagnostics.Bag) []string {
	out := make([]string, 0, len(diags.Items()))
	for _, d := range diags.Items() {
		out = append(out, d.Code)
	}
	return out
}

const gcpHappyPath = `
language "0.1"

def main():
    web = Provider.gcp.instance(
        region="us-central1-a",
        shape_code="e2-micro",
        image="projects/debian-cloud/global/images/family/debian-12"
    )
    return web
`

func TestCompileInstances_HappyPath(t *testing.T) {
	prog := parseOK(t, gcpHappyPath)
	specs, diags := CompileInstances(prog)
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores, tuve:\n%s", diags.String())
	}
	if len(specs) != 1 {
		t.Fatalf("esperaba 1 spec, hubo %d", len(specs))
	}
	spec := specs[0]
	if spec.Name != "web" {
		t.Errorf("Name = %q, quería \"web\" (del AssignStmt)", spec.Name)
	}
	if spec.Provider != "gcp" {
		t.Errorf("Provider = %q", spec.Provider)
	}
	if spec.Region != "us-central1-a" {
		t.Errorf("Region = %q", spec.Region)
	}
	if spec.ShapeCode != "e2-micro" {
		t.Errorf("ShapeCode = %q", spec.ShapeCode)
	}
	if spec.Image != "projects/debian-cloud/global/images/family/debian-12" {
		t.Errorf("Image = %q", spec.Image)
	}
	if spec.AssignPublicIP {
		t.Errorf("AssignPublicIP debería ser false por default")
	}
}

func TestCompileInstances_AssignPublicIP(t *testing.T) {
	prog := parseOK(t, `
def main():
    web = Provider.gcp.instance(
        region="us-central1-a",
        shape_code="e2-micro",
        image="projects/debian-cloud/global/images/family/debian-12",
        assign_public_ip=true
    )
    return web
`)
	specs, diags := CompileInstances(prog)
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores, tuve:\n%s", diags.String())
	}
	if !specs[0].AssignPublicIP {
		t.Errorf("AssignPublicIP debería ser true")
	}
}

func TestCompileInstances_ASTR400_MissingRequiredArg(t *testing.T) {
	prog := parseOK(t, `
def main():
    web = Provider.gcp.instance(
        shape_code="e2-micro",
        image="projects/debian-cloud/global/images/family/debian-12"
    )
    return web
`)
	_, diags := CompileInstances(prog)
	if !hasCode(diags, "ASTR400") {
		t.Errorf("esperaba ASTR400 (falta 'region'), tuve: %v", codes(diags))
	}
}

func TestCompileInstances_ASTR401_WrongArgType(t *testing.T) {
	prog := parseOK(t, `
def main():
    web = Provider.gcp.instance(
        region="us-central1-a",
        shape_code="e2-micro",
        image="projects/debian-cloud/global/images/family/debian-12",
        assign_public_ip="si"
    )
    return web
`)
	_, diags := CompileInstances(prog)
	if !hasCode(diags, "ASTR401") {
		t.Errorf("esperaba ASTR401 (assign_public_ip debe ser bool), tuve: %v", codes(diags))
	}
}

func TestCompileInstances_ASTR402_UnsupportedMethod(t *testing.T) {
	prog := parseOK(t, `
def main():
    net = Provider.gcp.network(cidr_block="10.0.0.0/16")
    return net
`)
	_, diags := CompileInstances(prog)
	if !hasCode(diags, "ASTR402") {
		t.Errorf("esperaba ASTR402 (.network todavía no soportado), tuve: %v", codes(diags))
	}
}

func TestCompileInstances_OCI_HappyPath(t *testing.T) {
	prog := parseOK(t, `
def main():
    web = Provider.oci.instance(
        region="us-ashburn-1",
        shape_code="VM.Standard.E2.1.Micro",
        image="ocid1.image.oc1.iad.aaaa",
        subnet="ocid1.subnet.oc1.iad.aaaa",
        assign_public_ip=true
    )
    return web
`)
	specs, diags := CompileInstances(prog)
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores, tuve:\n%s", diags.String())
	}
	if len(specs) != 1 {
		t.Fatalf("esperaba 1 spec, hubo %d", len(specs))
	}
	spec := specs[0]
	if spec.Provider != "oci" {
		t.Errorf("Provider = %q", spec.Provider)
	}
	if spec.Region != "us-ashburn-1" {
		t.Errorf("Region = %q", spec.Region)
	}
	if spec.Subnet != "ocid1.subnet.oc1.iad.aaaa" {
		t.Errorf("Subnet = %q", spec.Subnet)
	}
}

// TestCompileInstances_ExampleFile_OCI — mismo criterio que
// TestCompileInstances_ExampleFile para el ejemplo de GCP.
func TestCompileInstances_ExampleFile_OCI(t *testing.T) {
	path := filepath.Join("..", "examples", "oci_instance.asterion")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no pude leer %s: %v", path, err)
	}
	prog, parseDiags := parser.Parse(src, path)
	if parseDiags.HasErrors() {
		t.Fatalf("%s no debería tener errores de lexer/parser: %s", path, parseDiags)
	}
	specs, diags := CompileInstances(prog)
	if diags.HasErrors() {
		t.Fatalf("%s no debería tener errores de compilación: %s", path, diags)
	}
	if len(specs) != 1 || specs[0].Provider != "oci" || specs[0].Subnet == "" {
		t.Errorf("specs = %+v", specs)
	}
}

func TestCompileInstances_ASTR403_UnsupportedProvider(t *testing.T) {
	prog := parseOK(t, `
def main():
    web = Provider.aws.instance(
        region="us-east-1",
        shape_code="t3.micro",
        image="ami-12345"
    )
    return web
`)
	_, diags := CompileInstances(prog)
	if !hasCode(diags, "ASTR403") {
		t.Errorf("esperaba ASTR403 (aws sin adapter real todavía), tuve: %v", codes(diags))
	}
}

func TestCompileInstances_IgnoresUnrelatedStatements(t *testing.T) {
	prog := parseOK(t, `
def main():
    web = Provider.gcp.instance(
        region="us-central1-a",
        shape_code="e2-micro",
        image="projects/debian-cloud/global/images/family/debian-12"
    )
    return web

def helper():
    return 1
`)
	specs, diags := CompileInstances(prog)
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores, tuve:\n%s", diags.String())
	}
	if len(specs) != 1 {
		t.Fatalf("esperaba 1 spec (helper() no debería afectar nada), hubo %d", len(specs))
	}
}

// TestCompileInstances_ExampleFile compila examples/gcp_instance.asterion
// — si esto se rompe, el ejemplo que linkea la documentación también
// quedó mintiendo, mismo criterio que pluginmanifest.TestCompile_ExampleFile.
func TestCompileInstances_ExampleFile(t *testing.T) {
	path := filepath.Join("..", "examples", "gcp_instance.asterion")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no pude leer %s: %v", path, err)
	}
	prog, parseDiags := parser.Parse(src, path)
	if parseDiags.HasErrors() {
		t.Fatalf("%s no debería tener errores de lexer/parser: %s", path, parseDiags)
	}
	specs, diags := CompileInstances(prog)
	if diags.HasErrors() {
		t.Fatalf("%s no debería tener errores de compilación: %s", path, diags)
	}
	if len(specs) != 1 || specs[0].Provider != "gcp" || specs[0].ShapeCode != "e2-micro" {
		t.Errorf("specs = %+v", specs)
	}
}
