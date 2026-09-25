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
6. **`System.*`** no pasa por estas reglas en absoluto — es un DSL
   separado (mismo lexer/parser, su propio compilador chico,
   `systemspec.Compile`, que nunca corre `semantic.Analyzer`), igual que
   `Contract.*` más abajo. Ver la sección "DSL de sistema de plugins"
   para sus propias reglas.

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
| ASTR500 | systemspec | un nombre de plugin (`db = System.plugin(...)`) ya fue declarado antes en el archivo |
| ASTR501 | systemspec | falta un argumento obligatorio en `System.plugin`/`System.wire` |
| ASTR502 | systemspec | un argumento tiene el tipo equivocado (se esperaba string/booleano/lista) |
| ASTR503 | systemspec | `to`/`from` de `System.wire` no es una referencia (`Ident`) a un plugin |
| ASTR504 | systemspec | `to`/`from` referencia un nombre que no fue declarado con `System.plugin(...)` antes de esa línea |

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
| `Contract.language(name, version?, venv?, requirements?)` | una vez | Language |
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

`venv`/`requirements` (ambos opcionales) le dicen a un plugin Python
DÓNDE viven su virtualenv y su `requirements.txt`, relativos a la raíz
del plugin (ej. `venv="backend/venv"`,
`requirements="backend/requirements.txt"`) — sin declararlos, Asterion
sigue infiriendo ambas rutas por convención a partir de `start.command`
(ej. `start.command="./backend/venv/bin/python"` ⇒ venv en
`backend/venv`, requirements en `backend/requirements.txt`), exactamente
como funcionaba antes de que existiera la forma explícita. Solo tienen
efecto cuando `language.name == "python"` — `asterion plugin build`
(`asterion-core`) es quien los usa para decidir dónde crear el venv (si
no existe) y qué instalar con `pip install -r`.

## DSL de sistema de plugins (`System.*`)

Un tercer uso de la misma gramática: en vez de describir infraestructura
(`Provider.*`/`Lab.*`) o el contrato de UN plugin (`Contract.*`), un
archivo de este tipo declara un SISTEMA de varios plugins YA
instalables, conectados entre sí. Compila con
`systemspec.Compile` (`asterion-language/systemspec`) — otro walker
propio, tampoco pasa por `semantic.Analyzer` ni por `pluginmanifest`.
`asterion-core` lo consume desde `asterion plugin system
apply/export/watch/watch-install/watch-uninstall`.

Es el primer compilador de este repo que resuelve una referencia real
entre dos statements del mismo archivo: en `System.wire(to=api,
from=db, ...)`, `api`/`db` deben ser identificadores que refieren a un
`System.plugin(...)` asignado antes en el archivo (misma regla 1 de
"declarar antes de usar" de la sección de semántica de arriba, pero acá
sí se resuelve y usa, no solo se valida y descarta — ver ASTR503/ASTR504).

Ejemplo completo: `examples/tutorial-system.asterion` (documentado en
`docs/TUTORIAL.md` § 7).

| Verbo | Cardinalidad | Descripción |
|---|---|---|
| `System.plugin(route, ref?, requires?, principal?)` | repetible, asignado a una variable (`nombre = System.plugin(...)`) | Declara un plugin del sistema. `route`: carpeta local o URL de git (heurística de `asterion-core`, no de este compilador). `ref`: branch/tag/commit (vacío = HEAD del default). `requires`: lista de strings — toolchains que el plugin necesita antes de compilarse (`"node@<versión exacta>"` para un Node sandboxed y verificado por checksum; `"python"`/`"go"` para solo verificar que ya están en el PATH; cualquier otra cosa es rechazada explícitamente — ver `docs/TUTORIAL.md` § 7). `principal`: marca el plugin central del sistema (mismo campo `IsMain` que ya usa `asterion local tunnel start`). |
| `System.wire(to, key, from, field?)` | repetible, statement suelto (sin asignación) | Antes de arrancar `to`, fija su config `key` a partir de un campo de `from` resuelto en runtime — nunca un valor literal. `to`/`from`: referencias (`Ident`) a plugins ya declarados con `System.plugin(...)` más arriba (ASTR503 si no es una referencia, ASTR504 si el nombre no fue declarado antes). `field` (default `"port"`): `"port"` usa la URL real donde `from` terminó escuchando; `"env:<clave>"` copia una config YA GUARDADA de `from` (falla en runtime, no acá, si `from` no la tiene configurada). |

Todos los argumentos van nombrados, igual que en `Contract.*`. Los
mensajes de runtime (arrancar cada plugin, resolver cada wire) no son
responsabilidad de este compilador — `systemspec.Compile` solo produce
`[]PluginDecl`/`[]WireDecl`; instalar, compilar, arrancar y conectar de
verdad vive en `asterion-core` (`cmd/asterion/plugin_system.go`).
