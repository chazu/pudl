package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/validator"
)

var modelShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a registered #SystemModel definition",
	Long: `Show the details of a registered #SystemModel by name (the instance's
'name:' field or its short definition name): populate arm, desired state,
and checks.`,
	Args:              cobra.ExactArgs(1),
	SilenceUsage:      true,
	ValidArgsFunction: completeModelNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		m, _, _, err := resolveModel(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			b, err := json.MarshalIndent(m, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(outw(), string(b))
			return nil
		}
		cfg, err := loadEffectiveConfig()
		if err != nil {
			return err
		}
		schemas, err := validator.NewChainValidator(effectiveSchemaPaths(cfg)...)
		if err != nil {
			return fmt.Errorf("load schemas: %w", err)
		}
		printModel(m, schemas)
		return nil
	},
}

func printModel(m *systemmodel.SystemModel, schemas *validator.ChainValidator) {
	fmt.Fprintf(outw(), "Model: %s\n", m.Name)
	fmt.Fprintln(outw(), strings.Repeat("-", 60))

	// Populate arm.
	switch m.Populate.Kind() {
	case systemmodel.KindCommand:
		fmt.Fprintf(outw(), "  Populate:  command (%d runs)\n", len(m.Populate.Runs))
		for _, run := range m.Populate.Runs {
			fmt.Fprintf(outw(), "    run: %s\n", strings.Join(run.Argv, " "))
		}
		if m.Populate.Schema != "" {
			fmt.Fprintf(outw(), "    schema: %s\n", m.Populate.Schema)
		}
	case systemmodel.KindHTTP:
		h := m.Populate.HTTP
		fmt.Fprintf(outw(), "  Populate:  http %s (%d runs)\n", h.Base, len(h.Runs))
		if h.Each != nil {
			fmt.Fprintf(outw(), "    each: GET %s\n", h.Each.Get)
		}
		for _, run := range h.Runs {
			fmt.Fprintf(outw(), "    run: GET %s\n", run.Get)
		}
		if m.Populate.Schema != "" {
			fmt.Fprintf(outw(), "    schema: %s\n", m.Populate.Schema)
		}
	case systemmodel.KindNone:
		fmt.Fprintln(outw(), "  Populate:  none (checks-only)")
	}

	// Desired state.
	fmt.Fprintf(outw(), "  Desired:   %d resource(s)\n", len(m.Desired))
	for _, d := range m.Desired {
		if s, ok := d["_schema"].(string); ok {
			fmt.Fprintf(outw(), "    - %s\n", describeDesiredSchema(s, schemas))
		}
	}

	// Checks.
	if len(m.Checks) > 0 {
		fmt.Fprintf(outw(), "  Checks:    %d\n", len(m.Checks))
		for _, c := range m.Checks {
			fmt.Fprintf(outw(), "    - %s (%s, expect %s)\n", c.Name, c.Severity, c.Expect)
		}
	}

	if len(m.DependsOn) > 0 {
		fmt.Fprintf(outw(), "  Depends:   %s\n", strings.Join(m.DependsOn, ", "))
	}
	if m.Freshness != nil {
		fmt.Fprintf(outw(), "  Freshness: every=%s drift=%t\n", m.Freshness.Every, m.Freshness.Drift)
	}
}

// Desired records may carry a resource type instead of a CUE schema reference.
// Resolve through metadata, not a guessed package/definition naming convention.
func describeDesiredSchema(ref string, schemas *validator.ChainValidator) string {
	if strings.Contains(ref, ".#") || strings.Contains(ref, ":#") {
		name := schemaname.Normalize(ref)
		if schemas.HasSchema(name) {
			return name
		}
		return name + " (not registered)"
	}
	matches := schemas.GetSchemasByResourceType(ref)
	switch len(matches) {
	case 0:
		return fmt.Sprintf("resource type: %s (no registered schema)", ref)
	case 1:
		return fmt.Sprintf("%s (resource type: %s)", matches[0], ref)
	default:
		return fmt.Sprintf("resource type: %s (multiple schemas: %s)", ref, strings.Join(matches, ", "))
	}
}

// completeModelNames provides shell completion of registered model names.
func completeModelNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	models, _, err := listModels()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var out []string
	for _, mi := range models {
		if toComplete == "" || strings.HasPrefix(mi.Name, toComplete) {
			out = append(out, mi.Name+"\t"+mi.SchemaName)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	modelCmd.AddCommand(modelShowCmd)
}
