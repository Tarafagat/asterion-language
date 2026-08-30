# Gramática — Asterion Language v0.1

Refleja exactamente lo que `lexer`/`parser` implementan hoy — si algo acá
y el código alguna vez difieren, el código manda.

## Léxico

```
IDENT      := [_a-zA-Z][_a-zA-Z0-9]*
STRING     := '"' ... '"'  |  "'" ... "'"      (con \n \t \\ \" \')
INT        := [0-9]+
FLOAT      := [0-9]+ '.' [0-9]+
SIZE       := [0-9]+('.'[0-9]+)? ('B'|'KB'|'MB'|'GB'|'TB')
DURATION   := [0-9]+('.'[0-9]+)? ('s'|'m'|'h')
COMMENT    := '#' hasta fin de línea (no genera token)
```

Palabras clave: `def`, `return`, `true`, `false`. No hay `if`/`for`/`while`/
`class`/`import` en v0.1.

Símbolos: `( ) [ ] , . = : ?`

La indentación es significativa (mismo criterio que Python): tabs están
prohibidos, solo espacios. Dentro de `(...)`/`[...]` los saltos de línea no
generan `NEWLINE` — se pueden partir argumentos en varias líneas
libremente.

## Sintaxis

```ebnf
program        := [ language_pragma ] { stmt } EOF
language_pragma := "language" STRING NEWLINE

stmt           := func_decl | assign_stmt | return_stmt | expr_stmt

func_decl      := "def" IDENT "(" [ params ] ")" ":" block
params         := IDENT { "," IDENT }
block          := NEWLINE INDENT { stmt } DEDENT

assign_stmt    := IDENT "=" expr stmt_end
return_stmt    := "return" [ expr { "," expr } ] stmt_end
expr_stmt      := expr stmt_end
stmt_end       := NEWLINE | EOF | DEDENT

expr           := postfix_expr [ "?" ]
postfix_expr   := primary { "." IDENT | "(" args ")" }
args           := [ arg { "," arg } ]
arg            := ( IDENT "=" expr ) | expr

primary        := IDENT | STRING | INT | FLOAT | SIZE | DURATION
                 | "true" | "false"
                 | "[" [ expr { "," expr } ] "]"
                 | "(" expr ")"
```

## Semántica de v0.1 (lo que el analyzer valida hoy)

1. **Declarar antes de usar.** Un `IDENT` en una expresión debe referirse
   a un parámetro, una función, o una asignación anterior en el mismo
   scope o uno que lo contenga — nunca hacia adelante. Ver el README para
   por qué esto es una simplificación deliberada, no un descuido.
2. **Nombres de recurso únicos por scope.** Si `X = expr` y `expr` tiene
   forma de recurso (`Provider.*`, `Lab.*`, o un constructor genérico como
   `Network(...)`), reasignar `X` en el mismo scope es un error
   (`ASTR201`) — un recurso no se redeclara.
3. **`Provider.<code>.<método>(...)` se valida contra capabilities
   reales**: `<code>` debe ser un provider que el Registry de
   `asterion-core` reconoce (`ASTR210`); `<método>` debe ser uno de
   `instance|network|database|bucket` — los únicos que
   `adapters.ProviderAdapter` expone hoy (`ASTR211`); y el provider debe
   declarar la capability que ese método requiere (`ASTR212`, con el
   detalle de qué capabilities sí tiene disponibles).
4. **`Lab.*` y los constructores genéricos** (`Network`, `Instance`,
   `Storage`, `Image`, `Container`) se reconocen como recursos (participan
   de la regla 2) pero no llevan chequeo de capability propio — Lab valida
   su propio spec más adelante, en la etapa de traducción hacia `LabSpec`
   (todavía no implementada).
5. **`Plugin.*`** parsea y resuelve referencias, sin ninguna validación
   semántica especial — la sintaxis de recursos de plugin es PLANNED.

## Códigos de diagnóstico

