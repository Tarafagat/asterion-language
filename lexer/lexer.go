// Package lexer convierte código fuente de Asterion Language en tokens.
// La indentación es significativa (mismo criterio que Python, que es lo
// que se decidió en spec/grammar.md tras resolver el conflicto entre las
// dos sintaxis candidatas) — así que este lexer, a diferencia de uno para
// un lenguaje con llaves, también produce INDENT/DEDENT, no solo tokens
// léxicos planos. Tabs se rechazan explícitamente: mezclar tabs y espacios
// es la fuente de bugs de indentación más común en cualquier lenguaje que
// la usa como sintaxis, y no vale la pena la ambigüedad de adivinar cuánto
// vale un tab.
package lexer

import (
	"strconv"
	"strings"

	"github.com/Tarafagat/asterion-language/diagnostics"
)

type Lexer struct {
	src         []byte
	file        string
	pos         int // offset del próximo byte a leer
	line        int
	col         int
	indents     []int // pila de niveles de indentación, arranca en [0]
	parens      int   // profundidad de (), [] — >0 desactiva NEWLINE/INDENT/DEDENT
	atLineStart bool
	tokens      []Token
	diags       *diagnostics.Bag
}

// Lex tokeniza source por completo. Nunca para en el primer error léxico —
// junta todos los que pueda y sigue, para que 'asterion language check'
// pueda reportar más de un problema por corrida.
func Lex(source []byte, filename string) ([]Token, *diagnostics.Bag) {
	l := &Lexer{
		src:         source,
		file:        filename,
		line:        1,
		col:         1,
		indents:     []int{0},
		atLineStart: true,
		diags:       &diagnostics.Bag{},
	}
	l.run()
	return l.tokens, l.diags
}

func (l *Lexer) pos_() diagnostics.Position {
	return diagnostics.Position{File: l.file, Line: l.line, Col: l.col}
}

func (l *Lexer) peek() byte {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) peekAt(offset int) byte {
	if l.pos+offset >= len(l.src) {
		return 0
	}
	return l.src[l.pos+offset]
}

func (l *Lexer) advance() byte {
	c := l.src[l.pos]
	l.pos++
	if c == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return c
}

func (l *Lexer) emit(t TokenType, lit string, value any, pos diagnostics.Position) {
	l.tokens = append(l.tokens, Token{Type: t, Lit: lit, Value: value, Pos: pos})
}

func (l *Lexer) run() {
	for {
		if l.atLineStart && l.parens == 0 {
			if !l.handleIndentation() {
				break // EOF consumido dentro de handleIndentation
			}
		}
		if l.pos >= len(l.src) {
			break
		}
		c := l.peek()
		switch {
		case c == ' ':
			l.advance()
		case c == '\t':
			l.diags.Errorf(l.pos_(), "ASTR001", "tabs no están permitidos para indentación ni espaciado — usá espacios")
			l.advance()
		case c == '#':
			l.skipComment()
		case c == '\n':
			l.advance()
			if l.parens == 0 {
				l.emit(NEWLINE, "\n", nil, l.pos_())
				l.atLineStart = true
			}
		case c == '\r':
			l.advance()
		case isDigit(c):
			l.lexNumber()
		case isIdentStart(c):
			l.lexIdentOrKeyword()
		case c == '"' || c == '\'':
			l.lexString(c)
		default:
			l.lexSymbol()
		}
	}
	l.finish()
}

// handleIndentation mide la indentación al principio de una línea lógica y
// emite INDENT/DEDENT según corresponda contra la pila. Devuelve false si
// se llegó a EOF sin más contenido. Líneas en blanco o de solo comentario
// se saltan enteras — nunca generan un cambio de indentación.
func (l *Lexer) handleIndentation() bool {
	for {
		spaces := 0
		for l.peek() == ' ' {
			l.advance()
			spaces++
		}
		if l.peek() == '\t' {
			l.diags.Errorf(l.pos_(), "ASTR001", "tabs no están permitidos para indentación — usá espacios")
			// se sigue igual, tratando el tab como si no aportara indentación,
			// para no cortar el resto del archivo por un solo carácter.
		}
		if l.pos >= len(l.src) {
			l.atLineStart = false
			return false
		}
		c := l.peek()
		if c == '\n' || c == '#' || c == '\r' {
			// línea en blanco o de puro comentario: se consume y se reintenta
			// sin tocar la pila de indentación.
			if c == '#' {
				l.skipComment()
			}
			if l.peek() == '\r' {
				l.advance()
			}
			if l.peek() == '\n' {
				l.advance()
			}
			continue
		}
		l.atLineStart = false
		top := l.indents[len(l.indents)-1]
		switch {
		case spaces > top:
			l.indents = append(l.indents, spaces)
			l.emit(INDENT, "", spaces, l.pos_())
		case spaces < top:
			for len(l.indents) > 1 && l.indents[len(l.indents)-1] > spaces {
				l.indents = l.indents[:len(l.indents)-1]
				l.emit(DEDENT, "", nil, l.pos_())
			}
			if l.indents[len(l.indents)-1] != spaces {
				l.diags.Errorf(l.pos_(), "ASTR002", "indentación inconsistente — no coincide con ningún nivel anterior")
				l.indents[len(l.indents)-1] = spaces
			}
		}
		return true
	}
}

