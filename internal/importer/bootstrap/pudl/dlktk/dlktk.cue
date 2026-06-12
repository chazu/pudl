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
}

#Link: {
	id:     string
	disc:   string
	src:    string
	dst:    string
	rel:    "responds_to" | "supports" | "objects_to"
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
	disc:        string
	issue:       string
	position:    string
	basis:       string
	decider:     string
	override:    bool
	supersedes?: string // prior decided position, when made via supersede
}

// relation -> schema binding
#byRelation: {
	"dlktk/discussion": #Discussion
	"dlktk/node":       #Node
	"dlktk/link":       #Link
	"dlktk/issue_card": #IssueCard
	"dlktk/preference": #Preference
	"dlktk/decision":   #Decision
}
