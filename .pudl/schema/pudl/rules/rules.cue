package rules

// Rule defines a Datalog inference rule with a head (derived fact)
// and body (conditions that must be satisfied). Variables use $-prefix
// convention ($X, $Y). The Go evaluator identifies variables by prefix;
// CUE validates the structure.
#Rule: {
	name?: string
	head:  #Atom
	body: [...#Atom] & [_, ...] // at least one body atom
}

// Atom is a single relation pattern with named arguments.
// Arguments are either ground values (strings, numbers, booleans)
// or variables ($-prefixed strings like "$X").
#Atom: {
	rel:  string
	args: {[string]: #Term}
}

// Term is a ground value or a $-prefixed variable reference.
#Term: string | number | bool