func (l *Lexer) skipComment() {
	for l.pos < len(l.src) && l.peek() != '\n' {
		l.advance()
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isIdentCont(c byte) bool { return isIdentStart(c) || isDigit(c) }

var sizeUnits = map[string]int64{
	"B":  1,
	"KB": 1024,
	"MB": 1024 * 1024,
	"GB": 1024 * 1024 * 1024,
	"TB": 1024 * 1024 * 1024 * 1024,
}

var durationUnits = map[string]float64{
	"s": 1,
	"m": 60,
	"h": 3600,
}

// lexNumber lee un literal numérico y decide, por lo que viene después de
// los dígitos, si es INT, FLOAT, SIZE ("8GB") o DURATION ("30s") — las
// unidades son parte del token, no un identificador aparte, para que el
// parser nunca tenga que "adivinar" que "GB" pegado a un número es una
// unidad y no una llamada.
func (l *Lexer) lexNumber() {
	startPos := l.pos_()
	start := l.pos
	for isDigit(l.peek()) {
		l.advance()
	}
	isFloat := false
	if l.peek() == '.' && isDigit(l.peekAt(1)) {
		isFloat = true
		l.advance()
		for isDigit(l.peek()) {
			l.advance()
		}
	}
	numLit := string(l.src[start:l.pos])

	// unidad de tamaño: dígitos seguidos directo de letras mayúsculas (GB, MB...)
	if isIdentStart(l.peek()) {
		unitStart := l.pos
		for isIdentCont(l.peek()) {
			l.advance()
		}
		unit := string(l.src[unitStart:l.pos])
		if mult, ok := sizeUnits[unit]; ok {
			n, _ := strconv.ParseFloat(numLit, 64)
			l.emit(SIZE, numLit+unit, SizeValue{Bytes: int64(n * float64(mult)), Unit: unit}, startPos)
			return
		}
		if mult, ok := durationUnits[unit]; ok {
			n, _ := strconv.ParseFloat(numLit, 64)
			l.emit(DURATION, numLit+unit, DurationValue{Seconds: n * mult, Unit: unit}, startPos)
			return
		}
		l.diags.Errorf(startPos, "ASTR003", "unidad %q no reconocida — tamaños válidos: B/KB/MB/GB/TB, duraciones válidas: s/m/h", unit)
		l.emit(INT, numLit, int64(0), startPos)
		return
	}

	if isFloat {
		f, _ := strconv.ParseFloat(numLit, 64)
		l.emit(FLOAT, numLit, f, startPos)
		return
	}
	n, err := strconv.ParseInt(numLit, 10, 64)
	if err != nil {
		l.diags.Errorf(startPos, "ASTR004", "número inválido %q", numLit)
	}
	l.emit(INT, numLit, n, startPos)
}

func (l *Lexer) lexIdentOrKeyword() {
	startPos := l.pos_()
	start := l.pos
	for isIdentCont(l.peek()) {
		l.advance()
	}
	lit := string(l.src[start:l.pos])
	if kw, ok := keywords[lit]; ok {
		var value any
		if kw == TRUE {
			value = true
		} else if kw == FALSE {
			value = false
		}
		l.emit(kw, lit, value, startPos)
		return
	}
	l.emit(IDENT, lit, nil, startPos)
}

func (l *Lexer) lexString(quote byte) {
	startPos := l.pos_()
	l.advance() // comilla de apertura
	var sb strings.Builder
	for {
		if l.pos >= len(l.src) {
			l.diags.Errorf(startPos, "ASTR005", "string sin cerrar")
			break
		}
		c := l.peek()
		if c == quote {
			l.advance()
			break
		}
		if c == '\n' {
			l.diags.Errorf(startPos, "ASTR005", "string sin cerrar antes de fin de línea")
			break
		}
		if c == '\\' {
			l.advance()
			esc := l.peek()
			switch esc {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case '\\', '"', '\'':
				sb.WriteByte(esc)
			default:
				sb.WriteByte('\\')
				sb.WriteByte(esc)
			}
			l.advance()
			continue
		}
		sb.WriteByte(c)
		l.advance()
	}
	l.emit(STRING, sb.String(), sb.String(), startPos)
}

func (l *Lexer) lexSymbol() {
	pos := l.pos_()
	c := l.advance()
	switch c {
	case '(':
		l.parens++
		l.emit(LPAREN, "(", nil, pos)
	case ')':
		if l.parens > 0 {
			l.parens--
		}
		l.emit(RPAREN, ")", nil, pos)
	case '[':
		l.parens++
		l.emit(LBRACKET, "[", nil, pos)
	case ']':
		if l.parens > 0 {
			l.parens--
		}
		l.emit(RBRACKET, "]", nil, pos)
	case ',':
		l.emit(COMMA, ",", nil, pos)
	case '.':
		l.emit(DOT, ".", nil, pos)
	case '=':
		l.emit(ASSIGN, "=", nil, pos)
	case ':':
		l.emit(COLON, ":", nil, pos)
	case '?':
		l.emit(QUESTION, "?", nil, pos)
	default:
		l.diags.Errorf(pos, "ASTR006", "carácter inesperado %q", string(c))
	}
}

// finish cierra el archivo: si la última línea no terminó en NEWLINE se
// agrega uno (para que el parser nunca tenga que tratar "fin de archivo"
// como un caso distinto de "fin de línea"), y se emite un DEDENT por cada
// nivel de indentación que haya quedado abierto.
func (l *Lexer) finish() {
	if n := len(l.tokens); n > 0 && l.tokens[n-1].Type != NEWLINE {
		l.emit(NEWLINE, "", nil, l.pos_())
	}
	for len(l.indents) > 1 {
		l.indents = l.indents[:len(l.indents)-1]
		l.emit(DEDENT, "", nil, l.pos_())
	}
	l.emit(EOF, "", nil, l.pos_())
}
