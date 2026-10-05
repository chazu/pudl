package doctor

import (
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/validator"
)

// CheckSchemaLoading reports schema packages that fail to load, loaded schemas
// that fail integrity checks, and base_schema cycles, for the global root.
func CheckSchemaLoading() *CheckResult {
	return CheckSchemaLoadingAt(config.GetPudlDir())
}

// CheckSchemaLoadingAt checks one PUDL root's schema repository.
//
// Loading is per package, so a broken package no longer stops the others from
// loading; imports keep working and classify what the broken package would
// have matched as the catch-all. This check is where those problems surface.
func CheckSchemaLoadingAt(pudlDir string) *CheckResult {
	cfg, err := config.LoadFrom(pudlDir)
	if err != nil {
		return &CheckResult{
			Status:  "warning",
			Message: "Failed to load configuration",
			Details: err.Error(),
			Fix:     "Check config file at " + config.ConfigPath(pudlDir),
		}
	}

	set := validator.LoadSchemaSet([]string{cfg.SchemaPath})

	var problems []string
	for _, loadErr := range set.Errors {
		problems = append(problems, loadErr.Error())
	}
	for _, cycle := range set.Cycles {
		problems = append(problems, "base_schema cycle: "+validator.FormatCycle(cycle))
	}

	if len(problems) == 0 {
		return &CheckResult{
			Status:  "ok",
			Message: "All schema packages load",
			Details: fmt.Sprintf("%d schemas loaded from %s", len(set.Schemas), cfg.SchemaPath),
		}
	}

	return &CheckResult{
		Status:  "error",
		Message: fmt.Sprintf("%d schema loading problem(s)", len(problems)),
		Details: strings.Join(problems, "\n  "),
		Fix:     "Fix the named packages (e.g. run `cue vet ./...` in the schema directory) and break any base_schema cycle; until then, data they would match is classified as the catch-all",
	}
}
