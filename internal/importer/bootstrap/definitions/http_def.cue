package definitions

import "pudl.schemas/pudl/model/examples"

api_endpoint: examples.#HTTPEndpointModel & {
	schema: {
		method: "GET"
		url:    "https://api.example.com/data"
	}
}
