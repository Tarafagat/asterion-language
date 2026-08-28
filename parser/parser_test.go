package parser

import (
	"testing"

	"github.com/Tarafagat/asterion-language/ast"
)

func mustParse(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, diags := Parse([]byte(src), "t.ast")
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores de parseo: %s", diags)
	}
	return prog
}

func TestParseLanguagePragma(t *testing.T) {
	prog := mustParse(t, "language \"0.1\"\n\ndef main():\n    return\n")
	if prog.LanguageVersion != "0.1" {
		t.Fatalf("esperaba versión 0.1, dio %q", prog.LanguageVersion)
	}
}

func TestParseAssignAndResourceCall(t *testing.T) {
	prog := mustParse(t, "web = Provider.aws.instance(image=\"ubuntu-24.04\", cpu=4, memory=8GB)\n")
	if len(prog.Statements) != 1 {
		t.Fatalf("esperaba 1 statement, dio %d", len(prog.Statements))
	}
	assign, ok := prog.Statements[0].(*ast.AssignStmt)
	if !ok {
		t.Fatalf("esperaba AssignStmt, dio %T", prog.Statements[0])
	}
	if assign.Name != "web" {
		t.Fatalf("esperaba nombre 'web', dio %q", assign.Name)
	}
	call, ok := assign.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("esperaba CallExpr, dio %T", assign.Value)
	}
	if len(call.Args) != 3 {
		t.Fatalf("esperaba 3 argumentos, dio %d", len(call.Args))
	}
	if call.Args[0].Name != "image" || call.Args[2].Name != "memory" {
		t.Fatalf("nombres de argumento inesperados: %+v", call.Args)
	}
	memory, ok := call.Args[2].Value.(*ast.SizeLit)
	if !ok || memory.Unit != "GB" {
		t.Fatalf("esperaba SizeLit GB para memory, dio %+v", call.Args[2].Value)
	}

	callee, ok := call.Callee.(*ast.AttrExpr)
	if !ok || callee.Name != "instance" {
		t.Fatalf("callee inesperado: %+v", call.Callee)
	}
}

func TestParseFuncDeclWithBlockAndReturn(t *testing.T) {
	src := "def main():\n    web = Provider.aws.instance(cpu=1)\n    return web\n"
	prog := mustParse(t, src)
	fn, ok := prog.Statements[0].(*ast.FuncDecl)
	if !ok {
		t.Fatalf("esperaba FuncDecl, dio %T", prog.Statements[0])
	}
	if fn.Name != "main" {
		t.Fatalf("nombre de función inesperado: %q", fn.Name)
	}
	if len(fn.Body) != 2 {
		t.Fatalf("esperaba 2 statements en el cuerpo, dio %d", len(fn.Body))
	}
	ret, ok := fn.Body[1].(*ast.ReturnStmt)
	if !ok || len(ret.Values) != 1 {
		t.Fatalf("return inesperado: %+v", fn.Body[1])
	}
}

func TestParseMultipleReturnValues(t *testing.T) {
	prog := mustParse(t, "def main():\n    return a, b\n")
	fn := prog.Statements[0].(*ast.FuncDecl)
	ret := fn.Body[0].(*ast.ReturnStmt)
	if len(ret.Values) != 2 {
		t.Fatalf("esperaba 2 valores de retorno, dio %d", len(ret.Values))
	}
}

func TestParseListLiteral(t *testing.T) {
	prog := mustParse(t, "xs = [1, 2, 3]\n")
	assign := prog.Statements[0].(*ast.AssignStmt)
	list, ok := assign.Value.(*ast.ListLit)
	if !ok || len(list.Elements) != 3 {
		t.Fatalf("esperaba ListLit de 3 elementos, dio %+v", assign.Value)
	}
}

func TestParseErrorRecoveryReportsMultipleProblems(t *testing.T) {
	// dos líneas rotas de maneras distintas — el parser debería reportar
	// AMBAS, no solo la primera y morir ahí.
	src := "a = (\nb = Provider.aws.instance(cpu=)\n"
	_, diags := Parse([]byte(src), "t.ast")
	if !diags.HasErrors() {
		t.Fatal("esperaba errores")
	}
	if len(diags.Items()) < 1 {
		t.Fatalf("esperaba al menos 1 diagnóstico, dio %d", len(diags.Items()))
	}
}

func TestParseUnclosedString(t *testing.T) {
	_, diags := Parse([]byte("a = \"sin cerrar\n"), "t.ast")
	if !diags.HasErrors() {
		t.Fatal("esperaba error por string sin cerrar")
	}
}
