// Package policy evaluates compliance policies against single resources.
//
// A policy is a schema with `_pudl.schema_type: "policy"` whose base_schema
// chain ends at a type schema. Evaluating a policy against a record yields a
// status (compliant, noncompliant or unknown) and named violations. The
// package is pure: it reads loaded CUE schemas and metadata (a Source, such as
// *inference.SchemaInferrer) and never touches the database.
//
// See docs/design/2026-11-01-compliance-policies.md.
package policy

import "sort"

// Severity ranks a rule. The zero value means "no violations".
type Severity string

// Severities, lowest to highest.
const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// DefaultSeverity applies to unannotated constraints and to a rule that
// declares no severity.
const DefaultSeverity = SeverityMedium

var severityRank = map[Severity]int{SeverityLow: 1, SeverityMedium: 2, SeverityHigh: 3, SeverityCritical: 4}

// Status is the outcome of one policy against one resource.
type Status string

// Statuses.
const (
	// StatusCompliant: no violations and nothing absent.
	StatusCompliant Status = "compliant"
	// StatusNoncompliant: at least one violation.
	StatusNoncompliant Status = "noncompliant"
	// StatusUnknown: no violations, but a constrained field was absent and its
	// absence is not a violation (the collector may not have gathered it).
	StatusUnknown Status = "unknown"
)

// Kind says why a constraint failed.
type Kind string

// Kinds.
const (
	KindConflict Kind = "conflict" // the record's value does not satisfy the constraint
	KindAbsent   Kind = "absent"   // the record lacks a field the policy requires
)

// Violation is one failed constraint. It never carries record values.
type Violation struct {
	// Rule is the nearest `@pudl(rule=...)` at or above Path, or Path itself
	// for an unannotated constraint.
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	// Path is a field path (internal/fieldpath syntax); list elements are
	// written as [*].
	Path string `json:"path"`
	Kind Kind   `json:"kind"`
	// DeclaredBy is the schema in the policy's chain that declared the rule:
	// the ancestor policy for an inherited rule, or the type schema for a
	// constraint the policy inherits from its type.
	DeclaredBy string `json:"declared_by_policy"`
}

// Result is the evaluation of one policy against one record.
type Result struct {
	Policy string `json:"policy"`
	// Type is the record's assigned type schema.
	Type   string `json:"type"`
	Digest string `json:"policy_digest"`
	Status Status `json:"status"`
	// Violations make the result noncompliant.
	Violations []Violation `json:"violations,omitempty"`
	// Unknown lists absent constraints whose absence is not a violation; they
	// make an otherwise clean result unknown.
	Unknown []Violation `json:"unknown,omitempty"`
	// Provisional is true when the record's type schema is not reviewed as
	// validated (`_pudl.review.status`).
	Provisional bool `json:"provisional"`
}

// HighestSeverity is the most severe violation's severity, or "" when there
// are none. It is derived, never stored.
func (r *Result) HighestSeverity() Severity {
	var best Severity
	for _, v := range r.Violations {
		if severityRank[v.Severity] > severityRank[best] {
			best = v.Severity
		}
	}
	return best
}

// Rule is a named constraint declared with `@pudl(rule=...)`.
type Rule struct {
	Name     string   `json:"name"`
	Severity Severity `json:"severity"`
	Doc      string   `json:"doc,omitempty"`
	// Paths are the field paths the rule annotates.
	Paths []string `json:"paths"`
	// DeclaredBy is the schema in the chain that declared the rule.
	DeclaredBy string `json:"declared_by_policy"`
}

// sortViolations orders violations by path, rule and kind.
func sortViolations(vs []Violation) {
	sort.Slice(vs, func(i, j int) bool {
		if vs[i].Path != vs[j].Path {
			return vs[i].Path < vs[j].Path
		}
		if vs[i].Rule != vs[j].Rule {
			return vs[i].Rule < vs[j].Rule
		}
		return vs[i].Kind < vs[j].Kind
	})
}
