# Tutorial — Asterion Language en vivo

Todo lo que sigue está corrido de verdad contra este mismo compilador
(`go run ./cmd/asterion-language check ...` y `asterion plugin from-asterion ...`)
— no es pseudocódigo ni salida inventada. Si copiás un bloque `.asterion` de
acá a un archivo y corrés el comando de al lado, te tiene que dar
exactamente esto mismo.

## 0. Cuatro lenguajes DSL, un solo compilador

Asterion Language sirve para cuatro cosas distintas, y es importante no
mezclarlas:

| | Describe... | Se valida con | Verbo raíz |
|---|---|---|---|
| **DSL de infraestructura** | qué instancias/redes querés que tenga tu infraestructura, para que Asterion Core lo ejecute | `semantic.Analyzer` → `asterion-language check` / `asterion language check` | `Provider.*`, `Lab.*`, `Network(...)` |
| **DSL de manifiesto de plugin** | el contrato de un plugin nuevo (nombre, config, permisos, endpoints), para compilarlo a un `plugin.yaml` | `pluginmanifest.Compile` → `asterion plugin from-asterion` | `Contract.*` |
| **DSL de sistema de plugins** | varios plugins ya instalables, cómo se conectan entre sí (wiring) y qué toolchains necesitan | `systemspec.Compile` → `asterion plugin system apply/export/watch` | `System.*` |
| **DSL de inteligencia cognitiva** | una Intelligence de AGCA (Cognitive Graph, neuronas, agentes, capabilities de plugin, memoria, políticas, bots) | `agcaspec.Compile` → `asterion graph validate/inspect/run/bot run` | `AGCA.*`, `Import(...)` |

Comparten el mismo lexer y el mismo parser (mismos tokens, misma
indentación, misma gramática de expresiones) — lo que cambia es qué
verbo raíz usa el archivo y qué compilador lo procesa después. Un
archivo `.asterion` que usa `Provider.*` nunca pasa por `pluginmanifest`, y uno
que usa `Contract.*` nunca pasa por `semantic.Analyzer` — por eso más
abajo vas a ver que `asterion-language check` rechaza un archivo
`Contract.*` (con `ASTR202`, "no está definido") — es el comportamiento
esperado, no un bug: `check` es para el DSL de infraestructura (y, en
menor medida, valida declarar-antes-de-usar para `System.*`/`AGCA.*`
también, ver § 7-8 — pero nunca para `Contract.*`).

`Import(...)` es la única excepción a "cada DSL vive en su propio
archivo": deja que un archivo `AGCA.*` traiga plugins ya declarados en
un archivo `System.*` distinto (§ 8) — el único punto de contacto real
entre dos de estos cuatro DSL.

## 1. La sintaxis: por qué se parece a Python

- **Indentación significativa.** Un bloque (el cuerpo de un `def`) se
  marca indentando, no con llaves. Tabs están prohibidos — solo espacios.
- **`def`, `return`, argumentos nombrados.** `def main(): ...` declara
  una función; `return web` o `return web, db` puede devolver una o
  varias cosas. Todo argumento a una llamada puede (y en la práctica,
  siempre debería) ir nombrado: `image="ubuntu-24.04"`.
- **Comentarios con `#`** hasta fin de línea, igual que Python.
- **Los literales son tipados por el lexer, no genéricos:**

  | Literal | Ejemplo | Uso típico |
  |---|---|---|
  | `STRING` | `"ubuntu-24.04"` | nombres, ids, imágenes |
  | `INT` / `FLOAT` | `4`, `0.5` | cpu, réplicas |
  | `SIZE` | `8GB`, `512MB` | memoria, disco — con unidad, no un número pelado |
  | `DURATION` | `90s`, `5m`, `1h` | timeouts |
  | `true` / `false` | (minúscula — no `True`/`False` como Python) | flags |
  | lista | `["smtp", "http"]` | permisos, crud, lo que sea plural |

  Probado en vivo — un `SIZE` y un `DURATION` en la misma llamada:

  ```python
  language "0.1"

  def main():
      web = Provider.aws.instance(image="ubuntu-24.04", cpu=2, memory=4GB, boot_timeout=90s)
      return web
  ```

  ```
  $ go run ./cmd/asterion-language check duration-test.asterion
  ✓ duration-test.asterion — 1 statement(s), sin errores (contract_version: 0.1)
  ```

