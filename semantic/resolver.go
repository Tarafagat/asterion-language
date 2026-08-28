package semantic

// CapabilityResolver responde qué providers existen y qué capability
// declara cada uno. asterion-language no sabe nada de proveedores de nube
// reales — eso vive en internal/capabilities + internal/adapters.Registry
// de asterion-core (ver el audit: internal/adapters/adapter.go). Cuando
// asterion-core invoca este analyzer, inyecta una implementación
// respaldada por su propio Registry real; StaticCapabilityResolver de
// abajo es solo para poder correr 'check' desde este repo de forma
// standalone, sin depender de asterion-core.
type CapabilityResolver interface {
	Providers() []string
	HasCapability(provider, capability string) bool
}

// ResourceCapability mapea cada método de recurso reconocido a la
// capability que internal/capabilities exige para él — reflejo exacto
// (no inventado) de los 4 Create* que el ProviderAdapter real expone hoy
// (ver internal/adapters/adapter.go: CreateInstance/CreateNetwork/
// CreateManagedDatabase/CreateBucket). Cualquier otro método no existe
// todavía del lado de Core, así que el analyzer lo rechaza en vez de
// fingir que compilaría a algo ejecutable.
var ResourceCapability = map[string]string{
	"instance": "compute",
	"network":  "network",
	"database": "database",
	"bucket":   "storage",
}

// staticProviderCapabilities es un espejo, mantenido a mano, de lo que
// internal/adapters/{aws,azure,gcp,oci} declaran hoy — confirmado por
// lectura directa de código, no supuesto: los cuatro declaran compute,
// network, subnet, firewall, storage, database, public_ip, vpn, iam,
// discovery; solo aws/azure/gcp declaran pricing, oci deliberadamente no.
// Es un snapshot de referencia para el binario standalone — la fuente de
// verdad real es siempre el Registry en vivo de asterion-core.
var staticProviderCapabilities = map[string]map[string]bool{
	"aws":   staticSet("compute", "network", "subnet", "firewall", "storage", "database", "public_ip", "vpn", "iam", "pricing", "discovery"),
	"azure": staticSet("compute", "network", "subnet", "firewall", "storage", "database", "public_ip", "vpn", "iam", "pricing", "discovery"),
	"gcp":   staticSet("compute", "network", "subnet", "firewall", "storage", "database", "public_ip", "vpn", "iam", "pricing", "discovery"),
	"oci":   staticSet("compute", "network", "subnet", "firewall", "storage", "database", "public_ip", "vpn", "iam", "discovery"),
}

func staticSet(caps ...string) map[string]bool {
	m := make(map[string]bool, len(caps))
	for _, c := range caps {
		m[c] = true
	}
	return m
}

// StaticCapabilityResolver es el resolver de referencia — ver comentario
// del paquete. NUNCA se debe presentar como la fuente de verdad en vivo:
// si asterion-core alguna vez cambia qué declara cada adapter, este mapa
// puede quedar desactualizado hasta que alguien lo actualice a mano.
type StaticCapabilityResolver struct{}

func (StaticCapabilityResolver) Providers() []string {
	out := make([]string, 0, len(staticProviderCapabilities))
	for code := range staticProviderCapabilities {
		out = append(out, code)
	}
	return out
}

func (StaticCapabilityResolver) HasCapability(provider, capability string) bool {
	caps, ok := staticProviderCapabilities[provider]
	if !ok {
		return false
	}
	return caps[capability]
}
