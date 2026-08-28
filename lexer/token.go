package lexer

import "github.com/Tarafagat/asterion-language/diagnostics"

// TokenType enumera los tokens de v0.1 de la gramática — ver
// spec/grammar.md. Deliberadamente chico: no hay operadores aritméticos,
// no hay comprehensions, no hay f-strings — nada que la especificación no
// haya pedido explícitamente para esta versión.
type TokenType int

const (
	EOF TokenType = iota
	NEWLINE
	INDENT
	DEDENT

	IDENT
	STRING
	INT
	FLOAT
	SIZE     // 8GB, 500MB, 1TB
	DURATION // 30s, 5m, 1h

	// Palabras clave — ver spec/grammar.md §keywords. Deliberadamente
	// pocas: no hay "class", "if", "for", "while", "import" en v0.1.
	DEF
	RETURN
	TRUE
	FALSE

	LPAREN
	RPAREN
	LBRACKET
	RBRACKET
	COMMA
	DOT
	ASSIGN
	COLON
	QUESTION // propagación de error, ver spec/errors.md — se lexea desde v0.1, la semántica llega después
)

var keywords = map[string]TokenType{
	"def":    DEF,
	"return": RETURN,
	"true":   TRUE,
	"false":  FALSE,
}

func (t TokenType) String() string {
	switch t {
	case EOF:
		return "EOF"
	case NEWLINE:
		return "NEWLINE"
	case INDENT:
		return "INDENT"
	case DEDENT:
		return "DEDENT"
	case IDENT:
		return "IDENT"
	case STRING:
		return "STRING"
	case INT:
		return "INT"
	case FLOAT:
		return "FLOAT"
	case SIZE:
		return "SIZE"
	case DURATION:
		return "DURATION"
	case DEF:
		return "def"
	case RETURN:
		return "return"
	case TRUE:
		return "true"
	case FALSE:
		return "false"
	case LPAREN:
		return "("
	case RPAREN:
		return ")"
	case LBRACKET:
		return "["
	case RBRACKET:
		return "]"
	case COMMA:
		return ","
	case DOT:
		return "."
	case ASSIGN:
		return "="
	case COLON:
		return ":"
	case QUESTION:
		return "?"
	default:
		return "?"
	}
}

// Token es una unidad léxica ya clasificada. Lit conserva el texto crudo
// (útil para mensajes de error); Value trae el literal ya interpretado
// para STRING/INT/FLOAT/SIZE/DURATION (nil para el resto).
type Token struct {
	Type  TokenType
	Lit   string
	Value any
	Pos   diagnostics.Position
}

// SizeValue es lo que trae Token.Value para un token SIZE — bytes, ya
// normalizado, para que el semantic analyzer nunca tenga que reparsear la
// unidad.
type SizeValue struct {
	Bytes int64
	Unit  string // la unidad tal como la escribió el usuario, para mensajes de error
}

// DurationValue es análogo para DURATION, normalizado a segundos.
type DurationValue struct {
	Seconds float64
	Unit    string
}
