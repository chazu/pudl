package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/systemmodel"
)

var modelValidateCmd = &cobra.Command{
	Use:   "validate <name>",
	Short: "Validate a registered #SystemModel without running it",
	Long: `Resolve and validate a #SystemModel by name. It validates the authored
CUE template and runs structural checks on its arms without touching any external
system.`,
	Args:              cobra.ExactArgs(1),
	SilenceUsage:      true,
	ValidArgsFunction: completeModelNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		template, err := resolveModelTemplate(args[0])
		if err != nil {
			return err
		}
		problems, err := validateModelTemplate(template)
		if err != nil {
			return err
		}
		if jsonOutput {
			if problems == nil {
				problems = []string{}
			}
			if err := printJSON(map[string]any{
				"model": template.Name, "valid": len(problems) == 0, "problems": problems,
			}); err != nil {
				return err
			}
			if len(problems) > 0 {
				return fmt.Errorf("model %q failed validation", template.Name)
			}
			return nil
		}
		if len(problems) == 0 {
			fmt.Fprintf(outw(), "✓ %s is valid\n", template.Name)
			return nil
		}
		fmt.Fprintf(outw(), "✗ %s has %d problem(s):\n", template.Name, len(problems))
		for _, p := range problems {
			fmt.Fprintf(outw(), "  - %s\n", p)
		}
		return fmt.Errorf("model %q failed validation", template.Name)
	},
}

func validateModelTemplate(template *systemmodel.ModelTemplate) ([]string, error) {
	model, err := template.Model()
	if err != nil {
		return nil, err
	}
	problems := validateModel(model)
	if err := invocationWorkspace().CheckModelSchemas(model); err != nil {
		problems = append(problems, err.Error())
	}
	return problems, nil
}

// validateModel runs structural checks beyond CUE decode (resolveModel already
// did the decode/unify). It reports problems the loader can't: an arm missing a
// required reference, or a desired entry missing its catalog-routing tag.
func validateModel(m *systemmodel.SystemModel) []string {
	var problems []string

	if m.Populate.Kind() == systemmodel.KindCommand {
		problems = append(problems, validateCommandRuns(m.Populate)...)
	}
	if h := m.Populate.HTTP; h != nil && h.Auth != nil && os.Getenv(h.Auth.Env) == "" {
		problems = append(problems, fmt.Sprintf("populate.auth: environment variable %s is not set", h.Auth.Env))
	}

	// Drift set-diffs desired against catalog records by _schema identity, so
	// each desired entry must carry a quoted "_schema" tag (a bare _schema is a
	// hidden CUE field json.Marshal drops).
	for i, d := range m.Desired {
		if _, ok := d["_schema"].(string); !ok {
			problems = append(problems, fmt.Sprintf("desired[%d]: missing quoted \"_schema\" routing tag", i))
		}
	}

	return problems
}

func init() {
	modelCmd.AddCommand(modelValidateCmd)
}

// validateCommandRuns checks a command arm's runs: each argv names a program
// that can be found, and the timeout parses.
func validateCommandRuns(p systemmodel.Populate) []string {
	var problems []string
	for i, run := range p.Runs {
		if len(run.Argv) == 0 || run.Argv[0] == "" {
			problems = append(problems, fmt.Sprintf("populate.runs[%d]: argv is empty", i))
			continue
		}
		// A path is resolved against the run's dir when the run starts; a bare
		// name must be on PATH.
		if !strings.ContainsRune(run.Argv[0], filepath.Separator) {
			if _, err := exec.LookPath(run.Argv[0]); err != nil {
				problems = append(problems, fmt.Sprintf("populate.runs[%d]: %q not found on PATH", i, run.Argv[0]))
			}
		}
	}
	if p.Timeout != "" {
		if _, err := time.ParseDuration(p.Timeout); err != nil {
			problems = append(problems, fmt.Sprintf("populate.timeout: %v", err))
		}
	}
	return problems
}
