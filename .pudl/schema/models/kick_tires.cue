package models

import sm "pudl.schemas/pudl/systemmodel@v0"

_kickPlugin: [{
	name:   "kicksecret"
	script: "../../populators/kick-observe/plugin.py"
}]

#KickProducer: sm.#SystemModel & {
	name:    "kick-producer"
	plugins: _kickPlugin
	populate: {
		plugin:       "kicksecret"
		differential: false
		input: {role: "producer", value: "ready"}
	}
}

#KickConsumer: sm.#SystemModel & {
	name:    "kick-consumer"
	plugins: _kickPlugin
	inputs: {
		value: string @pudl(binding=plain)
	}
	bindings: value: {
		source: {
			model:  "kick-producer"
			schema: "kicktires.#Thing"
			identity: {name: "one"}
		}
		path: "/value"
	}
	populate: {
		plugin:       "kicksecret"
		differential: false
		input: {role: "consumer", bound: inputs.value}
	}
}

#KickDeniedSource: sm.#SystemModel & {
	name:    "kick-denied-source"
	plugins: _kickPlugin
	inputs: {
		value: string @pudl(binding=plain)
	}
	bindings: value: {
		source: {
			model:  "kick-producer"
			schema: "kicktires.#Thing"
			identity: {name: "one"}
		}
		path: "/private"
	}
	populate: {
		plugin:       "kicksecret"
		differential: false
		input: {role: "consumer", bound: inputs.value}
	}
}

#KickInvalidPointer: sm.#SystemModel & {
	name:    "kick-invalid-pointer"
	plugins: _kickPlugin
	inputs: {
		value: string @pudl(binding=plain)
	}
	bindings: value: {
		source: {
			model:  "kick-producer"
			schema: "kicktires.#Thing"
			identity: {name: "one"}
		}
		path: "value"
	}
	populate: {
		plugin:       "kicksecret"
		differential: false
		input: {role: "consumer", bound: inputs.value}
	}
}

#KickUnannotatedInput: sm.#SystemModel & {
	name:    "kick-unannotated-input"
	plugins: _kickPlugin
	inputs: value: string
	bindings: value: {
		source: {
			model:  "kick-producer"
			schema: "kicktires.#Thing"
			identity: {name: "one"}
		}
		path: "/value"
	}
	populate: {
		plugin:       "kicksecret"
		differential: false
		input: {role: "consumer", bound: inputs.value}
	}
}

#KickCycleA: sm.#SystemModel & {
	name: "kick-cycle-a"
	depends_on: ["kick-cycle-b"]
	plugins: _kickPlugin
	populate: {plugin: "kicksecret", differential: false, input: {role: "cycle-a"}}
}

#KickCycleB: sm.#SystemModel & {
	name: "kick-cycle-b"
	depends_on: ["kick-cycle-a"]
	plugins: _kickPlugin
	populate: {plugin: "kicksecret", differential: false, input: {role: "cycle-b"}}
}

#KickMutator: sm.#SystemModel & {
	name:    "kick-mutator"
	plugins: _kickPlugin
	populate: {
		plugin: "kicksecret"
		input: {role: "mutator", state: "state/mutator"}
	}
	desired: [{"_schema": "kicktires.mutation", name: "mutator"}]
	converge: {
		plugin: "kicksecret"
		input: {role: "mutator", state: "state/mutator"}
	}
}

#KickMutatorStale: sm.#SystemModel & {
	name:    "kick-mutator-stale"
	plugins: _kickPlugin
	populate: {
		plugin: "kicksecret"
		input: {role: "mutator-stale", state: "state/mutator-stale"}
	}
	desired: [{"_schema": "kicktires.mutation", name: "mutator-stale"}]
	converge: {
		plugin: "kicksecret"
		input: {role: "mutator-stale", state: "state/mutator-stale"}
	}
}

#KickSealedProducer: sm.#SystemModel & {
	name:    "kick-sealed-producer"
	plugins: _kickPlugin
	populate: {
		plugin: "kicksecret"
		input: {role: "sealed-producer", state: "state/sealed-producer"}
	}
	desired: [{"_schema": "kicktires.mutation", name: "sealed-producer"}]
	converge: {
		plugin: "kicksecret"
		input: {role: "sealed-producer", state: "state/sealed-producer"}
		sealed_outputs: {
			TOKEN: {
				ref:        "kicksecret:token"
				store_mode: "create_if_absent"
			} @pudl(binding=sealed)
		}
	}
}

#KickSealedConsumer: sm.#SystemModel & {
	name: "kick-sealed-consumer"
	depends_on: ["kick-sealed-producer"]
	plugins: _kickPlugin
	populate: {
		plugin: "kicksecret"
		input: {role: "sealed-consumer", state: "state/sealed-consumer"}
	}
	desired: [{"_schema": "kicktires.mutation", name: "sealed-consumer"}]
	converge: {
		plugin: "kicksecret"
		input: {role: "sealed-consumer", state: "state/sealed-consumer"}
		sealed_inputs: {
			TOKEN: {
				source: {model: "kick-sealed-producer", output: "TOKEN"}
				delivery_mode: "env"
			} @pudl(binding=sealed)
		}
	}
}

#KickDeniedOutput: sm.#SystemModel & {
	name:    "kick-denied-output"
	plugins: _kickPlugin
	populate: {
		plugin: "kicksecret"
		input: {role: "denied-output", state: "state/denied-output"}
	}
	desired: [{"_schema": "kicktires.mutation", name: "denied-output"}]
	converge: {
		plugin: "kicksecret"
		input: {role: "denied-output", state: "state/denied-output"}
		sealed_outputs: {
			TOKEN: {
				ref:        "kicksecret:denied"
				store_mode: "overwrite"
			} @pudl(binding=sealed)
		}
	}
}
