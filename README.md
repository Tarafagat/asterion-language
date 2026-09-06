# Asterion Language

La capa declarativa nativa del ecosistema Asterion — la forma de decir
*"quiero que mi infraestructura tenga este estado"* en vez de describir cada
paso operacional a mano. No es un reemplazo de
[asterion-core](https://github.com/Tarafagat/asterion-core): el lenguaje
expresa la intención, Core la ejecuta.

**Estado real: `check` funciona, y `apply` ya crea infraestructura real —
para un único recurso soportado hoy (`Provider.gcp.instance(...)`), el
primero de los Provider Adapters de Core con un `CreateInstance` real del
otro lado.** `plan` (un DAG de múltiples recursos con orden de
dependencias) sigue sin existir: esta fase aplica un recurso a la vez, en
el orden en que aparece en el archivo. El nuevo paquete
[`providerspec`](providerspec/) compila `Provider.<code>.instance(...)` a
un spec propio (sin depender de `asterion-core`, que es un módulo Go
aparte); `asterion language apply <archivo.asterion> --credentials-file
<json>` (en `asterion-core`) hace la conversión final y llama de verdad al
servicio de adapters. AWS/Azure/OCI siguen dando un diagnóstico claro
(`ASTR403`, "todavía no soportado para apply") — sus adapters siguen
siendo stubs, bloqueado aguas arriba, no por este compilador.

**Tutorial con ejemplos en vivo**: [`docs/TUTORIAL.md`](docs/TUTORIAL.md)
— cada bloque de código de ahí está corrido de verdad contra este mismo
compilador, con la salida real al lado (incluida la parte de por qué la
sintaxis se parece a Python, y los dos DSL — infraestructura y manifiesto
de plugin — con sus propios ejemplos de error reales).

## Qué hace hoy

```bash
go run ./cmd/asterion-language check examples/instance.asterion
# ✓ examples/instance.asterion — 1 statement(s), sin errores
```

Lexer → parser → semantic analyzer, con diagnósticos con formato estable
(`ASTRnnn`), posición de archivo:línea:columna, y — para errores de
capability — el mismo formato de "Required/Available/Missing" que se
definió antes de escribir una sola línea de este repo.

```
ERROR ASTR212: provider oci no declara la capability requerida por database
  --> demo.asterion:3:9

Required capability:
    database

Available operations:
    instance

Missing:
    database

No infrastructure was modified.
```

## Compilar un manifiesto de plugin (`Contract.*`)

Un segundo uso del mismo lenguaje, separado de "describir infraestructura
para que Core la ejecute": describir el **contrato de un plugin nuevo**
(nombre, config, permisos, resources, actions) en un `.asterion`, y compilarlo
a un `plugin.yaml` real del [Asterion Plugin
Contract](https://github.com/Tarafagat/asterion-plugin-contract).

```bash
asterion plugin from-asterion examples/plugin-manifest.asterion --out mi-plugin
asterion plugin validate mi-plugin   # 'from-asterion' ya lo corre solo, esto es para volver a chequear después de editar
```

Es la alternativa sin heurística a `asterion plugin from-openapi`: ese
comando infiere `resources`/`actions` de un OpenAPI existente adivinando
por la forma de la URL (y a veces adivina mal — un endpoint como `/send`
sale como "resource" en vez de "action"), mientras que acá el autor
declara cada campo explícitamente con `Contract.define(...)`,
`Contract.config(...)`, `Contract.resource(...)`, etc. — nada que adivinar.
Ver `spec/grammar.md` § "DSL de manifiesto de plugin" por la tabla
completa de verbos, y el paquete `pluginmanifest/` por la implementación
(no pasa por `semantic.Analyzer` — es su propio compilador chico, sin
tocar lexer/parser/semantic existentes).

## Cómo se conecta con asterion-core

Igual que `asterion-lab` y `asterion-plugin-contract`: repo hermano,
`go.mod` lo referencia con `replace` porque no está publicado en ningún
registry. Este repo, a su vez, depende de `asterion-plugin-contract`
(mismo mecanismo) para el tipo `apc.Manifest` que usa `pluginmanifest/`.

```
asterion/
├── asterion-core/
├── asterion-lab/
├── asterion-plugin-contract/
└── asterion-language/   ← este repo
```

`asterion-core` expone `asterion language check <archivo.asterion>` — que valida
capabilities contra el servicio real de adapters (`cmd/asterion-core`,
mismo canal HTTP que ya usan `asterion providers`/`asterion capabilities`,
nunca una copia en memoria del Registry) cuando ese servicio está corriendo,
y cae a un snapshot estático de referencia (avisándolo explícitamente,
nunca en silencio) cuando no.

## Ejemplo real

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
    return web
```

Más ejemplos reales, corridos contra este mismo compilador como tests, en
[`examples/`](examples).

## Diseño

- **Sintaxis**: inspirada en Python — `def`, argumentos nombrados,
  `return` — con indentación significativa. Las primitivas de
  infraestructura (`Provider.aws.instance(...)`) son siempre declarativas
  aunque el código se vea imperativo: no ejecutan nada, producen un nodo
  de recurso deseado.
- **v0.1 requiere declarar antes de usar** (una sola pasada, sin
  referencias hacia adelante) — a propósito: mientras esa regla valga, el
  grafo de dependencias es un DAG por construcción, sin necesitar un
  detector de ciclos aparte. Permitir cualquier orden es una extensión
  futura, no parte de v0.1.
- **Sin AIR como motor nuevo**: cada dominio (Lab, Plugin, Cloud) se
  traduce a lo que ya existe y ya funciona — nunca se duplica un motor de
  ejecución. Ver el audit para el detalle de por qué. `pluginmanifest/` no
  es parte de esto — no ejecuta nada ni habla con ningún servicio en
  runtime, es una traducción en tiempo de compilación (AST → struct → YAML),
  la misma categoría de cosa que ya es `openapi.Infer` en
  `asterion-plugin-contract`, solo que con un `.asterion` como entrada en vez de
  un `openapi.yaml`.
- **Dos sistemas de capabilities reales, no uno inventado**:
  `internal/capabilities` (dominios cloud: compute/network/storage/...) y
  `internal/safety` (detect/inspect/plan/apply/verify/rollback, runtime
  local) — este compilador solo habla con el primero hoy (`Provider.*`);
  el segundo (validar declaraciones sobre esta máquina) queda para cuando
  el propio `safety.RequireSafeApply` tenga un consumidor real en Core.

## Estructura

```
lexer/          tokeniza, indentación significativa (INDENT/DEDENT)
ast/            nodos del árbol de sintaxis — structs de datos, sin lógica
parser/         descenso recursivo, con recuperación de errores
diagnostics/    formato único ASTRnnn para lexer/parser/semantic/pluginmanifest
semantic/       resolución de nombres + validación de provider/capability (infraestructura)
pluginmanifest/ compila Contract.*(...) a un apc.Manifest (plugin.yaml) — DSL separado, ver arriba
providerspec/   compila Provider.<code>.instance(...) a un InstanceSpec — lo que 'asterion language apply' aplica de verdad (hoy: solo gcp)
examples/       archivos .asterion reales, usados como golden tests
cmd/asterion-language/  CLI standalone (check, sin depender de asterion-core)
```

## Qué falta (a propósito, ver "Fases" en el audit)

- `plan` real (DAG de múltiples recursos con orden de dependencias) — hoy
  `apply` aplica un recurso a la vez, en el orden del archivo.
- `apply` para otros dominios/proveedores: LabSpec (Lab), el contrato de
  plugins vía un `ProvisioningRequest` de Cloud, y AWS/Azure/OCI en
  `providerspec` — bloqueado por los stubs de esos adapters, no por este
  compilador (ver `providerspec.CompileInstances`, que ya da un
  diagnóstico `ASTR403` explícito en vez de fallar en silencio).
- Resolver `network=`/`subnet=` como referencia a OTRO recurso del mismo
  archivo (hoy son strings literales — la ruta real del proveedor).
- Sintaxis para referenciar plugins (`Plugin.*`) — hoy parsea, no tiene
  validación de capability propia (ver `examples/plugin.asterion`).
- Referenciar un recurso físico ya existente por ID
  (`Instance.reference("inst_xxx")`) — diseñado en el audit, no
  implementado.
- Operador `?` de propagación de error — se lexea y parsea, sin semántica
  todavía.
- Cualquier cosa marcada PLANNED en este README o en `spec/`.

## Licencia

Apache License 2.0 — igual que `asterion-core`, `asterion-lab` y
`asterion-plugin-contract`.
