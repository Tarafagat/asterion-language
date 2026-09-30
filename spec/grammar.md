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
7. **`AGCA.*`** — mismo criterio que `System.*`: DSL separado, su propio
   compilador (`agcaspec.Compile`). Ver "DSL de inteligencia cognitiva".

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
| ASTR600 | agcaspec | un nombre de variable (`x = AGCA.<verbo>(...)`) ya fue declarado antes en el archivo |
| ASTR601 | agcaspec | falta un argumento obligatorio en una llamada `AGCA.*` |
| ASTR602 | agcaspec | un argumento tiene el tipo equivocado (ej. se esperaba una lista o un booleano) |
| ASTR603 | agcaspec | un argumento de referencia (ej. `intelligence=`) no es un `Ident` |
| ASTR604 | agcaspec | una referencia apunta a un nombre que no fue declarado antes de esa línea |
| ASTR605 | agcaspec | `AGCA.<verbo>` no existe |
| ASTR606 | agcaspec | un valor puntual es inválido para ese campo (ej. `runtime`+`adapter` a la vez, `instances` no positivo) |
| ASTR607 | agcaspec | un valor de tipo enum (ej. `privacy`) no está en el conjunto válido |
| ASTR608 | agcaspec | una referencia resuelve a un nombre real, pero declarado con el verbo (o `Import`) equivocado |
| ASTR609 | agcaspec | `AGCA.secret`: declaró `source` y `from`/`field` a la vez, o ninguno de los dos |
| ASTR611 | agcaspec | `Import(path=...)`: el archivo referenciado no existe, no parsea, o no compila como sistema de plugins |
| ASTR612 | agcaspec | una referencia `from=` no tiene la forma `<import>.<plugin>` |
| ASTR613 | agcaspec | `from=<import>.<plugin>`: ese `Import(...)` no declaró un plugin de ese nombre |
| ASTR621 | agcaspec | `Tool.<verbo>` no existe (válidos: `define`, `capability`) |
| ASTR622 | agcaspec | una `Tool.capability(...)` declara efectos contradictorios (no-mutante y mutante a la vez) |

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
| `Contract.service(name, kind, version?, database?, user?, maps_host?, maps_port?, maps_user?, maps_password?, maps_database?, maps_url?)` | repetible | append a Services |
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

### `Contract.service(...)`: los servicios externos que el plugin necesita

`Contract.service(...)` declara una dependencia de infraestructura del
plugin — una base de datos o un Redis. `kind` es uno de `postgres`,
`mysql`, `mariadb` o `redis`. `database` y `user` son los nombres a crear
(si no se declaran, se usa el `name` del servicio); un `redis` no acepta
ninguno de los dos, porque no tiene ni bases ni usuarios que crear.

Los `maps_*` son el punto del verbo: dicen a QUÉ claves del propio
`config_schema` volcar los datos de conexión una vez resueltos. Cada
`maps_*` tiene que nombrar una clave que exista en un `Contract.config`
del mismo archivo, y `Validate()` lo verifica — un `maps_host="DB_HOTS"`
mal tipeado, si pasara, dejaría al plugin sin configurar sin que nada lo
dijera. Un `maps_*` que no se declara simplemente no se completa: el
manifiesto decide cuánto de la conexión le interesa recibir (un plugin
que solo quiere una `DATABASE_URL` declara nada más que `maps_url`).

Quien consume esto es `asterion plugin services` (ver el README de
`asterion-core`), y el orden que sigue es siempre: detectar lo que ya
existe, configurar adentro de eso, y levantar un contenedor solo si se
pide explícitamente. El manifiesto describe la necesidad; no decide cómo
se satisface.

```asterion
Contract.config(key="DB_HOST", type="string", required=true)
Contract.config(key="DB_PORT", type="string", required=true)
Contract.config(key="DB_USER", type="string", required=true)
Contract.config(key="DB_PASSWORD", type="secret", required=true)
Contract.config(key="DB_NAME", type="string", required=true)

Contract.service(
    name="db",
    kind="postgres",
    version="16",
    database="fuelity",
    user="fuelity_app",
    maps_host="DB_HOST",
    maps_port="DB_PORT",
    maps_user="DB_USER",
    maps_password="DB_PASSWORD",
    maps_database="DB_NAME",
)
```

`type="secret"` y `secret=true` son equivalentes en `Contract.config`:
las dos marcan el campo como secreto a todos los efectos (enmascarado en
`plugin config show`, excluido del `.env` del frontend en `plugin
export`).

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

## Contratos de capability (`Tool.*`)

