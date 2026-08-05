package cmd

import (
	"fmt"

	"cuelang.org/go/cue"
	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/systemmodel"
)

var modelValidateCmd = &cobra.Command{
	Use:   "validate <name>",
	Short: "Validate a registered #SystemModel without running it",
	Long: `Resolve and validate a #SystemModel by name. It validates the authored
CUE template and runs structural checks on its arms without touching any external
system. Bound models remain valid templates before their catalog values exist;
their final concrete values are validated again when pudl runs them.`,
	Args:              cobra.ExactArgs(1),
	SilenceUsage:      true,
	ValidArgsFunction: completeModelNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		template, _, _, err := resolveModelTemplate(args[0])
		if err != nil {
			return err
		}
		problems, err := validateModelTemplate(template)
		if err != nil {
			return err
		}
		if len(problems) == 0 {
			fmt.Printf("✓ %s is valid\n", template.Name)
			return nil
		}
		fmt.Printf("✗ %s has %d problem(s):\n", template.Name, len(problems))
		for _, p := range problems {
			fmt.Printf("  - %s\n", p)
		}
		return fmt.Errorf("model %q failed validation", template.Name)
	},
}

func validateModelTemplate(template *systemmodel.ModelTemplate) ([]string, error) {
	if len(template.Inputs) == 0 {
		model, err := template.Elaborate(map[string]any{})
		if err != nil {
			return nil, err
		}
		return validateModel(model), nil
	}

	summary, err := template.Summary()
	if err != nil {
		return nil, err
	}
	var problems []string
	switch summary.PopulateKind {
	case systemmodel.KindPluginObserve:
		if summary.PopulatePlugin == "" {
			problems = append(problems, "populate: observe arm has no concrete plugin name")
		}
	case systemmodel.KindEweTarget:
		populate := template.Value().LookupPath(cue.ParsePath("populate"))
		if source, _ := populate.LookupPath(cue.ParsePath("eweSource")).String(); source == "" {
			problems = append(problems, "populate: ewe arm has no eweSource")
		}
		if outputs, listErr := populate.LookupPath(cue.ParsePath("outputs")).List(); listErr != nil || !outputs.Next() {
			problems = append(problems, "populate: ewe arm declares no outputs")
		}
	}
	if summary.ConvergePlugin != "" && summary.DesiredCount == 0 {
		problems = append(problems, "converge: declared but desired is empty (nothing to reconcile)")
	}

	if !templateUsesDifferentialDrift(template, summary.PopulateKind) {
		desired := template.Value().LookupPath(cue.ParsePath("desired"))
		if list, listErr := desired.List(); listErr == nil {
			for index := 0; list.Next(); index++ {
				field := list.Value().LookupPath(cue.MakePath(cue.Str("_schema")))
				if schema, schemaErr := field.String(); schemaErr != nil || schema == "" {
					problems = append(problems, fmt.Sprintf("desired[%d]: missing quoted \"_schema\" routing tag", index))
				}
			}
		}
	}
	return problems, nil
}

func templateUsesDifferentialDrift(template *systemmodel.ModelTemplate, kind systemmodel.PopulateKind) bool {
	if kind == systemmodel.KindEweTarget {
		return false
	}
	value := template.Value().LookupPath(cue.ParsePath("populate.differential"))
	differential, err := value.Bool()
	return err == nil && differential
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

	// Inventory-style models set-diff desired against catalog records by _schema
	// identity, so each desired entry must carry a quoted "_schema" tag (a bare
	// _schema is a hidden CUE field json.Marshal drops — schema.cue #EweTarget
	// note). Differential models (k8s) instead route desired verbatim to the
	// converge plugin as its native manifests (raw k8s objects, no _schema), so
	// the tag does not apply there.
	if !m.DifferentialDrift() {
		for i, d := range m.Desired {
			if _, ok := d["_schema"].(string); !ok {
				problems = append(problems, fmt.Sprintf("desired[%d]: missing quoted \"_schema\" routing tag", i))
			}
		}
	}

	return problems
}

func init() {
	modelCmd.AddCommand(modelValidateCmd)
}
