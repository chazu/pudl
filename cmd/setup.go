package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/errors"
)

var (
	setupShell    string
	setupDryRun   bool
	setupUninstall bool
)

// setupCmd represents the setup command
var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set up shell integration for PUDL",
	Long: `Set up shell integration for PUDL by adding helpful aliases and functions
to your shell configuration files.

This command adds the following integrations:
- 'pcd' alias: Quick navigation to PUDL schema repository
- 'pudl-cd' function: Enhanced directory navigation with status
- Shell completion setup (if supported)

Supported shells:
- bash (.bashrc)
- zsh (.zshrc)
- fish (config.fish)

The setup will:
1. Detect your current shell automatically
2. Add PUDL integration snippets to the appropriate config file
3. Backup existing config before making changes
4. Provide instructions for activating the changes

Examples:
    pudl setup                     # Auto-detect shell and install
    pudl setup --shell bash        # Force bash setup
    pudl setup --dry-run           # Show what would be added
    pudl setup --uninstall         # Remove PUDL integration`,
	Run: func(cmd *cobra.Command, args []string) {
		// Create error handler for CLI context
		errorHandler := errors.NewCLIErrorHandler(true)

		// Run the setup command and handle any errors
		if err := runSetupCommand(cmd, args); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

// Shell integration snippets
const (
	bashIntegration = `
# PUDL Shell Integration
# Added by 'pudl setup' - Do not edit manually
export PUDL_SCHEMA_PATH="$HOME/.pudl/schema"

# Quick navigation to PUDL schema repository
alias pcd='cd "$PUDL_SCHEMA_PATH"'

# Enhanced PUDL directory navigation with git status
pudl-cd() {
    local pudl_path
    pudl_path=$(pudl git cd --shell 2>/dev/null)
    if [ $? -eq 0 ] && [ -n "$pudl_path" ]; then
        eval "$pudl_path"
        echo "📁 PUDL Schema Repository"
        if command -v git >/dev/null 2>&1 && [ -d .git ]; then
            echo "🌿 $(git branch --show-current 2>/dev/null || echo 'detached')"
            local status=$(git status --porcelain 2>/dev/null | wc -l | tr -d ' ')
            if [ "$status" -gt 0 ]; then
                echo "📝 $status uncommitted changes"
            else
                echo "✅ Repository clean"
            fi
        fi
    else
        echo "❌ PUDL not initialized. Run 'pudl init' first."
        return 1
    fi
}

# PUDL command completion (if available)
if command -v pudl >/dev/null 2>&1; then
    if pudl completion bash >/dev/null 2>&1; then
        eval "$(pudl completion bash)"
    fi
fi
# End PUDL Shell Integration
`

	zshIntegration = `
# PUDL Shell Integration
# Added by 'pudl setup' - Do not edit manually
export PUDL_SCHEMA_PATH="$HOME/.pudl/schema"

# Quick navigation to PUDL schema repository
alias pcd='cd "$PUDL_SCHEMA_PATH"'

# Enhanced PUDL directory navigation with git status
pudl-cd() {
    local pudl_path
    pudl_path=$(pudl git cd --shell 2>/dev/null)
    if [ $? -eq 0 ] && [ -n "$pudl_path" ]; then
        eval "$pudl_path"
        echo "📁 PUDL Schema Repository"
        if command -v git >/dev/null 2>&1 && [ -d .git ]; then
            echo "🌿 $(git branch --show-current 2>/dev/null || echo 'detached')"
            local status=$(git status --porcelain 2>/dev/null | wc -l | tr -d ' ')
            if [ "$status" -gt 0 ]; then
                echo "📝 $status uncommitted changes"
            else
                echo "✅ Repository clean"
            fi
        fi
    else
        echo "❌ PUDL not initialized. Run 'pudl init' first."
        return 1
    fi
}

# PUDL command completion (if available)
if command -v pudl >/dev/null 2>&1; then
    if pudl completion zsh >/dev/null 2>&1; then
        eval "$(pudl completion zsh)"
    fi
fi
# End PUDL Shell Integration
`

	fishIntegration = `
# PUDL Shell Integration
# Added by 'pudl setup' - Do not edit manually
set -gx PUDL_SCHEMA_PATH "$HOME/.pudl/schema"

# Quick navigation to PUDL schema repository
alias pcd='cd "$PUDL_SCHEMA_PATH"'

# Enhanced PUDL directory navigation with git status
function pudl-cd
    set pudl_path (pudl git cd --shell 2>/dev/null)
    if test $status -eq 0 -a -n "$pudl_path"
        eval $pudl_path
        echo "📁 PUDL Schema Repository"
        if command -v git >/dev/null 2>&1; and test -d .git
            echo "🌿 "(git branch --show-current 2>/dev/null; or echo 'detached')
            set status_count (git status --porcelain 2>/dev/null | wc -l | tr -d ' ')
            if test "$status_count" -gt 0
                echo "📝 $status_count uncommitted changes"
            else
                echo "✅ Repository clean"
            end
        end
    else
        echo "❌ PUDL not initialized. Run 'pudl init' first."
        return 1
    end
end

# PUDL command completion (if available)
if command -v pudl >/dev/null 2>&1
    if pudl completion fish >/dev/null 2>&1
        pudl completion fish | source
    end
end
# End PUDL Shell Integration
`
)

// runSetupCommand contains the actual setup logic
func runSetupCommand(cmd *cobra.Command, args []string) error {
	if setupUninstall {
		return runUninstallIntegration()
	}

	// Detect shell if not specified
	shell := setupShell
	if shell == "" {
		detectedShell, err := detectShell()
		if err != nil {
			return errors.NewSystemError("Failed to detect shell", err)
		}
		shell = detectedShell
	}

	// Validate shell
	if !isValidShell(shell) {
		return errors.NewInputError(
			fmt.Sprintf("Unsupported shell: %s", shell),
			"Supported shells: bash, zsh, fish")
	}

	// Get integration snippet
	snippet := getIntegrationSnippet(shell)
	if snippet == "" {
		return errors.NewSystemError("No integration available for shell: "+shell, nil)
	}

	if setupDryRun {
		return showDryRun(shell, snippet)
	}

	return installIntegration(shell, snippet)
}

// detectShell attempts to detect the user's current shell
func detectShell() (string, error) {
	// Check SHELL environment variable
	shell := os.Getenv("SHELL")
	if shell != "" {
		// Extract shell name from path
		shellName := filepath.Base(shell)
		switch shellName {
		case "bash":
			return "bash", nil
		case "zsh":
			return "zsh", nil
		case "fish":
			return "fish", nil
		}
	}

	// Fallback: check for common shell config files
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// Check in order of preference
	if _, err := os.Stat(filepath.Join(homeDir, ".zshrc")); err == nil {
		return "zsh", nil
	}
	if _, err := os.Stat(filepath.Join(homeDir, ".bashrc")); err == nil {
		return "bash", nil
	}
	if _, err := os.Stat(filepath.Join(homeDir, ".config", "fish", "config.fish")); err == nil {
		return "fish", nil
	}

	// Default fallback based on OS
	if runtime.GOOS == "darwin" {
		return "zsh", nil // macOS default
	}
	return "bash", nil // Linux default
}

// isValidShell checks if the shell is supported
func isValidShell(shell string) bool {
	validShells := []string{"bash", "zsh", "fish"}
	for _, valid := range validShells {
		if shell == valid {
			return true
		}
	}
	return false
}

// getIntegrationSnippet returns the integration snippet for the given shell
func getIntegrationSnippet(shell string) string {
	switch shell {
	case "bash":
		return bashIntegration
	case "zsh":
		return zshIntegration
	case "fish":
		return fishIntegration
	default:
		return ""
	}
}

// getConfigFile returns the config file path for the given shell
func getConfigFile(shell string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch shell {
	case "bash":
		return filepath.Join(homeDir, ".bashrc"), nil
	case "zsh":
		return filepath.Join(homeDir, ".zshrc"), nil
	case "fish":
		configDir := filepath.Join(homeDir, ".config", "fish")
		// Ensure fish config directory exists
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return "", err
		}
		return filepath.Join(configDir, "config.fish"), nil
	default:
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}
}

