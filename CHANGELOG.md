# Changelog — Asterion Language

Formato basado en [Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/).
Este proyecto todavía no tiene releases etiquetados en git.

## [Unreleased]

### Changed
- **Extensión de archivo: `.ast` → `.asterion`.** `.ast` colisionaba con
  el significado universal en compiladores ("Abstract Syntax Tree") —
  confuso para un archivo que es CÓDIGO FUENTE, no un árbol ya parseado.
  Cambio puramente de nombre: la extensión nunca se usó en el
  lexer/parser/semantic para decidir comportamiento (siempre fue solo una
  etiqueta de diagnóstico, `archivo:línea:columna`), así que no hay
  riesgo funcional. Se renombraron los 10 archivos de `examples/`, y todo
  el código/docs que los mencionaba (`README.md`, `CHANGELOG.md`,
  `docs/TUTORIAL.md`, `spec/grammar.md`, comentarios en `ast/`,
  `semantic/`, `pluginmanifest/`, y las etiquetas internas `"t.ast"` de
  los tests). El comando en `asterion-core` pasa de `plugin from-ast` a
  `plugin from-asterion` para mantener consistencia (repo hermano,
  actualizado en el mismo cambio). Verificado: `go build`/`go test ./...`
  sin regresiones, y recompilar `asterion-mail-plugin-basic/plugin.asterion`
  reproduce el `plugin.yaml` ya commiteado byte a byte.

### Added
- Paquete `pluginmanifest`: compila un `.asterion` de definición de plugin
  (llamadas `Contract.define/language/start/health_path/api/permissions/
  events/config/resource/action`) a un `apc.Manifest` real —
  `asterion plugin from-asterion <archivo.asterion> --out <dir>` en `asterion-core`.
  Alternativa sin heurística a `from-openapi`: cada campo se declara
  explícito, nada que adivinar mal. No pasa por `semantic.Analyzer` (es un
  DSL distinto, no infraestructura) — cero cambios a lexer/parser/semantic
  existentes. Nuevos códigos `ASTR300`-`ASTR305` (ver `spec/grammar.md`).
  `go.mod` ahora depende de `asterion-plugin-contract` (repo hermano).
- Primera implementación real: lexer (indentación significativa, tipos
  `Size`/`Duration` como literales de primera clase), parser de descenso
  recursivo con recuperación de errores, AST, y un semantic analyzer que
  resuelve referencias, detecta nombres de recurso duplicados, y valida
  `Provider.<code>.<método>(...)` contra capabilities reales.
- Diagnósticos con formato estable `ASTRnnn`, posición archivo:línea:columna,
  y el formato "Required/Available/Missing" para errores de capability
  faltante.
- `CapabilityResolver` inyectable: `StaticCapabilityResolver` (snapshot de
  referencia, para el binario standalone) y el resolver real que usa
  `asterion-core` (vía `internal/coreclient`, el mismo canal HTTP que
  `asterion providers`/`asterion capabilities` — nunca una copia en memoria
  del Registry de adapters).
- CLI standalone (`cmd/asterion-language`) y comando `asterion language
  check` en `asterion-core`.
- 9 ejemplos reales en `examples/`, corridos como golden tests contra el
  compilador (incluido uno deliberadamente roto, para probar que los
  diagnósticos salen con el código y el detalle correctos).
- **`docs/TUTORIAL.md`**: tutorial con ejemplos en vivo — cada bloque de
  código corrido de verdad contra el compilador (`check` y `plugin
  from-asterion`), con la salida real al lado. Cubre por qué la sintaxis se
  parece a Python (indentación, literales `Size`/`Duration`, `def`
  opcional a nivel de archivo, declarar-antes-de-usar) y los dos DSL
  (infraestructura y manifiesto de plugin) con un ejemplo de éxito y uno
  de error real para cada uno. Nuevo `examples/tutorial-ping-plugin.asterion`
  (plugin de juguete, para el ejemplo de `Contract.*` del tutorial).

### Not implemented (a propósito)
- `plan`/`apply` — ver README, sección "Qué falta".
- Sintaxis de recursos de plugin, referencia a recursos físicos existentes,
  semántica del operador `?`.
