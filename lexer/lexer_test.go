package lexer

import "testing"

func typesOf(toks []Token) []TokenType {
	out := make([]TokenType, len(toks))
	for i, t := range toks {
		out[i] = t.Type
	}
	return out
}

func assertTypes(t *testing.T, got []Token, want []TokenType) {
	t.Helper()
	gotTypes := typesOf(got)
	if len(gotTypes) != len(want) {
		t.Fatalf("cantidad de tokens distinta: got %v, want %v", gotTypes, want)
	}
	for i := range want {
		if gotTypes[i] != want[i] {
			t.Fatalf("token %d: got %s, want %s (todos: %v)", i, gotTypes[i], want[i], gotTypes)
		}
	}
}

func TestIdentifiersAndKeywords(t *testing.T) {
	toks, diags := Lex([]byte("def main web_1"), "t.asterion")
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores: %s", diags)
	}
	assertTypes(t, toks, []TokenType{DEF, IDENT, IDENT, NEWLINE, EOF})
	if toks[1].Lit != "main" || toks[2].Lit != "web_1" {
		t.Fatalf("literales inesperados: %+v", toks[1:3])
	}
}

func TestNumbersSizesAndDurations(t *testing.T) {
	toks, diags := Lex([]byte("4 3.5 8GB 500MB 30s 5m"), "t.asterion")
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores: %s", diags)
	}
	assertTypes(t, toks, []TokenType{INT, FLOAT, SIZE, SIZE, DURATION, DURATION, NEWLINE, EOF})

	gb := toks[2].Value.(SizeValue)
	if gb.Bytes != 8*1024*1024*1024 {
		t.Fatalf("8GB debería ser %d bytes, dio %d", 8*1024*1024*1024, gb.Bytes)
	}
	dur := toks[4].Value.(DurationValue)
	if dur.Seconds != 30 {
		t.Fatalf("30s debería ser 30 segundos, dio %v", dur.Seconds)
	}
	fiveMin := toks[5].Value.(DurationValue)
	if fiveMin.Seconds != 300 {
		t.Fatalf("5m debería ser 300 segundos, dio %v", fiveMin.Seconds)
	}
}

func TestStringsAndComments(t *testing.T) {
	toks, diags := Lex([]byte("\"hola mundo\" # esto es un comentario\n'con comillas simples'"), "t.asterion")
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores: %s", diags)
	}
	if toks[0].Type != STRING || toks[0].Lit != "hola mundo" {
		t.Fatalf("string mal lexeado: %+v", toks[0])
	}
}

func TestIndentation(t *testing.T) {
	src := "def main():\n    x = 1\n    y = 2\nz = 3\n"
	toks, diags := Lex([]byte(src), "t.asterion")
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores: %s", diags)
	}
	assertTypes(t, toks, []TokenType{
		DEF, IDENT, LPAREN, RPAREN, COLON, NEWLINE,
		INDENT,
		IDENT, ASSIGN, INT, NEWLINE,
		IDENT, ASSIGN, INT, NEWLINE,
		DEDENT,
		IDENT, ASSIGN, INT, NEWLINE,
		EOF,
	})
}

func TestBlankLinesDontAffectIndentation(t *testing.T) {
	src := "def main():\n    x = 1\n\n    # comentario suelto\n\n    y = 2\n"
	toks, diags := Lex([]byte(src), "t.asterion")
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores: %s", diags)
	}
	assertTypes(t, toks, []TokenType{
		DEF, IDENT, LPAREN, RPAREN, COLON, NEWLINE,
		INDENT,
		IDENT, ASSIGN, INT, NEWLINE,
		IDENT, ASSIGN, INT, NEWLINE,
		DEDENT,
		EOF,
	})
}

func TestParensSuppressNewlines(t *testing.T) {
	src := "f(\n  a,\n  b\n)\n"
	toks, diags := Lex([]byte(src), "t.asterion")
	if diags.HasErrors() {
		t.Fatalf("no esperaba errores: %s", diags)
	}
	assertTypes(t, toks, []TokenType{IDENT, LPAREN, IDENT, COMMA, IDENT, RPAREN, NEWLINE, EOF})
}

func TestTabsAreRejected(t *testing.T) {
	_, diags := Lex([]byte("def main():\n\tx = 1\n"), "t.asterion")
	if !diags.HasErrors() {
		t.Fatal("esperaba un error por usar tabs")
	}
	found := false
	for _, d := range diags.Items() {
		if d.Code == "ASTR001" {
			found = true
		}
	}
	if !found {
		t.Fatalf("esperaba ASTR001, diags: %s", diags)
	}
}

func TestUnknownUnitIsReported(t *testing.T) {
	_, diags := Lex([]byte("5XB"), "t.asterion")
	if !diags.HasErrors() {
		t.Fatal("esperaba un error por unidad desconocida")
	}
}
