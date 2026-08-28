# Changelog — Asterion Language

Formato basado en [Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/).
Este proyecto todavía no tiene releases etiquetados en git.

## [Unreleased]

### Added
- Paquete `pluginmanifest`: compila un `.ast` de definición de plugin
  (llamadas `Contract.define/language/start/health_path/api/permissions/
  events/config/resource/action`) a un `apc.Manifest` real —
  `asterion plugin from-ast <archivo.ast> --out <dir>` en `asterion-core`.
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
- 8 ejemplos reales en `examples/`, corridos como golden tests contra el
  compilador (incluido uno deliberadamente roto, para probar que los
  diagnósticos salen con el código y el detalle correctos).

### Not implemented (a propósito)
- `plan`/`apply` — ver README, sección "Qué falta".
- Sintaxis de recursos de plugin, referencia a recursos físicos existentes,
  semántica del operador `?`.