- **Declarar antes de usar, siempre.** No hay referencias hacia
  adelante — un nombre tiene que existir en una línea anterior del mismo
  scope (o uno que lo contenga) antes de poder usarse. Es a propósito
  (ver el README, sección "Diseño"): mientras esa regla valga, el grafo
  de dependencias entre recursos es un DAG por construcción, sin
  necesitar un detector de ciclos aparte.
- **`def` es opcional a nivel de archivo.** Un `.asterion` puede ser una
  función (`def main(): ...`) o una secuencia de statements sueltos al
  nivel del módulo — las dos formas son válidas, ver el ejemplo 1 más
  abajo.

## 2. Ejemplo en vivo — infraestructura mínima

El archivo más chico posible (`examples/minimal.asterion`):

```python
language "0.1"

def main():
    return
```

```
$ go run ./cmd/asterion-language check examples/minimal.asterion
✓ examples/minimal.asterion — 1 statement(s), sin errores (contract_version: 0.1)
```

Sin `def` — statements sueltos a nivel de módulo (`examples/network.asterion`),
igual de válido:

```python
language "0.1"

network = Network(cidr="10.0.0.0/24")

web = Provider.aws.instance(
    image="ubuntu-24.04",
    cpu=4,
    memory=8GB,
    network=network
)
```

```
$ go run ./cmd/asterion-language check examples/network.asterion
✓ examples/network.asterion — 2 statement(s), sin errores (contract_version: 0.1)
```

## 3. Ejemplo en vivo — recursos que se referencian entre sí

`examples/multi-resource.asterion` — dos recursos (`web`, `db`) comparten la
misma red, y la función devuelve los dos:

```python
language "0.1"

def main():
    network = Network(cidr="10.0.0.0/24")
    web = Provider.aws.instance(
        image="ubuntu-24.04",
        cpu=4,
        memory=8GB,
        network=network
    )
    db = Provider.aws.database(
        engine="postgres",
        network=network
    )
    return web, db
```

```
$ go run ./cmd/asterion-language check examples/multi-resource.asterion
✓ examples/multi-resource.asterion — 1 statement(s), sin errores (contract_version: 0.1)
```

(Un solo "statement" reportado porque a nivel de archivo hay un único
`def` — las asignaciones de adentro son statements del bloque de esa
función, no del programa.)

## 4. Ejemplo en vivo — errores reales

`examples/error.asterion` junta a propósito tres errores distintos, para
mostrar que el analyzer los reporta **todos juntos**, no uno por
corrida:

```python
language "0.1"

def main():
    # referencia a algo que nunca se declaró -> ASTR202
    broken = Provider.aws.instance(image="ubuntu-24.04", cpu=2, memory=4GB, network=nunca_declarada)

    dup = Provider.aws.instance(image="ubuntu-24.04", cpu=1, memory=1GB)
    dup = Provider.aws.instance(image="ubuntu-24.04", cpu=1, memory=1GB)  # ASTR201: nombre de recurso repetido

    ghost = Provider.oci.load_balancer(cpu=1)  # ASTR211: método no reconocido

    return broken
```

```
$ go run ./cmd/asterion-language check examples/error.asterion
ERROR ASTR202: "nunca_declarada" no está definido — ¿falta declararlo antes de esta línea?
  --> examples/error.asterion:5:85

ERROR ASTR201: "dup" ya fue declarado como recurso (línea 10) — los nombres de recurso deben ser únicos dentro de su scope
  --> examples/error.asterion:11:5

ERROR ASTR211: Provider.oci.load_balancer no existe — Asterion Core hoy solo expone: instance, network, database, bucket
  --> examples/error.asterion:13:39

$ echo $?
1
```

Cada diagnóstico trae su código estable (`ASTRnnn`, ver
`spec/grammar.md` § "Códigos de diagnóstico" por la tabla completa) y la
posición exacta `archivo:línea:columna` — pensado para que un editor
pueda subrayar la línea exacta, no solo mostrar el error en una consola.

