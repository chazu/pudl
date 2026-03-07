package model

#Method: {
	kind:         "action" | "qualification" | "attribute" | "codegen" | *"action"
	description?: string
	inputs?:      _
	returns?:     _
	timeout?:     =~"^[0-9]+(s|m|h)$" | *"5m"
	retries?:     int & >=0 & <=5 | *0
	blocks?:      [...string]
}

#Socket: {
	direction:    "input" | "output"
	type:         _
	description?: string
	required?:    bool | *true
}

#QualificationResult: {
	passed:  bool
	message: string
}

#AuthConfig: {
	method:       "bearer" | "sigv4" | "basic" | "custom"
	credentials?: _
}

#ModelMetadata: {
	name:        string
	description: string
	category:    "compute" | "storage" | "network" | "security" | "data" | "custom"
	icon?:       string
}

#Model: {
	schema:    _
	state?:    _
	metadata:  #ModelMetadata
	methods:   [string]: #Method
	sockets?:  [string]: #Socket
	auth?:     #AuthConfig
}
