package nous

// Observation represents a structured observation about a codebase or system,
// recorded by an agent or human. Observations are stored as facts in the
// bitemporal fact store and serve as EDB for the Datalog evaluator and
// raw material for the nous reasoning engine.
//
// The scope field uses "repo:path" format (e.g. "pudl:internal/database")
// to globally identify where the observation applies. Omit the path for
// repo-wide observations (e.g. "pudl").
#Observation: {
	kind:        "fact" | "obstacle" | "pattern" | "antipattern" | "suggestion" | "bug" | "opportunity"
	description: string
	scope?:      string // "repo:path" format, e.g. "pudl:internal/database"
	source:      string // agent name or "human"
	status:      "raw" | "reviewed" | "promoted" | "rejected" | *"raw"
	worth:       number & >=0 & <=1 | *0.5
	promotedTo?: string // if promoted, what rule/convention it became
	prevVersion?: string // fact ID of the prior maturity version (set on transition)
}

// Feedback is a reinforcement signal about another fact or rule, recorded by an
// agent or human after acting on it. Feedback is append-only and stored as a
// fact in the bitemporal store; corroboration is preserved as signal (multiple
// sources giving the same verdict produce distinct facts, not an aggregated
// count). Downstream scoring (decay, harmful-weighting) reads feedback at query
// time — feedback ingestion itself carries no weighting.
//
// The target field is the ID (or rule reference) the feedback is about.
#Feedback: {
	target:   string                                // fact/rule ID this feedback concerns
	verdict:  "helpful" | "harmful" | "neutral"     // direction of the signal
	outcome?: "success" | "failure"                 // task result, if the feedback follows an action
	source:   string                                // agent name or "human"
	note?:    string                                // optional free-text rationale
}
