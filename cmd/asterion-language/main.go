// Command asterion-language es el binario standalone del compilador —
// existe para poder correr 'check' sin depender de asterion-core (útil
// para CI de un plugin, un editor, etc). Cuando se invoca como
// 'asterion language check' DESDE asterion-core, en cambio, no se llama a
// este binario: asterion-core importa este módulo directo y le inyecta un
// CapabilityResolver respaldado por su propio Registry real — ver
// cmd/asterion/language.go en ese repo. Acá, sin ese contexto, se usa
// siempre semantic.StaticCapabilityResolver (el snapshot de referencia).
package main

import (
	"fmt"
	"os"

	"github.com/Tarafagat/asterion-language/parser"
	"github.com/Tarafagat/asterion-language/semantic"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "check" {
		fmt.Fprintln(os.Stderr, "uso: asterion-language check <archivo.ast>")
		os.Exit(2)
	}
	path := os.Args[2]
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "no pude leer %s: %v\n", path, err)
		os.Exit(1)
	}

	prog, diags := parser.Parse(src, path)
	if diags.HasErrors() {
		fmt.Fprint(os.Stderr, diags.String())
		os.Exit(1)
	}

	semDiags := semantic.NewAnalyzer(nil).Analyze(prog)
	if semDiags.HasErrors() {
		fmt.Fprint(os.Stderr, semDiags.String())
		os.Exit(1)
	}

	fmt.Printf("✓ %s — %d statement(s), sin errores (contract_version: %s)\n", path, len(prog.Statements), orDefault(prog.LanguageVersion, "0.1 (asumida)"))
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
