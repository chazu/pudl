package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/definition"
	"pudl/internal/errors"
)

var definitionValidateCmd = &cobra.Command{
	Use:   "validate [name]",
	Short: "Validate definitions against their models",
	Long: `Validate one or all definitions against their model schemas.

Without arguments, validates all definitions. With a name argument,
validates only the specified definition.

Examples:
    pudl definition validate
    pudl def validate prod_instance`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeDefinitionNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return runDefinitionValidateOne(args[0])
		}
		return runDefinitionValidateAll()
	},
}

func init() {
	definitionCmd.AddCommand(definitionValidateCmd)
}

func runDefinitionValidateOne(name string) error {
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	validator := definition.NewValidator(cfg.SchemaPath)
	result, err := validator.ValidateDefinition(name)
	if err != nil {
		return errors.NewInputError(
			fmt.Sprintf("Definition not found: %s", name),
			"Check available definitions with 'pudl definition list'",
		)
	}

	printValidationResult(result)
	return nil
}

func runDefinitionValidateAll() error {
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	validator := definition.NewValidator(cfg.SchemaPath)
	results, err := validator.ValidateAll()
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to validate definitions", err)
	}

	if len(results) == 0 {
		fmt.Println("No definitions found to validate.")
		return nil
	}

	passCount := 0
	failCount := 0

	for _, r := range results {
		printValidationResult(&r)
		if r.Valid {
			passCount++
		} else {
			failCount++
		}
	}

	fmt.Printf("\nResults: %d passed, %d failed, %d total\n", passCount, failCount, len(results))

	if failCount > 0 {
		return errors.NewValidationError("definitions", nil, fmt.Errorf("%d definitions failed validation", failCount))
	}
	return nil
}

func printValidationResult(r *definition.ValidationResult) {
	if r.Valid {
		fmt.Printf("  PASS  %s\n", r.Name)
	} else {
		fmt.Printf("  FAIL  %s\n", r.Name)
		for _, e := range r.Errors {
			fmt.Printf("        %s\n", e)
		}
	}
}