| Código | Etapa | Significado |
|---|---|---|
| ASTR001 | lexer | tab usado para indentación/espaciado |
| ASTR002 | lexer | indentación no coincide con ningún nivel anterior |
| ASTR003 | lexer | unidad de tamaño/duración no reconocida |
| ASTR004 | lexer | literal numérico inválido |
| ASTR005 | lexer | string sin cerrar |
| ASTR006 | lexer | carácter inesperado |
| ASTR100 | parser | token inesperado |
| ASTR101 | parser | bloque sin cerrar |
| ASTR102 | parser | falta fin de línea |
| ASTR103 | parser | se esperaba una expresión |
| ASTR200 | semantic | nombre ya declarado en este scope (función) |
| ASTR201 | semantic | nombre de recurso ya declarado en este scope |
| ASTR202 | semantic | referencia a algo no definido |
| ASTR210 | semantic | provider no reconocido |
| ASTR211 | semantic | método de recurso no existe en `ProviderAdapter` |
| ASTR212 | semantic | provider no declara la capability requerida |
| ASTR300 | pluginmanifest | el statement no es una llamada `Contract.<verbo>(...)` |
| ASTR301 | pluginmanifest | `Contract.<verbo>` no existe |
| ASTR302 | pluginmanifest | falta un argumento obligatorio en una llamada `Contract.*` |
| ASTR303 | pluginmanifest | un argumento tiene el tipo equivocado (ej. se esperaba una lista) |
| ASTR304 | pluginmanifest | un verbo de "una sola vez" (`define`/`start`/...) se llamó más de una vez |
| ASTR305 | pluginmanifest | el archivo nunca llamó `Contract.define(...)` |

## DSL de manifiesto de plugin (`Contract.*`)

Un segundo uso de la misma gramática, separado del modelo de
infraestructura de arriba: en vez de describir qué crear (`Provider.*`,
`Lab.*`), un archivo de este tipo describe el contrato de un plugin nuevo
— compila a un `plugin.yaml` (Asterion Plugin Contract) vía
`asterion plugin from-asterion <archivo.asterion> --out <dir>`.

Es sintácticamente el mismo `.asterion` (mismo lexer, mismo parser) pero
semánticamente distinto: una secuencia plana de llamadas
`Contract.<verbo>(clave=valor, ...)`, sin `def`, sin asignaciones, sin
`Provider.*`/`Lab.*`/`Plugin.*` — el compilador de este DSL
(`asterion-language/pluginmanifest`) es un walker propio, no pasa por el
`semantic.Analyzer` de infraestructura de arriba. `Contract` es un builtin
nuevo, deliberadamente distinto de `Plugin` (que sigue reservado para
*usar* un plugin ya instalado desde un `.asterion` de infraestructura — ver
`examples/plugin.asterion`).

Ejemplo completo: `examples/plugin-manifest.asterion`.

| Verbo | Cardinalidad | Campo de `apc.Manifest` |
|---|---|---|
| `Contract.define(name, version, description?, author?, license?, repo?)` | una vez, obligatorio | Name/Version/Description/Author/License/Repo |
| `Contract.language(name, version?)` | una vez | Language |
| `Contract.start(command, port?, args?)` | una vez, obligatorio | Start, Port |
| `Contract.health_path(path)` | una vez | HealthPath |
| `Contract.api(base_path?, openapi?)` | una vez | API |
| `Contract.permissions(network?, filesystem?, database?, secrets?)` | una vez | Permissions |
| `Contract.events(publishes?, subscribes?)` | una vez | Events |
| `Contract.config(key, label?, type?, secret?, required?, default?)` | repetible | append a ConfigSchema |
| `Contract.resource(name, endpoint, schema?, primary_key?, crud?)` | repetible | append a Resources |
| `Contract.action(name, method, endpoint, description?)` | repetible | append a Actions |

Todos los argumentos van nombrados. `asterion plugin from-asterion` corre
`apc.Manifest.Validate()` sobre el resultado antes de escribirlo — el
compilador de este DSL solo traduce sintaxis a datos, nunca duplica esas
reglas.
