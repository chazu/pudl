package infra

// Account represents a named account within an organization (org).
#Account: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "infra.account"
		identity_fields: ["org", "name"]
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
	}

	name:        string
	platforms:   {[string]: {}}
	cluster?:    string
	server?:     string
	registry?:   string
	description?: string
}
