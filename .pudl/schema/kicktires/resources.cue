package kicktires

#Thing: {
	_pudl: {
		schema_type:   "base"
		resource_type: "kicktires.thing"
		identity_fields: ["name"]
		tracked_fields: ["value", "private"]
	}
	"_schema"?: string
	name:       string
	value:      string @pudl(binding=plain)
	private:    string
	...
}

#Consumer: {
	_pudl: {
		schema_type:   "base"
		resource_type: "kicktires.consumer"
		identity_fields: ["name"]
		tracked_fields: ["bound"]
	}
	"_schema"?: string
	name:       string
	bound:      string
	...
}
