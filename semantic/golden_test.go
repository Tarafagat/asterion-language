package semantic

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tarafagat/asterion-language/parser"
)

// TestGoldenExamples corre el pipeline completo (lexer → parser →
// semantic) sobre cada archivo real de examples/ — son los mismos
// ejemplos citados en el README y en la especificación, así que si algo
// acá se rompe, la documentación también quedó mintiendo.
func TestGoldenExamples(t *testing.T) {
	cases := []struct {
		file       string
		wantErrors bool
		wantCodes  []string // si wantErrors, al menos estos códigos deben aparecer
	}{
		{"minimal.asterion", false, nil},
		{"instance.asterion", false, nil},
		{"network.asterion", false, nil},
		{"multi-resource.asterion", false, nil},
		{"provider.asterion", false, nil},
		{"lab.asterion", false, nil},
		{"plugin.asterion", false, nil},
		{"error.asterion", true, []string{"ASTR202", "ASTR201", "ASTR211"}},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join("..", "examples", tc.file)
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("no pude leer %s: %v", path, err)
			}
			prog, diags := parser.Parse(src, tc.file)
			if diags.HasErrors() {
				t.Fatalf("%s no debería tener errores de lexer/parser: %s", tc.file, diags)
			}
			semDiags := NewAnalyzer(nil).Analyze(prog)
			if tc.wantErrors != semDiags.HasErrors() {
				t.Fatalf("%s: esperaba errores=%v, dio errores=%v (%s)", tc.file, tc.wantErrors, semDiags.HasErrors(), semDiags)
			}
			for _, code := range tc.wantCodes {
				found := false
				for _, d := range semDiags.Items() {
					if d.Code == code {
						found = true
					}
				}
				if !found {
					t.Errorf("%s: esperaba el código %s entre los diagnósticos, dio %s", tc.file, code, semDiags)
				}
			}
		})
	}
}
