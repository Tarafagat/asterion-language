// Package parser arma el AST a partir de los tokens que produce lexer. Es
// descenso recursivo de mano, sin generador de parsers — la gramática de
// v0.1 (ver spec/grammar.md) es chica a propósito, así que no hace falta
// la maquinaria de un parser generator para algo que un puñado de
// funciones recursivas resuelve con claridad.
package parser

import (
	"fmt"

	"github.com/Tarafagat/asterion-language/ast"
	"github.com/Tarafagat/asterion-language/diagnostics"
	"github.com/Tarafagat/asterion-language/lexer"
)

type Parser struct {
	toks  []lexer.Token
	pos   int
	file  string
	diags *diagnostics.Bag
}

// parseError es el tipo interno que synchronize() atrapa — nunca escapa de
// Parse(). Permite que una función de parseo profundamente anidada aborte
// el statement actual sin que cada nivel intermedio tenga que propagar un
// error a mano.
type parseError struct{}

// Parse tokeniza y parsea source, devolviendo el Program y todos los
// diagnósticos (léxicos + sintácticos) juntos, en el orden en que
// ocurrieron.
func Parse(source []byte, filename string) (*ast.Program, *diagnostics.Bag) {
	toks, lexDiags := lexer.Lex(source, filename)
	p := &Parser{toks: toks, file: filename, diags: &diagnostics.Bag{}}

	merged := &diagnostics.Bag{}
	for _, d := range lexDiags.Items() {
		merged.Add(d)
	}

	prog := p.parseProgram()

	for _, d := range p.diags.Items() {
		merged.Add(d)
	}
	return prog, merged
}

func (p *Parser) cur() lexer.Token { return p.toks[p.pos] }
func (p *Parser) at(o int) lexer.Token {
	i := p.pos + o
	if i >= len(p.toks) {
		return p.toks[len(p.toks)-1] // EOF
	}
	return p.toks[i]
}
func (p *Parser) advance() lexer.Token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *Parser) errorf(pos diagnostics.Position, code, format string, args ...any) {
	p.diags.Errorf(pos, code, format, args...)
}

// expect consume el token actual si coincide con tt; si no, reporta
// ASTR100 y aborta el statement actual vía panic(parseError{}) —
// synchronize() en parseStmt lo atrapa y sigue con la próxima línea.
func (p *Parser) expect(tt lexer.TokenType) lexer.Token {
	if p.cur().Type != tt {
		p.errorf(p.cur().Pos, "ASTR100", "se esperaba %s, encontré %s", tt, describeTok(p.cur()))
		panic(parseError{})
	}
	return p.advance()
}

func describeTok(t lexer.Token) string {
	if t.Type == lexer.IDENT || t.Type == lexer.STRING {
		return fmt.Sprintf("%s (%q)", t.Type, t.Lit)
	}
	return t.Type.String()
}

// --- Programa -------------------------------------------------------------

func (p *Parser) parseProgram() *ast.Program {
	prog := &ast.Program{Pos: p.cur().Pos}

	// pragma opcional: language "0.1" — se reconoce por forma (IDENT
	// "language" seguido de STRING), no es una palabra reservada porque no
	// tiene sentido en ningún otro lugar de la gramática.
	if p.cur().Type == lexer.IDENT && p.cur().Lit == "language" && p.at(1).Type == lexer.STRING {
		p.advance()
		v := p.advance()
		prog.LanguageVersion = v.Lit
		if p.cur().Type == lexer.NEWLINE {
			p.advance()
		}
	}

	for p.cur().Type != lexer.EOF {
		if p.cur().Type == lexer.NEWLINE {
			p.advance()
			continue
		}
		stmt := p.parseStmtRecovering()
		if stmt != nil {
			prog.Statements = append(prog.Statements, stmt)
		}
	}
	return prog
}

