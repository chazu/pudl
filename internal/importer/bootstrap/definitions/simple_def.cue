package definitions

import "pudl.schemas/pudl/model/examples"

my_simple: examples.#SimpleModel & {
	schema: {
		id:    "resource-001"
		name:  "My Simple Resource"
		value: 42
	}
}
