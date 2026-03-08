package cmd

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/definition"
	"pudl/internal/errors"
	"pudl/internal/executor"
	"pudl/internal/glojure"
	"pudl/internal/model"
)

var methodListCmd = &cobra.Command{
	Use:   "list <definition>",
	Short: "List methods available for a definition",
	Long: `List all methods available for a definition, grouped by kind.

Shows which methods have .clj implementation files.

Examples:
    pudl method list prod_instance
    pudl method list my_simple`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMethodListCommand(args[0])
	},
}

func init() {
	methodCmd.AddCommand(methodListCmd)
}

func runMethodListCommand(defName string) error {
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	rt := glojure.New()
	if err := rt.Init(); err != nil {
		return fmt.Errorf("failed to initialize Glojure runtime: %w", err)
	}

	registry := glojure.NewRegistry(rt)
	modelDisc := model.NewDiscoverer(cfg.SchemaPath)
	defDisc := definition.NewDiscoverer(cfg.SchemaPath)
	methodsDir := filepath.Join(cfg.SchemaPath, "methods")

	exec := executor.New(rt, registry, modelDisc, defDisc, methodsDir, nil)

	methods, err := exec.ListMethods(defName)
	if err != nil {
		return err
	}

	if len(methods) == 0 {
		fmt.Printf("No methods found for definition %q.\n", defName)
		return nil
	}

	// Group by kind
	grouped := map[string][]executor.MethodStatus{}
	for _, m := range methods {
		grouped[m.Kind] = append(grouped[m.Kind], m)
	}

	// Display in consistent order
	kindOrder := []string{"action", "qualification", "attribute", "codegen"}
	fmt.Printf("Methods for definition %q:\n\n", defName)

	for _, kind := range kindOrder {
		ms, ok := grouped[kind]
		if !ok {
			continue
		}
		sort.Slice(ms, func(i, j int) bool { return ms[i].Name < ms[j].Name })

		fmt.Printf("  %s:\n", kind)
		for _, m := range ms {
			impl := " "
			if m.HasImplementation {
				impl = "*"
			}
			desc := ""
			if m.Description != "" {
				desc = " — " + m.Description
			}
			fmt.Printf("    [%s] %s%s\n", impl, m.Name, desc)
		}
		fmt.Println()
	}

	fmt.Println("  [*] = has .clj implementation")
	return nil
}
