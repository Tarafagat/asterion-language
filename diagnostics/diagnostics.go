// Package diagnostics es la forma única de error de todo el compilador —
// lexer, parser y semantic analyzer producen todos el mismo tipo
// Diagnostic, nunca un error de Go genérico envuelto a mano distinto en
// cada capa. El formato (código ASTR-xxx + posición + mensaje + detalle)
// es el que ya fijó la especificación de Asterion Language antes de que
// existiera una sola línea de este paquete — ver spec/errors.md.
package diagnostics

import (
	"fmt"
	"strings"
)

// Position es dónde, en el archivo fuente, ocurrió el diagnóstico. Line y
// Col son 1-indexados (como los reporta cualquier editor), no 0-indexados
// como los índices internos del lexer.
type Position struct {
	File string
	Line int
	Col  int
}

func (p Position) String() string {
	if p.File == "" {
		return fmt.Sprintf("%d:%d", p.Line, p.Col)
	}
	return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Col)
}

// Severity distingue un error (el programa no compila) de un warning (compila,
// pero hay algo que vale la pena señalar) — hoy solo se emiten errores, pero
// el campo existe desde ahora para no tener que romper el formato después.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Diagnostic es un único hallazgo del compilador. Code es el identificador
// estable "ASTRnnn" (sin guion, para poder usarlo como anchor de
// documentación) — Message es la primera línea, Detail es el bloque
// explicativo opcional que sigue (capabilities faltantes, sugerencias, etc).
type Diagnostic struct {
	Severity Severity
	Code     string
	Pos      Position
	Message  string
	Detail   string
}

func (d Diagnostic) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: %s\n", strings.ToUpper(string(d.Severity)), d.Code, d.Message)
	fmt.Fprintf(&b, "  --> %s\n", d.Pos)
	if d.Detail != "" {
		b.WriteString("\n")
		b.WriteString(d.Detail)
		b.WriteString("\n")
	}
	return b.String()
}

// Bag junta todos los diagnósticos de una corrida — el compilador nunca
// para en el primer error salvo que sea irrecuperable (ver lexer/parser):
// reportar 3 errores de una es mejor experiencia que hacer que el usuario
// corra el compilador 3 veces.
type Bag struct {
	items []Diagnostic
}

func (b *Bag) Add(d Diagnostic) { b.items = append(b.items, d) }

func (b *Bag) Errorf(pos Position, code, format string, args ...any) {
	b.Add(Diagnostic{Severity: SeverityError, Code: code, Pos: pos, Message: fmt.Sprintf(format, args...)})
}

func (b *Bag) HasErrors() bool {
	for _, d := range b.items {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (b *Bag) Items() []Diagnostic { return b.items }

func (b *Bag) String() string {
	var sb strings.Builder
	for i, d := range b.items {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(d.Error())
	}
	return sb.String()
}
