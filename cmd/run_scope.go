package cmd

import (
	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/systemmodel"
)

// scopeModelForRun keeps the command package's existing test seam while the
// selector policy lives with the ACUTE run plan.
func scopeModelForRun(model *systemmodel.SystemModel, selectors []string) (*systemmodel.SystemModel, error) {
	return acute.ScopeModelForRun(model, selectors)
}