// showDryRun shows what would be added without making changes
func showDryRun(shell, snippet string) error {
	configFile, err := getConfigFile(shell)
	if err != nil {
		return errors.NewSystemError("Failed to get config file path", err)
	}

	fmt.Printf("🔍 Dry run mode - showing what would be added to %s:\n", configFile)
	fmt.Println()
	fmt.Println("📝 Integration snippet:")
	fmt.Println(strings.Repeat("-", 60))
	fmt.Print(snippet)
	fmt.Println(strings.Repeat("-", 60))
	fmt.Println()
	fmt.Println("💡 To actually install, run: pudl setup")
	fmt.Printf("💡 To install for specific shell: pudl setup --shell %s\n", shell)

	return nil
}

// installIntegration installs the shell integration
func installIntegration(shell, snippet string) error {
	configFile, err := getConfigFile(shell)
	if err != nil {
		return errors.NewSystemError("Failed to get config file path", err)
	}

	// Check if PUDL integration already exists
	if hasIntegration(configFile) {
		fmt.Printf("✅ PUDL shell integration already installed in %s\n", configFile)
		fmt.Println()
		fmt.Println("💡 To reinstall, first run: pudl setup --uninstall")
		return nil
	}

	// Create backup
	if err := createBackup(configFile); err != nil {
		return errors.NewSystemError("Failed to create backup", err)
	}

	// Add integration snippet
	if err := appendToFile(configFile, snippet); err != nil {
		return errors.NewSystemError("Failed to add integration", err)
	}

	fmt.Printf("✅ PUDL shell integration installed successfully!\n")
	fmt.Printf("📁 Config file: %s\n", configFile)
	fmt.Println()
	fmt.Println("🚀 New features available:")
	fmt.Println("   - 'pcd' alias: Quick navigation to schema repository")
	fmt.Println("   - 'pudl-cd' function: Enhanced navigation with git status")
	fmt.Println("   - Shell completion for pudl commands (if supported)")
	fmt.Println()
	fmt.Println("💡 To activate the changes:")
	fmt.Printf("   source %s\n", configFile)
	fmt.Println("   OR restart your terminal")

	return nil
}