Una **Tool** es un conjunto CERRADO de operaciones declaradas que una
Intelligence puede invocar sobre un World. Se declara en el mismo
archivo `.asterion` que la Intelligence, con el namespace `Tool.*`, y lo
compila el mismo `agcaspec.Compile`.

La garantía central: **AGCA solo puede seleccionar un `CapabilityID`
declarado acá**, que el runtime resuelve contra un handler registrado de
antemano. Nunca hay un camino desde una decisión cognitiva hacia código
arbitrario:

```text
AGCA -> CapabilityID -> Tool Contract -> Handler predefinido -> Ejecución
```

y nunca:

```text
AGCA -> string arbitrario -> eval / exec / shell / SQL crudo
```

Una Tool de base de datos puede usar SQL por dentro; lo que AGCA conoce
es `inventory.get_stock`, no la query. `database.raw_sql`,
`system.shell` o `statistics.execute(...)` solo existen si alguien los
declara EXPLÍCITAMENTE como capability — y aun así un agente puede
tenerlos en su `deny`.

| Verbo | Cardinalidad | Descripción |
|---|---|---|
| `Tool.define(name, category?, isolation?)` | repetible, asignado a una variable | Declara una Tool. `category` es taxonomía (ej. `"statistics"`). `isolation="sandbox"` marca una Tool que ejecuta código: sus capabilities exigen que el handler haya sido registrado como aislado, o no se ejecutan (nunca heredan los permisos del proceso host de Asterion). |
| `Tool.capability(tool, name, description?, input?, output?, effects?, requires?, guarantees?)` | repetible, asignado a una variable | Una operación declarada de esa Tool. `tool` es una referencia a un `Tool.define(...)` (ASTR608 si apunta a otra clase de verbo). Su ID estable es `"<tool en minúsculas>.<name>"` — ej. `statistics.search_series`. `effects`: qué le puede pasar al World (`read_only`, `pure`, `mutating`, `external_write`, `destructive`, más tags libres como `"creates:Payment"`); declarar a la vez uno no-mutante y uno mutante es ASTR622. `requires`: precondiciones verificadas ANTES del handler (ej. `"network.available"`) — sin verificador registrado, una capability con requires NO se ejecuta. `guarantees`: lo que promete devolver (`"returns:series"`), y es contra eso que la Experience evalúa después si cumplió. |

**Declarar no es implementar.** Un `Tool.capability(...)` produce un
contrato visible y puntuable, pero sin handler atado (`BindHandler` en
`asterion-graph-cognitive-architecture`) no es ejecutable: el runtime
distingue "no existe" (`ErrUnknownCapability`) de "existe el contrato,
falta el código" (`ErrNotImplemented`), y nunca improvisa una respuesta
por ninguna de las dos.

## DSL de inteligencia cognitiva (`AGCA.*`)

