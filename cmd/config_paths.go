package cmd

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/workspace"
)

func inspectDefinitionPaths(legacy bool) error {
	policy := wsPolicy
	if policy == nil {
		var err error
		policy, err = workspace.ResolveForCWD()
		if err != nil {
			return err
		}
	}
	if !legacy {
		out := map[string]any{"workspace": effectivePudlDir(), "schemas": policy.SchemaSearchPaths, "models": policy.ModelSearchPaths, "rules": policy.RuleSearchPaths, "definitions": policy.DefinitionSearchPaths, "config_file": config.ConfigPath(effectivePudlDir())}
		if jsonOutput {
			return printJSON(out)
		}
		fmt.Fprintf(outw(), "Workspace: %s\n", effectivePudlDir())
		for _, key := range []string{"schemas", "models", "rules", "definitions"} {
			fmt.Fprintf(outw(), "%s: %v\n", key, out[key])
		}
		return nil
	}
	globalSchema := filepath.Join(policy.GlobalDir, "schema")
	inferrer, err := inference.NewSchemaInferrer(globalSchema)
	if err != nil {
		return err
	}
	names := inferrer.GetAvailableSchemas()
	sort.Strings(names)
	rules, err := datalog.LoadRulesFromPaths(filepath.Join(globalSchema, "pudl", "rules"))
	if err != nil {
		return err
	}
	ruleNames := make([]string, 0, len(rules))
	for _, r := range rules {
		ruleNames = append(ruleNames, r.Head.Rel)
	}
	sort.Strings(ruleNames)
	var diagnostics []string
	for _, e := range inferrer.LoadErrors() {
		diagnostics = append(diagnostics, e.Error())
	}
	if jsonOutput {
		return printJSON(map[string]any{"global_schema_path": globalSchema, "active_in_project": false, "schemas": names, "rules": ruleNames, "diagnostics": diagnostics, "next_action": "Vendor the needed packages and declare dependencies in .pudl/workspace.cue; see docs/workspace.md"})
	}
	fmt.Fprintf(outw(), "Global definitions (ignored by project resolution): %s\n", globalSchema)
	for _, name := range names {
		fmt.Fprintf(outw(), "  schema: %s\n", name)
	}
	for _, name := range ruleNames {
		fmt.Fprintf(outw(), "  rule: %s\n", name)
	}
	for _, message := range diagnostics {
		fmt.Fprintf(outw(), "  diagnostic: %s\n", message)
	}
	fmt.Fprintln(outw(), "Vendor the required packages and declare dependencies in .pudl/workspace.cue. See docs/workspace.md.")
	return nil
}
