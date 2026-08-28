// Package ast define los nodos del árbol de sintaxis de Asterion Language.
// Deliberadamente chico y sin lógica propia (ni siquiera un Visitor
// genérico): cada nodo es un struct de datos, y quien lo recorre (el
// semantic analyzer, hoy; algo que compile hacia LabSpec/APC/un
// ProvisioningRequest más adelante) decide cómo. Ver
// asterion-plugin-contract/spec/apc-v1.md por el mismo criterio de "el
// dato manda, la interpretación vive afuera" aplicado ahí a plugin.yaml.
package ast

import "github.com/Tarafagat/asterion-language/diagnostics"

type Node interface {
	Position() diagnostics.Position
}

// Program es la raíz: todo archivo .ast parsea a uno de estos.
// LanguageVersion viene del pragma `language "0.1"` (spec/grammar.md §18)
// — vacío si el archivo no lo declaró, que el semantic analyzer trata como
// "asumí la versión que este compilador entiende", igual criterio que
// contract_version en el Asterion Plugin Contract.
type Program struct {
	LanguageVersion string
	Statements      []Stmt
	Pos             diagnostics.Position
}

func (p *Program) Position() diagnostics.Position { return p.Pos }

// --- Statements ---------------------------------------------------------

type Stmt interface {
	Node
	stmtNode()
}

// AssignStmt es `name = expr`. En Asterion Language esto nunca ejecuta
// nada — si expr es una llamada de recurso (Provider.aws.instance(...)),
// AssignStmt solo le pone un nombre lógico al nodo de recurso deseado que
// esa llamada produce (ver spec/resources.md).
type AssignStmt struct {
	Name  string
	Value Expr
	Pos   diagnostics.Position
}

func (s *AssignStmt) stmtNode()                      {}
func (s *AssignStmt) Position() diagnostics.Position { return s.Pos }

// FuncDecl es `def name(params):` seguido de un bloque indentado. No hay
// funciones anónimas, closures, ni funciones de primera clase en v0.1 —
// son subrutinas planas, deliberadamente.
type FuncDecl struct {
	Name   string
	Params []string
	Body   []Stmt
	Pos    diagnostics.Position
}

func (s *FuncDecl) stmtNode()                      {}
func (s *FuncDecl) Position() diagnostics.Position { return s.Pos }

// ReturnStmt soporta múltiples valores (`return a, b`) para el patrón
// `value, error = operation()` de spec/errors.md.
type ReturnStmt struct {
	Values []Expr
	Pos    diagnostics.Position
}

func (s *ReturnStmt) stmtNode()                      {}
func (s *ReturnStmt) Position() diagnostics.Position { return s.Pos }

// ExprStmt es una expresión usada como statement completo — ej. una
// llamada cuyo resultado nadie asigna a un nombre.
type ExprStmt struct {
	X   Expr
	Pos diagnostics.Position
}

func (s *ExprStmt) stmtNode()                      {}
func (s *ExprStmt) Position() diagnostics.Position { return s.Pos }

// --- Expressions ---------------------------------------------------------

type Expr interface {
	Node
	exprNode()
}

type Ident struct {
	Name string
	Pos  diagnostics.Position
}

func (e *Ident) exprNode()                      {}
func (e *Ident) Position() diagnostics.Position { return e.Pos }

type StringLit struct {
	Value string
	Pos   diagnostics.Position
}

func (e *StringLit) exprNode()                      {}
func (e *StringLit) Position() diagnostics.Position { return e.Pos }

type IntLit struct {
	Value int64
	Pos   diagnostics.Position
}

func (e *IntLit) exprNode()                      {}
func (e *IntLit) Position() diagnostics.Position { return e.Pos }

type FloatLit struct {
	Value float64
	Pos   diagnostics.Position
}

func (e *FloatLit) exprNode()                      {}
func (e *FloatLit) Position() diagnostics.Position { return e.Pos }

type BoolLit struct {
	Value bool
	Pos   diagnostics.Position
}

func (e *BoolLit) exprNode()                      {}
func (e *BoolLit) Position() diagnostics.Position { return e.Pos }

// SizeLit es un literal como 8GB — BytesValue ya viene normalizado a bytes
// desde el lexer (ver lexer.SizeValue), Unit es la unidad tal como la
// escribió el usuario, para poder reportarla en mensajes de error sin
// tener que reconstruirla.
type SizeLit struct {
	Bytes int64
	Unit  string
	Pos   diagnostics.Position
}

func (e *SizeLit) exprNode()                      {}
func (e *SizeLit) Position() diagnostics.Position { return e.Pos }

// DurationLit es un literal como 30s — Seconds ya normalizado.
type DurationLit struct {
	Seconds float64
	Unit    string
	Pos     diagnostics.Position
}

func (e *DurationLit) exprNode()                      {}
func (e *DurationLit) Position() diagnostics.Position { return e.Pos }

// ListLit es `[a, b, c]`.
type ListLit struct {
	Elements []Expr
	Pos      diagnostics.Position
}

func (e *ListLit) exprNode()                      {}
func (e *ListLit) Position() diagnostics.Position { return e.Pos }

// AttrExpr es `X.Name` — ej. el `Provider.aws` de `Provider.aws.instance(...)`,
// o el `web.public_ip` de un Output leído después de un recurso.
type AttrExpr struct {
	X    Expr
	Name string
	Pos  diagnostics.Position
}

func (e *AttrExpr) exprNode()                      {}
func (e *AttrExpr) Position() diagnostics.Position { return e.Pos }

// Arg es un argumento de CallExpr — Name vacío significa posicional. En
// v0.1 la convención de la especificación es siempre nombrado
// (`cpu=4, memory=8GB`), pero el parser soporta ambos.
type Arg struct {
	Name  string
	Value Expr
}

// CallExpr es `Callee(args...)`. Es la única forma en que un recurso se
// declara (spec/resources.md) — Provider.aws.instance(...), Lab.vm(...),
// Network(...) son todos, sintácticamente, un CallExpr; qué significan
// semánticamente (crear un recurso vs. una llamada normal) lo decide el
// semantic analyzer mirando la forma de Callee, no el parser.
type CallExpr struct {
	Callee Expr
	Args   []Arg
	Pos    diagnostics.Position
}

func (e *CallExpr) exprNode()                      {}
func (e *CallExpr) Position() diagnostics.Position { return e.Pos }
