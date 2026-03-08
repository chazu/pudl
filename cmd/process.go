package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	pudlcue "pudl/internal/cue"
	"pudl/internal/errors"
	"pudl/internal/glojure"
)

// processCmd represents the process command
var processCmd = &cobra.Command{
	Use:   "process <cue-file>",
	Short: "Process a CUE file with custom functions",
	Long: `Process a CUE file, executing custom functions and performing unification.

This command takes a CUE file as input, processes any custom functions defined
within it (such as op.#Uppercase, op.#Lowercase, op.#Concat), and outputs the
unified result.

The CUE file should import the "op" package to use custom functions:

    import "op"

    greeting: op.#Uppercase & {
        args: ["hello, world!"]
    }

Example usage:
    pudl process example.cue
    pudl process simple_test.cue`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Create error handler for CLI context
		errorHandler := errors.NewCLIErrorHandler(true)

		// Run the process command and handle any errors
		if err := runProcessCommand(cmd, args); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

// runProcessCommand contains the actual process logic with structured error handling
func runProcessCommand(cmd *cobra.Command, args []string) error {
	filename := args[0]

	// Validate file extension
	if !strings.HasSuffix(filename, ".cue") {
		return errors.NewInvalidFormatError("cue", []string{"cue"})
	}

	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return errors.NewFileNotFoundError(filename)
	}

	// Initialize Glojure runtime and function registry
	rt := glojure.New()
	if err := rt.Init(); err != nil {
		return fmt.Errorf("failed to initialize Glojure runtime: %w", err)
	}

	registry := glojure.NewRegistry(rt)
	if err := glojure.RegisterBuiltins(registry); err != nil {
		return fmt.Errorf("failed to register builtin functions: %w", err)
	}

	// Process the file
	processor := pudlcue.NewCUEProcessor(registry)
	if err := processor.ProcessFile(filename); err != nil {
		return errors.WrapError(errors.ErrCodeParsingFailed, "Error processing CUE file", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(processCmd)
}
