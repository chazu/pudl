package cmd

import (
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/cue"
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
		filename := args[0]
		
		// Validate file extension
		if !strings.HasSuffix(filename, ".cue") {
			log.Fatal("File must have .cue extension")
		}

		// Check if file exists
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			log.Fatalf("File %s does not exist", filename)
		}

		// Process the file
		processor := cue.NewCUEProcessor()
		if err := processor.ProcessFile(filename); err != nil {
			log.Fatalf("Error processing file: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(processCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// processCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// processCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