## 5. Ejemplo en vivo — compilar un manifiesto de plugin (`Contract.*`)

Un archivo nuevo, chico, para este tutorial
(`examples/tutorial-ping-plugin.asterion`) — un plugin de juguete con un solo
endpoint:

```python
language "0.1"

Contract.define(
    name="tutorial-ping",
    version="0.1.0",
    description="Plugin de ejemplo: responde /ping con la hora del servidor.",
    author="Vos",
    license="MIT",
)

Contract.start(command="./tutorial-ping", port=0)
Contract.health_path(path="/health")
Contract.api(base_path="/api/v1")

Contract.action(
    name="ping",
    method="GET",
    endpoint="/ping",
    description="Devuelve {status: \"ok\", timestamp: ...}",
)
```

```
$ asterion plugin from-asterion examples/tutorial-ping-plugin.asterion --out /tmp/tutorial-ping
✓ /tmp/tutorial-ping/plugin.yaml generado a partir de examples/tutorial-ping-plugin.asterion
  0 config field(s), 0 resource(s), 1 action(s)
✓ /tmp/tutorial-ping cumple el Asterion Plugin Contract
```

El `plugin.yaml` que salió, tal cual (`from-asterion` ya corrió `asterion
plugin validate` solo — el "✓ cumple el contrato" de arriba es esa
validación real, no un supuesto):

```yaml
name: tutorial-ping
version: 0.1.0
description: 'Plugin de ejemplo: responde /ping con la hora del servidor.'
author: Vos
license: MIT
contract_version: asterion.plugin/v1
start:
    command: ./tutorial-ping
port: 0
health_path: /health
api:
    base_path: /api/v1
actions:
    - name: ping
      method: GET
      endpoint: /ping
      description: 'Devuelve {status: "ok", timestamp: ...}'
```

Para un ejemplo real y completo (no de juguete) — el manifiesto real de
`asterion-mail-plugin-basic`, con `config_schema`, `permissions` y un
`resource` con CRUD — ver `examples/plugin-manifest.asterion`.

## 6. Ejemplo en vivo — errores en un manifiesto de plugin

Mismo espíritu que la sección 4, para el otro DSL. Un archivo con tres
problemas distintos a propósito:

```python
language "0.1"

Contract.define(name="roto", version="0.1.0")

Contract.define(name="roto-otra-vez", version="0.2.0")

Contract.start(command="./roto")

Contract.action(name="ping", method="GET")

Contract.wat(cosa="no existe")
```

```
$ asterion plugin from-asterion broken-manifest.asterion --out /tmp/broken-out
ERROR ASTR304: Contract.define ya se llamó antes en este archivo — solo puede aparecer una vez
  --> broken-manifest.asterion:5:16

ERROR ASTR302: Contract.action: falta el argumento obligatorio "endpoint"
  --> broken-manifest.asterion:9:16

ERROR ASTR301: Contract.wat no existe — verbos reconocidos: define, language, start, health_path, api, permissions, events, config, resource, action
  --> broken-manifest.asterion:11:13

error: broken-manifest.asterion no se pudo compilar a un plugin.yaml
```

Igual que con la infraestructura: los tres errores salen juntos en una
sola corrida (`Contract.define` repetido, falta un argumento obligatorio
de `Contract.action`, y un verbo que no existe) — nunca hace falta
corregir de a uno y volver a correr para encontrar el siguiente.

## 7. Ejemplo en vivo — sistema de plugins interconectados (`System.*`)

Un tercer uso del mismo lenguaje, separado de los dos de arriba: en vez de
describir infraestructura (`Provider.*`) o el contrato de UN plugin
(`Contract.*`), declarar un **sistema de varios plugins YA instalables**
que se conectan entre sí — para levantarlos todos de una, con sus
variables de conexión resueltas contra el estado REAL de cada uno (nunca
tipeadas a mano). Se compila con `systemspec.Compile` (no pasa por
`semantic.Analyzer` ni por `pluginmanifest` — su propio walker chico,
mismo criterio que los otros dos) y se aplica con
`asterion plugin system apply`/`export`/`watch` (`asterion-core`).

