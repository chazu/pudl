package operation

import (
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/modeldeps"
	"github.com/chazu/pudl/internal/systemmodel"
)

// reconcileModelDependencies reconciles a model's DECLARED depends_on into
// model_depends_on facts (source model:<name>) and returns any warnings. Runs on
// every model of a `pudl check`.
func reconcileModelDependencies(cat *runCatalog, modelDirs []string, m *systemmodel.SystemModel) (warnings []string, err error) {
	declared, warnings := modeldeps.Declared(m.Name, m.DependsOn, modeldeps.RegisteredIn(modelDirs))
	db, err := cat.optional()
	if err != nil {
		return warnings, err
	}
	if rerr := modeldeps.Reconcile(db, m.Name, modeldeps.DeclaredSource(m.Name), declared); rerr != nil {
		return warnings, rerr
	}
	return warnings, nil
}

// checkUpstreamFreshness warns when a transitive upstream of the model is
// drifted or failed (see modeldeps.StaleUpstreams).
func checkUpstreamFreshness(cat *runCatalog, rulePaths []string, m *systemmodel.SystemModel) []string {
	rules, err := datalog.LoadRulesFromPaths(rulePaths...)
	if err != nil {
		return nil
	}
	// The open error is dropped without a word here alone: this is a read-only
	// advisory documented to stay silent on any failure.
	db, err := cat.optional()
	if err != nil {
		return nil
	}
	return modeldeps.StaleUpstreams(db, rules, m.Name)
}