Un cuarto uso de la misma gramática: en vez de infraestructura, un
manifiesto de plugin o un sistema de plugins, un archivo de este tipo
declara una **Intelligence** de Asterion Graph Cognitive Architecture
(AGCA) — un Cognitive Graph, neuronas intercambiables, agentes/swarms,
qué capabilities de Asterion Plugins necesita, memoria, políticas y
bots. Ver la propuesta de investigación completa ("Camino a la AGI") y
el repo hermano
[`asterion-graph-cognitive-architecture`](https://github.com/Tarafagat/asterion-graph-cognitive-architecture),
que consume este DSL para construir y correr la Intelligence de verdad
(`asterion graph validate|inspect|run|bot run`, en `asterion-core`).

Compila con `agcaspec.Compile` (`asterion-language/agcaspec`) — otro
walker propio, tampoco pasa por `semantic.Analyzer`/`systemspec`/
`pluginmanifest`.

**Por qué esta sintaxis y no bloques con llaves.** El documento de
investigación original propone una sintaxis de bloques anidados
(`intelligence Vision { graph VisualWorld { hierarchical = true } }`) —
Asterion Language no tiene llaves ni bloques anónimos (ver § Léxico más
arriba). `AGCA.*` expresa la misma idea con el estilo que ya usan
`Contract.*`/`System.*`: una secuencia de llamadas
`AGCA.<verbo>(clave=valor, ...)`, cada una asignada a una variable que
los verbos siguientes referencian con `intelligence=<esa variable>`
— es el segundo compilador de este repo (después de `systemspec`) que
resuelve referencias reales entre variables, con el agregado de validar
el KIND de cada referencia (`AGCA.agent(intelligence=miGrafo, ...)` es
ASTR608: `miGrafo` existe, pero es un `AGCA.graph(...)`, no una
`AGCA.intelligence(...)`).

Ejemplo completo: `examples/agca-company.asterion` (incluye un bot).

| Verbo | Cardinalidad | Descripción |
|---|---|---|
| `AGCA.intelligence(name)` | repetible, asignado a una variable | Declara una Intelligence — el ancla de agrupación a la que cuelgan todos los verbos siguientes vía `intelligence=<ref>`. |
| `AGCA.graph(intelligence, name?, hierarchical?, persistent?, temporal?, provenance?)` | repetible, asignado a una variable | El Cognitive Graph de esa Intelligence. Los cuatro flags son booleanos (default `false`) que describen qué propiedades mantiene el runtime — este compilador solo los traduce a datos. |
| `AGCA.neuron(intelligence, name, runtime?, adapter?, model?, capabilities?, privacy?)` | repetible, asignado a una variable | Una neurona intercambiable. `runtime` (ej. `"gguf"`) es para ejecución local, `adapter` (ej. `"remote-llm"`) para un proveedor remoto — mutuamente excluyentes (ASTR606 si se declaran los dos). `model` acepta `"env:VAR"` para leerlo de una variable de entorno en runtime (mismo convenio que `System.wire(field="env:...")`). `privacy` es `"local"` o `"remote"` (ASTR607 si es otra cosa, default `"local"`). |
| `AGCA.swarm(intelligence, name, instances)` | repetible, asignado a una variable | `instances`: un entero positivo, o el string `"adaptive"` (ASTR606 si es 0, negativo, o un string que no sea `"adaptive"`). |
| `AGCA.agent(intelligence, name, strategy?, allow?, deny?, role?)` | repetible, asignado a una variable | Un agente ejecutivo o especializado. `strategy` es un string libre (ej. `"adaptive"`, `"evidence"`). `allow`/`deny` son la FRONTERA DE AUTORIDAD sobre capabilities de Tool: `deny` gana siempre sobre `allow`, y un `allow` vacío significa "ninguna capability autorizada" — deny-by-default. Que una Tool declare 20 capabilities no significa que este agente pueda usar las 20. `role` es el `AGCA.role(...)` que este agente asume por default: la autoridad efectiva es la INTERSECCIÓN de rol y agente (ninguno amplía al otro). |
| `AGCA.role(intelligence, name, description?, allow?, deny?, inherits?, users?)` | repetible, asignado a una variable | La autoridad de QUIÉN OPERA, distinta de la del agente (quién actúa): el mismo agente, operado por un viewer o por un admin, no puede lo mismo. `inherits` es una lista de REFERENCIAS a otros roles ya declarados — heredar AMPLÍA lo permitido, pero **deny gana siempre**: si cualquier rol de la cadena deniega una capability, ningún `allow` posterior la reabre. `users` mapea identidades concretas a este rol, para que `asterion graph act --user <email>` resuelva la autoridad sin declararla a mano en cada invocación. Un rol sin `allow` no puede invocar nada. |
| `AGCA.requires_capability(intelligence, capability)` | repetible, statement suelto (sin asignación) | Declara que la Intelligence necesita una capability de Asterion Plugins (ej. `"database.query"`) — nunca el nombre de un plugin puntual: qué plugin instalado la satisface se resuelve en runtime (Capability Router), no acá. |
| `AGCA.memory(intelligence, name, type?, graph)` | repetible, asignado a una variable | `graph`: referencia a un `AGCA.graph(...)` ya declarado de la MISMA Intelligence (ASTR608 si es de otra clase de verbo). `type` es un string libre (ej. `"hybrid"`). |
| `AGCA.policy(intelligence, name, when?, allow?, deny?, prefer?, unless?)` | repetible, asignado a una variable | Declarativo puro: `when`/`allow`/`deny`/`prefer`/`unless` son expresiones en TEXTO PLANO (ej. `"privacy==local"`), nunca evaluadas por este compilador — evaluarlas de verdad es trabajo futuro de un Policy Engine en el runtime. |
| `AGCA.bot(intelligence, name, interface, permissions?)` | repetible, asignado a una variable | Una interfaz (nunca una inteligencia aparte) hacia la Intelligence — `interface` ej. `"terminal"`, `"web"`, `"api"`. `permissions`: lista de capabilities que ESTE bot puntual puede invocar. |
| `AGCA.secret(name, source?, from?, field?, allow?)` | repetible, asignado a una variable | Una REFERENCIA a un secreto — nunca el valor real (que se sigue incorporando por el canal seguro de siempre, ej. `asterion secret set`). Dos formas MUTUAMENTE EXCLUYENTES (ASTR609 si se declaran las dos, o ninguna): `source` (literal, la ruta/clave dentro del gestor de secretos — para un secreto que no pertenece a ningún plugin importado, ej. una credencial de la empresa) **o** `from`+`field` (declara la intención de usar un secreto de un plugin importado con route TODAVÍA no resoluble localmente — ver más abajo). `allow`: lista de plugins autorizados a recibirlo inyectado — deny-by-default, vacía significa "ninguno todavía". **Si el plugin de `Import(...)` YA es resoluble localmente, sus campos `secret: true` se descubren solos — declarar acá un `AGCA.secret(from=, field=)` para uno de ellos es una doble declaración innecesaria, ver "Uniendo `AGCA.*` con `System.*`" más abajo.** |
| `Import(path)` | repetible, asignado a una variable — **builtin de llamada desnuda, no bajo `AGCA.*`** | Trae los nombres de plugin que OTRO archivo `.asterion` declaró con `System.plugin(...)` (se lee, parsea y compila con `systemspec.Compile` — nunca copia su `System.wire(...)`), para poder referenciarlos como `<var>.<plugin>` desde este archivo — ver `AGCA.secret(from=, field=)` arriba. `path` se resuelve relativo al directorio del propio archivo (nunca al cwd de quien corre el comando). ASTR611 si el archivo no existe, no parsea, o no compila como sistema de plugins. |

Todos los argumentos van nombrados, igual que en `Contract.*`/`System.*`
— incluido `Import(path="...")`, a propósito: aunque es un builtin de
una sola llamada (más parecido a `Network(cidr=...)` que a un verbo con
namespace), este DSL nunca usa argumentos posicionales en ningún otro
lado, y `Import` no es una excepción.

**Uniendo `AGCA.*` con `System.*`: capabilities Y secretos de un plugin
importado se DESCUBREN solos, sin declarar nada más.** Un `Import(...)`
con route LOCAL (una carpeta que ya existe en disco) hace que
`asterion-graph-cognitive-architecture` lea el `plugin.yaml` real de
cada plugin y derive de ahí sus capabilities (`resources[].crud` +
`actions[]`) y qué campos de su `config_schema` son `secret: true` — ver
`runtime.discoverImportedPlugins` y `asterion graph inspect`, que
reporta ambos bajo "Capabilities descubiertas"/"Secretos descubiertos".
**Escribir un `AGCA.secret(from=<plugin>, field=...)` para repetir un
secreto que el plugin YA declaró como tal en su propio manifiesto es una
doble declaración innecesaria** — el descubrimiento automático ya lo
sabe, sin que este archivo tenga que decirlo de nuevo.

`from=`/`field=` en `AGCA.secret(...)` sigue teniendo un uso real, para
el caso de borde en que el descubrimiento automático TODAVÍA no puede
resolver el plugin (ej. una route de git sin clonar todavía) pero igual
querés declarar, con anticipación, que esta Intelligence va a necesitar
uno de sus secretos:

```python
sys = Import(path="./system.asterion")   # api tiene route="git://..." — no clonada, no resoluble localmente todavía

# 'asterion graph inspect' reporta sys.api como "no resuelto" (route no
# es una carpeta local) — no hay descubrimiento automático posible
# todavía. Declarar la intención con anticipación igual compila:
api_key_ref = AGCA.secret(name="ApiKeyRef", from=sys.api, field="config:api_key")
```

`from=sys.api` es un `AttrExpr` (`<import>.<plugin>`) — `sys` debe ser un
`Import(...)` ya declarado (ASTR612 si no tiene esa forma, ASTR604 si
`sys` no existe, ASTR608 si existe pero no es un `Import`, ASTR613 si
`sys` existe pero no trajo un plugin llamado `api`). Esta declaración
NO se valida contra el `config_schema` real de `api` (no hay uno que leer
todavía) — es responsabilidad del autor que, cuando `api` sí se resuelva,
el campo `field=` siga existiendo.

`agcaspec.Compile(prog, baseDir)` solo produce un `*agcaspec.Spec`
(todas las declaraciones, en el orden del archivo, con `Import(...)`
resuelto contra `baseDir`) — construir la Intelligence de verdad
(Cognitive Graph, Neuron Registry, Agent Scheduler, Capability
Registry) y correr un ciclo cognitivo vive en
`asterion-graph-cognitive-architecture`, consumido desde `asterion-core`
(`cmd/asterion/graph.go`).
