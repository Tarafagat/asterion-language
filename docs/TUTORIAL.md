# Tutorial — Asterion Language en vivo

Todo lo que sigue está corrido de verdad contra este mismo compilador
(`go run ./cmd/asterion-language check ...` y `asterion plugin from-asterion ...`)
— no es pseudocódigo ni salida inventada. Si copiás un bloque `.asterion` de
acá a un archivo y corrés el comando de al lado, te tiene que dar
exactamente esto mismo.

## 0. Dos lenguajes DSL, un solo compilador

Asterion Language sirve para dos cosas distintas, y es importante no
mezclarlas:

| | Describe... | Se valida con | Verbo raíz |
|---|---|---|---|
| **DSL de infraestructura** | qué instancias/redes querés que tenga tu infraestructura, para que Asterion Core lo ejecute | `semantic.Analyzer` → `asterion-language check` / `asterion language check` | `Provider.*`, `Lab.*`, `Network(...)` |
| **DSL de manifiesto de plugin** | el contrato de un plugin nuevo (nombre, config, permisos, endpoints), para compilarlo a un `plugin.yaml` | `pluginmanifest.Compile` → `asterion plugin from-asterion` | `Contract.*` |

Comparten el mismo lexer y el mismo parser (mismos tokens, misma
indentación, misma gramática de expresiones) — lo que cambia es qué
verbo raíz usa el archivo y qué compilador lo procesa después. Un
archivo `.asterion` que usa `Provider.*` nunca pasa por `pluginmanifest`, y uno
que usa `Contract.*` nunca pasa por `semantic.Analyzer` — por eso más
abajo vas a ver que `asterion-language check` rechaza un archivo
`Contract.*` (con `ASTR202`, "no está definido") — es el comportamiento
esperado, no un bug: `check` es para el otro DSL.

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

## 8. Dónde seguir

- `README.md` — panorama general, estado real del proyecto, qué falta.
- `spec/grammar.md` — gramática completa (léxico + EBNF) y la tabla
  completa de verbos `Contract.*` con su cardinalidad y sus campos.
- `examples/` — todos los `.asterion` de este tutorial (y más) corren como
  golden tests reales del compilador, no son solo ilustrativos.
- `pluginmanifest/compile_test.go` y `semantic/golden_test.go` — si
  querés ver exactamente qué casos ya están cubiertos por tests.
