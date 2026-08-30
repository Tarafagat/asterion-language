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

## 7. Dónde seguir

- `README.md` — panorama general, estado real del proyecto, qué falta.
- `spec/grammar.md` — gramática completa (léxico + EBNF) y la tabla
  completa de verbos `Contract.*` con su cardinalidad y sus campos.
- `examples/` — todos los `.asterion` de este tutorial (y más) corren como
  golden tests reales del compilador, no son solo ilustrativos.
- `pluginmanifest/compile_test.go` y `semantic/golden_test.go` — si
  querés ver exactamente qué casos ya están cubiertos por tests.