`examples/tutorial-system.asterion`:

```python
language "0.1"

db = System.plugin(
    route="./tutorial-db-plugin",
    principal=true,
)

api = System.plugin(
    route="./tutorial-web-plugin",
    requires=["node@20.11.0"],
)

System.wire(to=api, key="DATABASE_URL", from=db)
System.wire(to=api, key="DB_NAME", from=db, field="env:database_name")
```

**Cómo se declaran las variables acá.** `db`/`api` son nombres de
variable como cualquier otro en Asterion Language (`db = expr` — mismo
`assign_stmt` de la gramática) — `System.plugin(...)` es, semánticamente,
un recurso más (como `Network(...)` o `Provider.aws.instance(...)`): la
asignación le pone un nombre lógico para poder referenciarlo DESPUÉS,
nunca ejecuta nada en el momento de "declararse". `System.wire(to=api,
from=db, ...)` es la primera construcción de todo este lenguaje que
resuelve una referencia DE VERDAD contra otra variable ya declarada — ni
`pluginmanifest` (todo son literales) ni `providerspec` (rechaza
`network=network` con "todavía no soportado acá") lo hacían hasta ahora.
`to`/`from` tienen que ser identificadores de un `System.plugin(...)`
declarado ANTES en el archivo — la misma regla de "declarar antes de
usar" de siempre, ahora aplicada de verdad a una referencia entre
recursos, no solo chequeada y descartada.

`route` acepta una carpeta local (relativa al propio `.asterion`) o una
URL de git — `asterion-core` decide cuál es mirando si existe como
carpeta en disco, el compilador no interpreta nada, solo lo pasa tal
cual (mismo criterio de "el dato manda, la interpretación vive afuera"
del resto de este repo). `principal=true` marca cuál es el plugin
central del sistema (reusa el mismo campo `IsMain` que ya usa `asterion
local tunnel start` para elegir qué exponer por default — no es un
concepto nuevo). `requires=["node@20.11.0"]` — ver más abajo.

Compilar de verdad (dos carpetas de plugin real al lado, con su propio
`plugin.yaml` — omitidas acá por espacio, ver el propio ejemplo en el
repo para el contenido mínimo):

```
$ asterion plugin system apply examples/tutorial-system.asterion
✓ db → "tutorial-db-plugin"
✓ api → "tutorial-web-plugin"
Error: db: no pude arrancarlo: no pude arrancar "./tutorial-db-plugin": fork/exec ./tutorial-db-plugin: no such file or directory — el binario todavía no está compilado. Si es un plugin en Go, 'asterion plugin build tutorial-db-plugin' lo compila (y su frontend, si tiene)
```

Error real, no inventado — ningún plugin declarado en un sistema se
compila solo. `--build` sí lo hace, un plugin a la vez:

```
$ asterion plugin system apply examples/tutorial-system.asterion --build
✓ db → "tutorial-db-plugin"
✓ api → "tutorial-web-plugin"
Compilando "tutorial-db-plugin"...
$ go build -o tutorial-db-plugin .   (en .../tutorial-db-plugin)

✓ "tutorial-db-plugin" corriendo — puerto 62889, pid 29489
Error: api: wire hacia "tutorial-web-plugin" (clave "DB_NAME"): "tutorial-db-plugin" no tiene configurada la clave "database_name" — 'asterion plugin config set tutorial-db-plugin database_name=...' primero
```

Otro error real: `field="env:database_name"` exige que esa clave YA esté
configurada en `db` — nunca inventa un valor. Configurándola y
corriendo de nuevo (`asterion plugin system apply` es idempotente — no
reinstala ni rearranca lo que ya estaba bien):

