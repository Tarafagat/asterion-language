module github.com/Tarafagat/asterion-language

go 1.25.0

require github.com/Tarafagat/asterion-plugin-contract v0.0.0-00010101000000-000000000000

require gopkg.in/yaml.v3 v3.0.1 // indirect

// asterion-plugin-contract todavía no está publicado en ningún registry —
// hasta que lo esté, se necesita clonado como carpeta hermana. Lo usa
// pluginmanifest/ para compilar un .ast al tipo apc.Manifest canónico.
replace github.com/Tarafagat/asterion-plugin-contract => ../asterion-plugin-contract
