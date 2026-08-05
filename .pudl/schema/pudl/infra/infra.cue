package infra

// Organization represents an organizational unit (company, team, AWS org, etc.)
#Organization: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "infra.organization"
		identity_fields: ["name"]
		tracked_fields: ["region"]
	}

	name:        string
	region?:     string
	description?: string
	...
}

// Account represents a named account within an organization.
#Account: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "infra.account"
		identity_fields: ["org", "name"]
		tracked_fields: ["id", "email"]
	}

	org:    string
	name:   string
	id:     string
	email?: string
	...
}

// Platform represents an execution platform composed of services.
// The kind field enables conditional constraints via CUE unification.
#Platform: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "infra.platform"
		identity_fields: ["name"]
		tracked_fields: ["kind"]
	}

	name:        string
	kind:        string
	path?:       string
	description?: string
	services:    {[string]: #ServiceBinding}
}

// ServiceBinding configures a service on a platform.
#ServiceBinding: {
	namespace?: string
	...
}

// Environment represents a deployment target composed of platforms.
#Environment: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "infra.environment"
		identity_fields: ["name"]
		tracked_fields: ["cluster", "server"]
	}

	name:        string
	platforms:   {[string]: {}}
	cluster?:    string
	server?:     string
	registry?:   string
	description?: string
}

// Service represents a deployable unit with a kind discriminator.
// Use CUE's conditional fields to gate kind-specific configuration:
//   if kind == "helm" { chart: string, chart_version: string }
#Service: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "infra.service"
		identity_fields: ["name"]
		tracked_fields: ["kind"]
	}

	name:        string
	kind:        string
	path?:       string
	description?: string
	...
}
