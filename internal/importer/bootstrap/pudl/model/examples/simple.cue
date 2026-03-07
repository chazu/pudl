package examples

import "pudl.schemas/pudl/model"

// Simple resource shape
#SimpleResource: {
	id:    string
	name:  string
	value: _
	...
}

// Minimal model with one action method, no sockets, no auth
#SimpleModel: model.#Model & {
	schema: #SimpleResource

	metadata: model.#ModelMetadata & {
		name:        "simple"
		description: "A minimal model demonstrating the simplest valid configuration"
		category:    "custom"
	}

	methods: {
		get: model.#Method & {
			kind:        "action"
			description: "Retrieve the resource"
			inputs: {
				id: string
			}
			returns: #SimpleResource
		}
	}
}
