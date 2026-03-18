package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
	"pudl/internal/model"
)

// modelShowCmd represents the model show command
var modelShowCmd = &cobra.Command{
	Use:   "show <model-name>",
	Short: "Display model details",
	Long: `Display detailed information about a model including metadata,
methods, sockets, and authentication configuration.

Examples:
    pudl model show pudl/model/examples.#EC2InstanceModel
    pudl model show pudl/model/examples.#SimpleModel`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeModelNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runModelShowCommand(args[0])
	},
}

func init() {
	modelCmd.AddCommand(modelShowCmd)
}

func runModelShowCommand(modelName string) error {
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	discoverer := model.NewDiscoverer(cfg.SchemaPath)
	m, err := discoverer.GetModel(modelName)
	if err != nil {
		return errors.NewInputError(
			fmt.Sprintf("Model not found: %s", modelName),
			"Check available models with 'pudl model list'",
		)
	}

	// Header
	fmt.Printf("Model: %s\n", m.Name)
	fmt.Println(strings.Repeat("-", 60))
	fmt.Println()

	// Metadata
	fmt.Println("Metadata:")
	fmt.Printf("  Name:        %s\n", m.Metadata.Name)
	fmt.Printf("  Description: %s\n", m.Metadata.Description)
	fmt.Printf("  Category:    %s\n", m.Metadata.Category)
	if m.Metadata.Icon != "" {
		fmt.Printf("  Icon:        %s\n", m.Metadata.Icon)
	}
	fmt.Printf("  Package:     %s\n", m.Package)
	fmt.Printf("  File:        %s\n", m.FilePath)
	fmt.Println()

	// Auth
	if m.Auth != nil {
		fmt.Println("Authentication:")
		fmt.Printf("  Method: %s\n", m.Auth.Method)
		fmt.Println()
	}

	// Methods grouped by kind
	if len(m.Methods) > 0 {
		fmt.Println("Methods:")
		printMethodsByKind(m.Methods, "action", "Actions")
		printMethodsByKind(m.Methods, "qualification", "Qualifications")
		printMethodsByKind(m.Methods, "attribute", "Attributes")
		printMethodsByKind(m.Methods, "codegen", "Code Generation")
	}

	// Sockets
	if len(m.Sockets) > 0 {
		fmt.Println("Sockets:")
		printSocketsByDirection(m.Sockets, "input", "Inputs")
		printSocketsByDirection(m.Sockets, "output", "Outputs")
	}

	// Lifecycle preview
	fmt.Println("Lifecycle Preview:")
	for name, method := range m.Methods {
		if method.Kind == "action" {
			lc, err := model.ResolveLifecycle(m, name)
			if err != nil {
				continue
			}
			if len(lc.Qualifications) > 0 {
				fmt.Printf("  %s: %s -> %s\n",
					name,
					strings.Join(lc.Qualifications, " + "),
					lc.Action)
			}
		}
	}
	fmt.Println()

	return nil
}

func printMethodsByKind(methods map[string]model.Method, kind, label string) {
	var names []string
	for name, m := range methods {
		if m.Kind == kind {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return
	}

	sort.Strings(names)
	fmt.Printf("  %s:\n", label)
	for _, name := range names {
		m := methods[name]
		fmt.Printf("    %s", name)
		if m.Description != "" {
			fmt.Printf(" - %s", m.Description)
		}
		fmt.Println()

		details := []string{}
		if m.Timeout != "" && m.Timeout != "5m" {
			details = append(details, fmt.Sprintf("timeout:%s", m.Timeout))
		}
		if m.Retries > 0 {
			details = append(details, fmt.Sprintf("retries:%d", m.Retries))
		}
		if len(m.Blocks) > 0 {
			details = append(details, fmt.Sprintf("blocks:[%s]", strings.Join(m.Blocks, ", ")))
		}
		if len(details) > 0 {
			fmt.Printf("      %s\n", strings.Join(details, "  "))
		}
	}
}

func printSocketsByDirection(sockets map[string]model.Socket, direction, label string) {
	var names []string
	for name, s := range sockets {
		if s.Direction == direction {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return
	}

	sort.Strings(names)
	fmt.Printf("  %s:\n", label)
	for _, name := range names {
		s := sockets[name]
		reqStr := ""
		if !s.Required {
			reqStr = " (optional)"
		}
		fmt.Printf("    %s%s", name, reqStr)
		if s.Description != "" {
			fmt.Printf(" - %s", s.Description)
		}
		fmt.Println()
	}
}

// completeModelNames returns a completion function for model names
func completeModelNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	discoverer := model.NewDiscoverer(cfg.SchemaPath)
	models, err := discoverer.ListModels()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	for _, m := range models {
		if toComplete == "" || strings.HasPrefix(m.Name, toComplete) {
			completions = append(completions, m.Name+"\t"+m.Metadata.Description)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}