```
$ asterion plugin config set tutorial-db-plugin database_name=tutorial_db
✓ Config de "tutorial-db-plugin" actualizada (1 campo(s))

$ asterion plugin system apply examples/tutorial-system.asterion --build
✓ db → "tutorial-db-plugin"
✓ api → "tutorial-web-plugin"
Compilando "tutorial-db-plugin"...
= "tutorial-db-plugin" ya está corriendo, sin cambios de wiring
Compilando "tutorial-web-plugin"...
$ pnpm install   (en .../tutorial-web-plugin/frontend)
$ pnpm build   (en .../tutorial-web-plugin/frontend)
✓ "tutorial-web-plugin" corriendo — puerto 62896, pid 29649
```

Confirmado pegándole de verdad a la API de `api` — nunca un valor
inventado, siempre lo que `db` reportó en runtime:

```
$ curl http://127.0.0.1:62896/health
{"status":"healthy","database_url":"http://127.0.0.1:62889","db_name":"tutorial_db"}
```

**`requires=["node@20.11.0"]`**: `api` tiene un frontend propio
(`frontend/package.json`) — este campo le pide a Asterion un Node
sandboxed en la versión EXACTA declarada (nunca `"node"` a secas: nunca
se adivina "la LTS actual", eso se pudre con el tiempo) antes de
compilarlo. Se descarga una sola vez desde nodejs.org, se verifica
contra el checksum oficial (`SHASUMS256.txt`), se cachea en
`~/.config/asterion/plugins/toolchains/` — nunca toca un Node que ya
tengas instalado en el sistema. pnpm no se declara aparte: viene con
Node vía Corepack. Hoy `requires` también acepta `"python"`/`"go"` (sin
versión, a propósito) para VERIFICAR que ya están en el PATH — a
diferencia de Node, ni Python ni Go publican un build portable oficial
por versión exacta, así que no hay un equivalente igual de limpio para
descargarlos sandboxed; pedir `"java"`/`"c"`/`"gcc"` da un error
explícito en vez de fingir que se resolvió.

**Exportar el sistema entero** — cada plugin a su propia carpeta
portable (backend compilado, `.env_asterion_produced` con sus valores
reales, un `Dockerfile` de referencia), más un `.env_asterion_produced`
combinado a nivel sistema como referencia única:

```
$ asterion plugin system export examples/tutorial-system.asterion --out ./salida
--- db → "tutorial-db-plugin" (puerto 8080) ---
✓ ./salida/tutorial-db-plugin/.env_asterion_produced — CONTIENE SECRETOS REALES (permisos 0600, nunca lo commitees)
--- api → "tutorial-web-plugin" (puerto 8081) ---
✓ ./salida/tutorial-web-plugin/.env_asterion_produced — CONTIENE SECRETOS REALES (permisos 0600, nunca lo commitees)
✓ ./salida/tutorial-web-plugin/frontend/.env.production — solo valores no-secretos, nunca ve .env_asterion_produced
✓ ./salida/.env_asterion_produced — referencia combinada de todos los plugins (permisos 0600, nunca lo commitees)

✓ sistema exportado a ./salida (2 plugin(s), cada uno en su propia subcarpeta)
```

Los puertos de desarrollo (62889/62896, efímeros, no significan nada
fuera de esta máquina) se reemplazan por puertos FIJOS asignados en el
export (8080/8081) — `tutorial-web-plugin/.env_asterion_produced` termina
con `ASTERION_PLUGIN_CONFIG_DATABASE_URL=http://127.0.0.1:8080`, no el
puerto de la corrida de desarrollo. Y el frontend NUNCA ve ese archivo:
`frontend/.env.production` es uno aparte, con solo los campos de
`config_schema` que el propio `plugin.yaml` de `api` no marcó `secret`
— así un secreto real no tiene ninguna forma de terminar en el bundle
público que un navegador descarga.

## 8. Ejemplo en vivo — inteligencia cognitiva (`AGCA.*`)

