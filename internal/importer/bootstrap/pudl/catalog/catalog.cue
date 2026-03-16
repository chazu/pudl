package catalog

// CatalogEntry describes a registered schema type in the pudl catalog.
// Users can extend the catalog by adding their own entries alongside
// the built-in ones.
#CatalogEntry: {
	schema:        string // canonical schema name e.g. "pudl/core.#Item"
	schema_type:   string // "catchall", "base", "collection", "policy", "custom"
	resource_type: string // e.g. "unknown", "generic.collection"
	description:   string
}

entries: [string]: #CatalogEntry

entries: {
	"pudl/core.#Item": {
		schema:        "pudl/core.#Item"
		schema_type:   "catchall"
		resource_type: "unknown"
		description:   "Universal fallback schema for any data"
	}
	"pudl/core.#Collection": {
		schema:        "pudl/core.#Collection"
		schema_type:   "collection"
		resource_type: "generic.collection"
		description:   "Collection of related data items"
	}
}
