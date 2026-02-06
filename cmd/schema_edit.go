package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
)

// schemaEditCmd represents the schema edit command
var schemaEditCmd = &cobra.Command{
	Use:   "edit <path>",
	Short: "Open a schema file in your editor",
	Long: `Open a schema file in your configured editor.

The path can optionally include a definition name to position the cursor at that
definition (supported for vim/nvim editors).

Examples:
    pudl schema edit aws/ec2:#Instance    # Opens the file and positions at #Instance if possible
    pudl schema edit aws/ec2              # Opens the aws/ec2.cue file`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSchemaEditCommand(args[0])
	},
}

func init() {
	schemaCmd.AddCommand(schemaEditCmd)
}

// runSchemaEditCommand opens a schema file in the user's editor
func runSchemaEditCommand(pathArg string) error {
	// Parse the path argument to extract package path and optional definition name
	packagePath, definitionName := parseSchemaEditPath(pathArg)

	// Load configuration to get schema path
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Resolve the file path
	// Schema files can be at:
	// 1. ~/.pudl/schema/<package_path>/<definition>.cue (created by pudl schema new)
	// 2. ~/.pudl/schema/<package_path>.cue (simple single-file schema)
	var filePath string
	var found bool

	// If definition name is provided, first try the definition-based path
	if definitionName != "" {
		// Try: aws/ec2 + Instance -> aws/ec2/instance.cue
		defPath := filepath.Join(cfg.SchemaPath, packagePath, strings.ToLower(definitionName)+".cue")
		if _, err := os.Stat(defPath); err == nil {
			filePath = defPath
			found = true
		}
	}

	// If not found, try the simple package path
	if !found {
		simplePath := filepath.Join(cfg.SchemaPath, packagePath+".cue")
		if _, err := os.Stat(simplePath); err == nil {
			filePath = simplePath
			found = true
		}
	}

	// If still not found, return error with helpful message
	if !found {
		if definitionName != "" {
			return errors.NewFileNotFoundError(
				fmt.Sprintf("%s/%s.cue or %s.cue",
					filepath.Join(cfg.SchemaPath, packagePath),
					strings.ToLower(definitionName),
					filepath.Join(cfg.SchemaPath, packagePath)))
		}
		return errors.NewFileNotFoundError(filepath.Join(cfg.SchemaPath, packagePath+".cue"))
	}

	// Get the editor from $EDITOR environment variable
	editor := os.Getenv("EDITOR")
	if editor == "" {
		// Try vi first, then nano as fallback
		editor = "vi"
	}

	// Build the command arguments
	var args []string

	// If a definition name was specified and we're using vim/nvim, try to position the cursor
	if definitionName != "" && isVimEditor(editor) {
		// Use vim's +/pattern command to search for the definition
		args = append(args, fmt.Sprintf("+/^#%s:", definitionName))
	}

	args = append(args, filePath)

	// Execute the editor
	editorCmd := exec.Command(editor, args...)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem,
			fmt.Sprintf("Failed to open editor '%s'", editor), err)
	}

	return nil
}

// parseSchemaEditPath parses the path argument for the edit command
// e.g., "aws/ec2:#Instance" -> ("aws/ec2", "Instance")
// e.g., "aws/ec2" -> ("aws/ec2", "")
func parseSchemaEditPath(path string) (packagePath, definitionName string) {
	// Check if path contains :# for explicit definition name
	if idx := strings.Index(path, ":#"); idx != -1 {
		packagePath = path[:idx]
		definitionName = path[idx+2:] // Skip :#
		return
	}

	// No explicit definition
	packagePath = path
	definitionName = ""
	return
}

// isVimEditor checks if the editor is vim or nvim
func isVimEditor(editor string) bool {
	// Get the base name of the editor (in case it's a full path)
	baseName := filepath.Base(editor)
	return baseName == "vim" || baseName == "nvim" || baseName == "vi"
}
