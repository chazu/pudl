package acute

import (
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/systemmodel"
)

// TupleScope classifies a query result against a run's `--only` selection.
//
// A check evaluates a catalog-wide relation, so under `--only` its results can
// name resources this run deliberately excluded. Gating the exit code on those
// would fail a run for something it never touched; dropping them silently would
// render a `FAIL` whose exit code vanished. TupleScope separates the two so the
// report can say which is which.
//
// It answers a *membership* question ("does this row name anything in scope?"),
// not the identity question `resolveSelector` answers, so a row matching several
// resources needs no disambiguation.
type TupleScope struct {
	// restricted is false when the run was not scoped, in which case nothing is
	// ever advisory and the classifier is a no-op.
	restricted bool
	// known holds the selector values of every desired resource in the original
	// model; selected holds those of the resources this run kept.
	known    map[string]bool
	selected map[string]bool
}

// NewTupleScope builds a classifier from a run's original and effective models.
// When the two are the same model — no `--only` — the result classifies nothing
// as advisory.
func NewTupleScope(original, effective *systemmodel.SystemModel) *TupleScope {
	scope := &TupleScope{
		known:    map[string]bool{},
		selected: map[string]bool{},
	}
	if original == nil || effective == nil || original == effective {
		return scope
	}
	scope.restricted = true
	for _, resource := range original.Desired {
		for value := range desiredSelectorValues(resource) {
			scope.known[value] = true
		}
	}
	for _, resource := range effective.Desired {
		for value := range desiredSelectorValues(resource) {
			scope.selected[value] = true
		}
	}
	return scope
}

// Restricted reports whether this run was scoped at all.
func (s *TupleScope) Restricted() bool { return s != nil && s.restricted }

// Advisory reports whether a result row falls entirely outside the run's scope.
//
// The rule is deliberately fail-safe: a row is advisory only when it names at
// least one known desired resource and *none* of the resources it names were
// selected. A row that resolves to nothing is of unknown scope and gates; a row
// naming both an included and an excluded resource gates. Every
// misclassification therefore lands on the side of a visible failure rather
// than a dropped one.
func (s *TupleScope) Advisory(values []string) bool {
	if !s.Restricted() {
		return false
	}
	sawKnown := false
	for _, value := range values {
		for _, candidate := range scopeCandidates(value) {
			if s.selected[candidate] {
				return false
			}
			if s.known[candidate] {
				sawKnown = true
			}
		}
	}
	return sawKnown
}

// scopeCandidates renders one argument value as the forms a selector namespace
// may hold it in: the value itself, and the short name after a schema's '#'
// (`pkg.#Nginx` also matches a resource selected as `Nginx`).
func scopeCandidates(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || value == "<nil>" {
		return nil
	}
	candidates := []string{value}
	if hash := strings.LastIndexByte(value, '#'); hash >= 0 && hash+1 < len(value) {
		candidates = append(candidates, value[hash+1:])
	}
	return candidates
}

// ArgValues renders a result row's argument values as strings for Advisory.
// Kept here so the classifier's notion of a value matches the selector
// namespace's, which is also built with fmt.Sprint.
func ArgValues(args map[string]interface{}) []string {
	values := make([]string, 0, len(args))
	for _, value := range args {
		values = append(values, fmt.Sprint(value))
	}
	return values
}