// runUninstallIntegration removes PUDL shell integration
func runUninstallIntegration() error {
	shells := []string{"bash", "zsh", "fish"}
	removed := false

	for _, shell := range shells {
		configFile, err := getConfigFile(shell)
		if err != nil {
			continue // Skip if we can't get config file
		}

		if !hasIntegration(configFile) {
			continue // Skip if no integration found
		}

		// Create backup before removal
		if err := createBackup(configFile); err != nil {
			fmt.Printf("⚠️  Warning: Failed to backup %s: %v\n", configFile, err)
			continue
		}

		// Remove integration
		if err := removeIntegration(configFile); err != nil {
			fmt.Printf("❌ Failed to remove integration from %s: %v\n", configFile, err)
			continue
		}

		fmt.Printf("✅ Removed PUDL integration from %s\n", configFile)
		removed = true
	}

	if !removed {
		fmt.Println("ℹ️  No PUDL shell integration found to remove")
	} else {
		fmt.Println()
		fmt.Println("💡 Restart your terminal or source your config files to apply changes")
	}

	return nil
}

// hasIntegration checks if PUDL integration exists in the config file
func hasIntegration(configFile string) bool {
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return false
	}

	content, err := os.ReadFile(configFile)
	if err != nil {
		return false
	}

	return strings.Contains(string(content), "# PUDL Shell Integration")
}

// createBackup creates a backup of the config file
func createBackup(configFile string) error {
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return nil // No file to backup
	}

	backupFile := configFile + ".pudl-backup"
	content, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}

	return os.WriteFile(backupFile, content, 0644)
}

// appendToFile appends content to a file
func appendToFile(filename, content string) error {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	return err
}

// removeIntegration removes PUDL integration from a config file
func removeIntegration(configFile string) error {
	content, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	inPudlSection := false

	for _, line := range lines {
		if strings.Contains(line, "# PUDL Shell Integration") {
			inPudlSection = true
			continue
		}
		if strings.Contains(line, "# End PUDL Shell Integration") {
			inPudlSection = false
			continue
		}
		if !inPudlSection {
			newLines = append(newLines, line)
		}
	}

	// Remove trailing empty lines
	for len(newLines) > 0 && strings.TrimSpace(newLines[len(newLines)-1]) == "" {
		newLines = newLines[:len(newLines)-1]
	}

	newContent := strings.Join(newLines, "\n")
	if len(newContent) > 0 {
		newContent += "\n" // Ensure file ends with newline
	}

	return os.WriteFile(configFile, []byte(newContent), 0644)
}

func init() {
	// Add setup command to root
	rootCmd.AddCommand(setupCmd)

	// Add flags
	setupCmd.Flags().StringVar(&setupShell, "shell", "", "Target shell (bash, zsh, fish) - auto-detected if not specified")
	setupCmd.Flags().BoolVar(&setupDryRun, "dry-run", false, "Show what would be added without making changes")
	setupCmd.Flags().BoolVar(&setupUninstall, "uninstall", false, "Remove PUDL shell integration")
}
