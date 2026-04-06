package nous

// Observation represents a structured observation about a codebase or system,
// recorded by an agent or human. Observations are stored as facts in the
// bitemporal fact store and serve as EDB for the Datalog evaluator and
// raw material for the nous reasoning engine.
#Observation: {
	kind:        "fact" | "obstacle" | "pattern" | "antipattern" | "suggestion" | "bug" | "opportunity"
	description: string
	repo?:       string // repository or package this observation pertains to
	source:      string // agent name or "human"
	status:      "raw" | "reviewed" | "promoted" | "rejected" | *"raw"
	worth:       number & >=0 & <=1 | *0.5
	promotedTo?: string // if promoted, what rule/convention it became
}
