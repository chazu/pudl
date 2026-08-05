package dlktk

// Args schemas for the dlktk/* fact relations written by dlktk, the dialectic
// toolkit that shares pudl's bitemporal fact store (one fact per move, all
// under the reserved dlktk/* relation namespace). The fact's relation name
// selects the schema; the fact's args object must unify with it. Registering
// these as a built-in package makes dlktk facts typed and interpretable by
// pudl and other fact-store consumers rather than opaque JSON blobs.

#Discussion: {
	id:         string
	title:      string
	subject:    string // file:… | pkg:… | commit:… | q:…
	created_by: string
}

#Node: {
	id:     string
	disc:   string
	kind:   "issue" | "position" | "argument"
	text:   string
	author: string
	tag?:   "assumption" // a challengeable premise; bookkeeping only, never reaches the evaluator
	drops?: [...string] // syntheses: what the hybrid explicitly excludes from its parents (metadata)
}

#Link: {
	id:     string
	disc:   string
	src:    string
	dst:    string
	rel:    "responds_to" | "supports" | "objects_to" | "synthesizes" | "raised_from" | "addresses"
	author: string
}

#IssueCard: {
	issue:       string
	cardinality: "select_one" | "open"
}

#Preference: {
	id:     string
	disc:   string
	winner: string
	loser:  string
	basis:  string
	author: string
}

#Decision: {
	disc:             string
	issue:            string
	position:         string
	basis:            string
	decider:          string
	override:         bool
	supersedes?:      string // prior decided position, when made via supersede
	review_by?:       int    // unix seconds; re-examination horizon check enforces
	kind?:            "map"  // a value-map decision: the object is the issue's audience map, not a single position (empty/absent = conventional position decision)
	superseded_kind?: "map"  // kind of the decision this one overturned, so a map->position or position->map conversion is legible on the record
}

#Roster: {
	disc:   string
	author: string // stable ownership identity
	role:   string // persona; metadata only, never reaches the evaluator
}

#Reframe: {
	disc:   string
	old:    string // the issue whose framing was replaced
	new:    string // the issue that replaced it
	basis:  string // why — mandatory, the Q4 force-capture ethos
	author: string
}

#Value: {
	disc:   string
	node:   string // the position/argument
	value:  string // the value it promotes (audience lens input)
	author: string
}

#Audience: {
	disc: string
	name: string
	ranking: [...string] // values, most important first (strict order)
	basis?:              string // recorded when a supersession retires a prior ranking
	author:              string
}

// relation -> schema binding
#byRelation: {
	"dlktk/discussion": #Discussion
	"dlktk/node":       #Node
	"dlktk/link":       #Link
	"dlktk/issue_card": #IssueCard
	"dlktk/preference": #Preference
	"dlktk/decision":   #Decision
	"dlktk/roster":     #Roster
	"dlktk/reframe":    #Reframe
	"dlktk/value":      #Value
	"dlktk/audience":   #Audience
}