Un cuarto uso del mismo lenguaje, separado de los tres de arriba: en vez
de infraestructura, un manifiesto de plugin o un sistema de plugins,
declarar una **Intelligence** de [Asterion Graph Cognitive Architecture
(AGCA)](https://github.com/Tarafagat/asterion-graph-cognitive-architecture)
— un Cognitive Graph, neuronas intercambiables, agentes/swarms, qué
capabilities de Asterion Plugins necesita, memoria, políticas y bots. Se
compila con `agcaspec.Compile` (otro walker propio, mismo criterio que
`systemspec`/`pluginmanifest`) y se usa con `asterion graph
validate/inspect/run/bot run` (`asterion-core`).

`examples/agca-company.asterion` (versión resumida — el archivo real
también declara `Finance`/`Support`/`Critic` y una segunda neurona
local; ver el repo para el contenido completo):

```python
language "0.1"

# Une esta inteligencia con un sistema de plugins YA declarado en otro
# archivo (ver § 7) — sys.db/sys.api quedan disponibles para referenciar.
sys = Import(path="./tutorial-system.asterion")

brain = AGCA.intelligence(name="CompanyBrain")

world = AGCA.graph(intelligence=brain, name="CompanyWorld", hierarchical=true, persistent=true, temporal=true, provenance=true)

local_fast = AGCA.neuron(intelligence=brain, name="LocalFast", runtime="gguf", capabilities=["classification", "extraction"], privacy="local")
remote_deep_reasoner = AGCA.neuron(intelligence=brain, name="RemoteDeepReasoner", adapter="remote-llm", model="env:DEEP_MODEL", capabilities=["reasoning", "planning", "critic"], privacy="remote")

operations = AGCA.swarm(intelligence=brain, name="Operations", instances="adaptive")
executive = AGCA.agent(intelligence=brain, name="Executive", strategy="adaptive")

AGCA.requires_capability(intelligence=brain, capability="database.query")
AGCA.requires_capability(intelligence=brain, capability="inventory.read")

memory = AGCA.memory(intelligence=brain, name="LongTerm", type="hybrid", graph=world)

admin_bot = AGCA.bot(
    intelligence=brain,
    name="AdminAssistant",
    interface="terminal",
    permissions=["inventory.read", "mail.read", "repository.read"],
)

# "database_password" es un secreto de la EMPRESA (una cuenta de
# MercadoPago compartida) — no pertenece a ningún plugin importado, así
# que declararlo a mano acá no es redundante con nada.
db_password = AGCA.secret(name="DatabasePassword", source="mycompany/prod/database_password")
```

**Por qué esta sintaxis y no bloques con llaves.** El documento de
investigación original ("Camino a la AGI") propone `intelligence X {
graph Y { hierarchical = true } }` — Asterion Language no tiene llaves
(§ 1 de este tutorial: los únicos símbolos son `( ) [ ] , . = : ?`).
`AGCA.*` expresa la misma idea con el estilo que ya usan
`Contract.*`/`System.*`: cada verbo es una llamada asignada a una
variable, referenciada después con `intelligence=<esa variable>` — la
misma resolución de referencias reales que `System.wire`, con un
agregado: `agcaspec` valida el KIND de cada referencia (pasar el nombre
de un `AGCA.graph(...)` donde se espera un `AGCA.intelligence(...)` es
un error, no una coincidencia de nombre que se acepta sin más).

`Import(path=...)` es el punto de unión con `System.*`: lee, parsea y
compila (con `systemspec.Compile`, el mismo compilador de § 7) el
archivo referenciado, y expone sus plugins como `<var>.<plugin>` — es un
builtin de llamada desnuda (como `Network(cidr=...)`), no un verbo
`AGCA.*`, porque no le pertenece más a este DSL que a `System.*`. Con
una route LOCAL (una carpeta que ya existe en disco),
`asterion-graph-cognitive-architecture` va un paso más allá: lee el
`plugin.yaml` real de ese plugin y DESCUBRE SOLO sus capabilities
(`resources[].crud` + `actions[]`) y cuáles de sus campos de config son
`secret: true` — sin que este archivo tenga que declarar nada más (ver
`asterion graph inspect` más abajo). `AGCA.secret(from=, field=)`
(mismo prefijo `"config:"` que ya usa `System.wire.field` con
`"env:"`) sigue disponible para el caso de borde en que el plugin
TODAVÍA no es resoluble localmente (route de git sin clonar) — usarlo
para repetir un secreto que un plugin YA resoluble ya declaró sería una
doble declaración sin ninguna información nueva.

Compilar de verdad:

```
$ go run ./cmd/asterion-language check examples/agca-company.asterion
✓ examples/agca-company.asterion — 19 statement(s), sin errores (contract_version: 0.1)
```

Construir la Intelligence completa e inspeccionarla — sin correr ningún
ciclo cognitivo todavía:

```
$ asterion graph inspect examples/agca-company.asterion
Intelligence: CompanyBrain
Graph: CompanyWorld (0 nodo(s))
Neuronas:
  - LocalFast [gguf] privacy=local capabilities=[classification extraction] — fuera de servicio
  - LocalReasoner [gguf] privacy=local capabilities=[reasoning code] — fuera de servicio
  - RemoteDeepReasoner [remote-llm] privacy=remote capabilities=[reasoning planning critic] — fuera de servicio
  - runtime.DeterministicClassifier [deterministic] privacy=local capabilities=[classification] — disponible
Swarms:
  - Operations (instances=adaptive)
  - Finance (instances=adaptive)
  - Support (instances=adaptive)
Agentes:
  - Executive (strategy=adaptive)
  - Critic (strategy=evidence)
Capabilities de plugin requeridas:
  - database.query
  - inventory.read
  - mail.read
  - repository.read
Bots:
  - AdminAssistant (interface=terminal)
Imports:
  - sys <- ./tutorial-system.asterion (plugins: [db api])
Secrets:
  - DatabasePassword (source=mycompany/prod/database_password)
Capabilities descubiertas (de plugins importados):
  - sys.db: no resuelto (route "./tutorial-db-plugin" no es una carpeta local (¿todavía no se clonó? Import no clona git, solo lee plugin.yaml ya presente en disco))
  - sys.api: no resuelto (route "./tutorial-web-plugin" no es una carpeta local (¿todavía no se clonó? Import no clona git, solo lee plugin.yaml ya presente en disco))
```

`Import` trajo de verdad los dos plugins que `tutorial-system.asterion`
declaró (`db`/`api`, § 7) — leyó y compiló ESE archivo en disco, no una
copia ni un resumen. Acá aparecen como "no resuelto" porque
`tutorial-db-plugin`/`tutorial-web-plugin` son las carpetas de juguete
del § 7 (creadas a mano solo quien sigue ese tutorial al pie de la
letra, no versionadas en este repo) — sin ellas en disco, no hay ningún
`plugin.yaml` que leer. Con las carpetas presentes (ver § 7), esta misma
sección mostraría de verdad las capabilities de `db`/`api` y cualquier
campo de su config marcado `secret: true` — sin que `agca-company.asterion`
tuviera que declarar un solo `AGCA.secret(from=, field=)` para eso. Una
corrida real con un plugin de juguete sí presente:

```
$ asterion graph inspect /tmp/demo/company.asterion
Imports:
  - sys <- ./system.asterion (plugins: [db])
Capabilities descubiertas (de plugins importados):
  - sys.db: [records.create records.read records.list]
Secretos descubiertos (config_schema del propio plugin, sin declarar nada acá):
  - sys.db: database_password (Contraseña de la base de datos)
```

Real, no un mock: `LocalFast`/`LocalReasoner`/`RemoteDeepReasoner` están
`fuera de servicio` porque `asterion-graph-cognitive-architecture` no
trae todavía un backend GGUF ni un adapter remoto de verdad — se
compilan, se registran y se listan tal cual el `.asterion` las declaró,
pero invocarlas fallaría honestamente (`ErrNotImplemented`) en vez de
inventar una respuesta. `runtime.DeterministicClassifier` es la única
neurona que el runtime siempre trae — una clasificación real por
keywords, no un LLM — así todo archivo AGCA tiene al menos una neurona
invocable de punta a punta.

Correr un ciclo cognitivo completo (perceive → elegir neurona → merge en
el grafo → registrar experiencia):

```
$ asterion graph run examples/agca-company.asterion --goal "necesito consultar el inventario de productos"
✓ neurona usada: runtime.DeterministicClassifier
  output: read
  grafo: observación trace-1790542950434331000-1:observation -> insight trace-1790542950434331000-1:insight (trace trace-1790542950434331000-1)
```

Pedir una capability que solo declaran las neuronas fuera de servicio da
un error honesto, no un resultado inventado:

```
$ asterion graph run examples/agca-company.asterion --goal "explicar por qué" --capability reasoning
Error: ninguna neurona registrada declara la capacidad "reasoning"
```

El bot: una **interfaz** hacia `CompanyBrain`, nunca una inteligencia
aparte (§ 16 de "Camino a la AGI") — cada línea de la sesión es un goal
nuevo, corrido contra el mismo ciclo cognitivo de `graph run`:

```
$ asterion graph bot run examples/agca-company.asterion --bot admin_bot
Bot "AdminAssistant" (interface=terminal) — escribí un goal por línea, Ctrl+D o 'exit' para salir.
> necesito consultar el inventario
read
> quiero crear un producto nuevo
write
> exit
```

Dos errores reales de `Import`/`AGCA.secret(from=, field=)`, ambos
detectados al COMPILAR (nunca en runtime):

```
$ echo 'language "0.1"
sys = Import(path="./nonexistent-file.asterion")' > /tmp/bad.asterion
$ asterion graph validate /tmp/bad.asterion
Error: /tmp/bad.asterion no se pudo compilar a una inteligencia AGCA:
ERROR ASTR611: Import(path="./nonexistent-file.asterion"): no pude leer /tmp/nonexistent-file.asterion: open /tmp/nonexistent-file.asterion: no such file or directory
  --> /tmp/bad.asterion:2:13
```

```
$ echo 'language "0.1"
sys = Import(path="./tutorial-system.asterion")
bad = AGCA.secret(name="X", from=sys.nonexistent_plugin, field="config:x")' > examples/agca-bad-secret.asterion
$ asterion graph validate examples/agca-bad-secret.asterion
Error: examples/agca-bad-secret.asterion no se pudo compilar a una inteligencia AGCA:
ERROR ASTR613: AGCA.secret: "sys" no declaró ningún plugin "nonexistent_plugin" — declarados: [db api]
  --> examples/agca-bad-secret.asterion:3:37
```

Ver `spec/grammar.md` § "DSL de inteligencia cognitiva" por la tabla
completa de verbos y los códigos `ASTR600`-`ASTR613`, y el README de
[`asterion-graph-cognitive-architecture`](https://github.com/Tarafagat/asterion-graph-cognitive-architecture)
por qué subsistemas son reales hoy (Cognitive Graph, Neuron Registry,
Agent Scheduler, ciclo cognitivo) y cuáles quedan para las próximas
fases del roadmap (backend GGUF/remoto real, Executive Agent que infiera
capabilities del lenguaje natural, Capability Registry conectado a
Asterion Plugins de verdad, Policy Engine que evalúe las políticas
declaradas).

## 9. Dónde seguir

- `README.md` — panorama general, estado real del proyecto, qué falta.
- `spec/grammar.md` — gramática completa (léxico + EBNF) y las tablas
  completas de verbos de los cuatro DSL (`Contract.*`/`System.*`/`AGCA.*`,
  más infraestructura) con su cardinalidad, sus campos y sus códigos
  `ASTRnnn`.
- `examples/` — todos los `.asterion` de este tutorial (y más) corren como
  golden tests reales del compilador, no son solo ilustrativos.
- `semantic/analyzer_test.go`/`semantic/golden_test.go`,
  `pluginmanifest/compile_test.go`, `systemspec/compile_test.go`,
  `agcaspec/compile_test.go` — si querés ver exactamente qué casos ya
  están cubiertos por tests, DSL por DSL.
- [`asterion-graph-cognitive-architecture`](https://github.com/Tarafagat/asterion-graph-cognitive-architecture)
  — el repo hermano que construye y CORRE de verdad una Intelligence
  `AGCA.*` (Cognitive Graph, Neuron Registry, Agent Scheduler, ciclo
  cognitivo), consumido desde `asterion-core` como `asterion graph
  validate/inspect/run/bot run`. Su README trae el estado real,
  milestone por milestone, de los 16 pasos del roadmap del paper
  "Camino a la AGI".
