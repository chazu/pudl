package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/systemmodel"
)

var modelValidateCmd = &cobra.Command{
	Use:   "validate <name>",
	Short: "Validate a registered #SystemModel without running it",
	Long: `Resolve and validate a #SystemModel by name. It loads and decodes the
model (CUE unification) and runs structural checks on its arms — without touching
any external system. Reports problems and exits non-zero if any are found.`,
	Args:              cobra.ExactArgs(1),
	SilenceUsage:      true,
	ValidArgsFunction: completeModelNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		m, _, _, err := resolveModel(args[0])
		if err != nil {
			return err
		}
		problems := validateModel(m)
		if len(problems) == 0 {
			fmt.Printf("✓ %s is valid\n", m.Name)
			return nil
		}
		fmt.Printf("✗ %s has %d problem(s):\n", m.Name, len(problems))
		for _, p := range problems {
			fmt.Printf("  - %s\n", p)
		}
		return fmt.Errorf("model %q failed validation", m.Name)
	},
}

// validateModel runs structural checks beyond CUE decode (resolveModel already
// did the decode/unify). It reports problems the loader can't: an arm missing a
// required reference, a convergent model with nothing to reconcile, or a desired
// entry missing its catalog-routing tag.
func validateModel(m *systemmodel.SystemModel) []string {
	var problems []string

	switch m.Populate.Kind() {
	case systemmodel.KindPluginObserve:
		if m.Populate.Plugin == "" {
			problems = append(problems, "populate: observe arm has no plugin name")
		}
	case systemmodel.KindEweTarget:
		if m.Populate.EweSource == "" {
			problems = append(problems, "populate: ewe arm has no eweSource")
		}
		if len(m.Populate.Outputs) == 0 {
			problems = append(problems, "populate: ewe arm declares no outputs")
		}
	}

	if m.Convergent() && len(m.Desired) == 0 {
		problems = append(problems, "converge: declared but desired is empty (nothing to reconcile)")
	}

	// Each desired entry must carry a quoted "_schema" tag — a bare _schema is a
	// hidden CUE field that json.Marshal drops, so routing would never reach the
	// records (schema.cue #EweTarget note).
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
