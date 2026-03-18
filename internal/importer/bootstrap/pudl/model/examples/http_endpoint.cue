package examples

import "pudl.schemas/pudl/model"

// HTTP request shape
#HTTPRequest: {
	method:  "GET" | "POST" | "PUT" | "DELETE" | "PATCH"
	url:     string
	headers?: [string]: string
	body?:   _
	...
}

// HTTP response shape
#HTTPResponse: {
	status_code: int
	headers?: [string]: string
	body?: _
	...
}

// HTTP endpoint model with GET/POST actions and auth
#HTTPEndpointModel: model.#Model & {
	schema: #HTTPRequest
	state:  #HTTPResponse

	metadata: model.#ModelMetadata & {
		name:        "http_endpoint"
		description: "Generic HTTP endpoint for API interactions"
		category:    "network"
		icon:        "globe"
	}

	methods: {
		get: model.#Method & {
			kind:        "action"
			description: "Send a GET request"
			inputs: {
				url:      string
				headers?: [string]: string
			}
			returns: #HTTPResponse
			timeout: "30s"
			retries: 2
		}
		post: model.#Method & {
			kind:        "action"
			description: "Send a POST request"
			inputs: {
				url:      string
				body:     _
				headers?: [string]: string
			}
			returns: #HTTPResponse
			timeout: "30s"
			retries: 1
		}
		health_check: model.#Method & {
			kind:        "qualification"
			description: "Check if the endpoint is reachable"
			inputs: {
				url: string
			}
			returns: model.#QualificationResult
			blocks:  ["get", "post"]
		}
	}

	sockets: {
		base_url: model.#Socket & {
			direction:   "input"
			type:        string
			description: "Base URL for the endpoint"
			required:    true
		}
		response_body: model.#Socket & {
			direction:   "output"
			type:        _
			description: "Response body from the last request"
			required:    false
		}
	}

	auth: model.#AuthConfig & {
		method: "bearer"
	}
}