// parseStmtRecovering envuelve parseStmt con recover(): si algo dentro
// hace panic(parseError{}), se descarta el statement roto y se avanza
// hasta la próxima línea para poder seguir parseando el resto del
// archivo — el objetivo es que un solo typo no oculte todos los demás
// errores reales que haya más abajo.
func (p *Parser) parseStmtRecovering() (stmt ast.Stmt) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(parseError); ok {
				p.synchronize()
				stmt = nil
				return
			}
			panic(r)
		}
	}()
	return p.parseStmt()
}

func (p *Parser) synchronize() {
	for p.cur().Type != lexer.EOF && p.cur().Type != lexer.NEWLINE && p.cur().Type != lexer.DEDENT {
		p.advance()
	}
	if p.cur().Type == lexer.NEWLINE {
		p.advance()
	}
}

func (p *Parser) parseStmt() ast.Stmt {
	switch p.cur().Type {
	case lexer.DEF:
		return p.parseFuncDecl()
	case lexer.RETURN:
		return p.parseReturnStmt()
	case lexer.IDENT:
		if p.at(1).Type == lexer.ASSIGN {
			return p.parseAssignStmt()
		}
		return p.parseExprStmt()
	default:
		return p.parseExprStmt()
	}
}

func (p *Parser) parseBlock() []ast.Stmt {
	p.expect(lexer.NEWLINE)
	p.expect(lexer.INDENT)
	var stmts []ast.Stmt
	for p.cur().Type != lexer.DEDENT && p.cur().Type != lexer.EOF {
		if p.cur().Type == lexer.NEWLINE {
			p.advance()
			continue
		}
		s := p.parseStmtRecovering()
		if s != nil {
			stmts = append(stmts, s)
		}
	}
	if p.cur().Type == lexer.DEDENT {
		p.advance()
	} else {
		p.errorf(p.cur().Pos, "ASTR101", "bloque sin cerrar — esperaba que la indentación volviera a un nivel anterior")
	}
	return stmts
}

func (p *Parser) parseFuncDecl() ast.Stmt {
	pos := p.cur().Pos
	p.expect(lexer.DEF)
	name := p.expect(lexer.IDENT).Lit
	p.expect(lexer.LPAREN)
	var params []string
	for p.cur().Type != lexer.RPAREN {
		params = append(params, p.expect(lexer.IDENT).Lit)
		if p.cur().Type == lexer.COMMA {
			p.advance()
		}
	}
	p.expect(lexer.RPAREN)
	p.expect(lexer.COLON)
	body := p.parseBlock()
	return &ast.FuncDecl{Name: name, Params: params, Body: body, Pos: pos}
}

func (p *Parser) parseReturnStmt() ast.Stmt {
	pos := p.cur().Pos
	p.expect(lexer.RETURN)
	var values []ast.Expr
	if p.cur().Type != lexer.NEWLINE && p.cur().Type != lexer.EOF {
		values = append(values, p.parseExpr())
		for p.cur().Type == lexer.COMMA {
			p.advance()
			values = append(values, p.parseExpr())
		}
	}
	p.expectStmtEnd()
	return &ast.ReturnStmt{Values: values, Pos: pos}
}

func (p *Parser) parseAssignStmt() ast.Stmt {
	pos := p.cur().Pos
	name := p.expect(lexer.IDENT).Lit
	p.expect(lexer.ASSIGN)
	value := p.parseExpr()
	p.expectStmtEnd()
	return &ast.AssignStmt{Name: name, Value: value, Pos: pos}
}

func (p *Parser) parseExprStmt() ast.Stmt {
	pos := p.cur().Pos
	x := p.parseExpr()
	p.expectStmtEnd()
	return &ast.ExprStmt{X: x, Pos: pos}
}

// expectStmtEnd cierra un statement — NEWLINE normalmente, pero también se
// acepta EOF/DEDENT directo para la última línea de un archivo o bloque
// que no terminó con salto de línea explícito.
func (p *Parser) expectStmtEnd() {
	switch p.cur().Type {
	case lexer.NEWLINE:
		p.advance()
	case lexer.EOF, lexer.DEDENT:
		// nada que consumir
	default:
		p.errorf(p.cur().Pos, "ASTR102", "se esperaba fin de línea, encontré %s", describeTok(p.cur()))
		panic(parseError{})
	}
}

// --- Expresiones ------------------------------------------------------

func (p *Parser) parseExpr() ast.Expr {
	x := p.parsePostfix()
	if p.cur().Type == lexer.QUESTION {
		// Propagación de error (spec/errors.md) — se lexea y se acepta
		// sintácticamente desde v0.1; el semantic analyzer todavía no le
		// da significado (ver semantic/README de la especificación).
		p.advance()
	}
	return x
}

func (p *Parser) parsePostfix() ast.Expr {
	x := p.parsePrimary()
	for {
		switch p.cur().Type {
		case lexer.DOT:
			pos := p.cur().Pos
			p.advance()
			name := p.expect(lexer.IDENT).Lit
			x = &ast.AttrExpr{X: x, Name: name, Pos: pos}
		case lexer.LPAREN:
			pos := p.cur().Pos
			p.advance()
			args := p.parseArgs()
			p.expect(lexer.RPAREN)
			x = &ast.CallExpr{Callee: x, Args: args, Pos: pos}
		default:
			return x
		}
	}
}

func (p *Parser) parseArgs() []ast.Arg {
	var args []ast.Arg
	for p.cur().Type != lexer.RPAREN {
		if p.cur().Type == lexer.IDENT && p.at(1).Type == lexer.ASSIGN {
			name := p.advance().Lit
			p.advance() // "="
			args = append(args, ast.Arg{Name: name, Value: p.parseExpr()})
		} else {
			args = append(args, ast.Arg{Value: p.parseExpr()})
		}
		if p.cur().Type == lexer.COMMA {
			p.advance()
		} else {
			break
		}
	}
	return args
}

func (p *Parser) parsePrimary() ast.Expr {
	t := p.cur()
	switch t.Type {
	case lexer.IDENT:
		p.advance()
		return &ast.Ident{Name: t.Lit, Pos: t.Pos}
	case lexer.STRING:
		p.advance()
		return &ast.StringLit{Value: t.Value.(string), Pos: t.Pos}
	case lexer.INT:
		p.advance()
		return &ast.IntLit{Value: t.Value.(int64), Pos: t.Pos}
	case lexer.FLOAT:
		p.advance()
		return &ast.FloatLit{Value: t.Value.(float64), Pos: t.Pos}
	case lexer.SIZE:
		p.advance()
		sv := t.Value.(lexer.SizeValue)
		return &ast.SizeLit{Bytes: sv.Bytes, Unit: sv.Unit, Pos: t.Pos}
	case lexer.DURATION:
		p.advance()
		dv := t.Value.(lexer.DurationValue)
		return &ast.DurationLit{Seconds: dv.Seconds, Unit: dv.Unit, Pos: t.Pos}
	case lexer.TRUE:
		p.advance()
		return &ast.BoolLit{Value: true, Pos: t.Pos}
	case lexer.FALSE:
		p.advance()
		return &ast.BoolLit{Value: false, Pos: t.Pos}
	case lexer.LBRACKET:
		p.advance()
		var elems []ast.Expr
		for p.cur().Type != lexer.RBRACKET {
			elems = append(elems, p.parseExpr())
			if p.cur().Type == lexer.COMMA {
				p.advance()
			} else {
				break
			}
		}
		p.expect(lexer.RBRACKET)
		return &ast.ListLit{Elements: elems, Pos: t.Pos}
	case lexer.LPAREN:
		p.advance()
		x := p.parseExpr()
		p.expect(lexer.RPAREN)
		return x
	default:
		p.errorf(t.Pos, "ASTR103", "se esperaba una expresión, encontré %s", describeTok(t))
		panic(parseError{})
	}
}
