package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	hooksHarness string
	hooksScope   string
	hooksDryRun  bool
)

// hookEntry is one Claude Code hook event → command mapping we manage.
type hookEntry struct {
	event   string // "SessionStart", "Stop", ...
	command string
	why     string
}

// managedHooks are the hooks pudl installs for the memory loop. The Stop hook runs
// only the deterministic curator — NOT the reflect cycle — because reflect shells
// out to the coding agent, and running that on every Stop risks recursion and
// cost. Run the full cycle ('pudl memory cycle') from a scheduler instead.
var managedHooks = []hookEntry{
	{event: "SessionStart", command: "pudl memory context", why: "inject ranked prior knowledge"},
	{event: "Stop", command: "pudl facts curate", why: "advance maturity from feedback (deterministic, no LLM)"},
}

var hooksCmd = &cobra.Command{
	Use:   "hooks",
	Short: "Print or install harness hooks for the agent-memory loop",
	Long: `Generate the harness hook configuration that wires the memory loop:

  SessionStart -> pudl memory context   (inject ranked prior knowledge)
  Stop         -> pudl facts curate     (advance maturity from feedback)

The Stop hook deliberately runs only the deterministic curator, not the reflect
cycle ('pudl memory cycle'), because reflect invokes your coding agent — running
that on every Stop risks recursion and cost. Trigger the full cycle from cron or a
scheduled agent instead.

'print' is non-destructive (default). 'install' merges into your settings with a
backup; it is idempotent.`,
	Run: func(cmd *cobra.Command, args []string) { cmd.Help() },
}

var hooksPrintCmd = &cobra.Command{
	Use:   "print",
	Short: "Print the hook configuration snippet (non-destructive)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if hooksHarness != "claude-code" {
			return fmt.Errorf("unsupported harness %q (only claude-code is supported)", hooksHarness)
		}
		snippet := map[string]interface{}{"hooks": buildHooksMap()}
		out, _ := json.MarshalIndent(snippet, "", "  ")
		fmt.Println(string(out))
		fmt.Fprintln(os.Stderr, "\nMerge this into your Claude Code settings.json, or run: pudl hooks install")
		return nil
	},
}

var hooksInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Merge the hooks into Claude Code settings.json (with backup)",
	Long: `Merge the memory-loop hooks into a Claude Code settings.json. Idempotent: an
already-present hook command is not duplicated. Writes a .bak backup before
changing an existing file.

--scope user    ~/.claude/settings.json (default)
--scope project ./.claude/settings.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if hooksHarness != "claude-code" {
			return fmt.Errorf("unsupported harness %q (only claude-code is supported)", hooksHarness)
		}
		path, err := claudeSettingsPath(hooksScope)
		if err != nil {
			return err
		}

		settings := map[string]interface{}{}
		if data, err := os.ReadFile(path); err == nil {
			if err := json.Unmarshal(data, &settings); err != nil {
				return fmt.Errorf("%s is not valid JSON: %w", path, err)
			}
		}

		added := mergeHooks(settings)
		if len(added) == 0 {
			fmt.Printf("All memory-loop hooks already present in %s — nothing to do.\n", path)
			return nil
		}

		out, _ := json.MarshalIndent(settings, "", "  ")
		if hooksDryRun {
			fmt.Printf("[dry-run] would update %s, adding: %v\n\n%s\n", path, added, string(out))
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if existing, err := os.ReadFile(path); err == nil {
			if err := os.WriteFile(path+".bak", existing, 0o644); err != nil {
				return fmt.Errorf("failed to write backup: %w", err)
			}
		}
		if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Printf("Updated %s (added: %v)\n", path, added)
		return nil
	},
}

// buildHooksMap builds the Claude Code hooks map for the managed hooks.
func buildHooksMap() map[string]interface{} {
	m := map[string]interface{}{}
	for _, h := range managedHooks {
		m[h.event] = []interface{}{
			map[string]interface{}{
				"hooks": []interface{}{
					map[string]interface{}{"type": "command", "command": h.command},
				},
			},
		}
	}
	return m
}

// mergeHooks adds any missing managed hooks into settings["hooks"], idempotently.
// Returns the list of commands actually added.
func mergeHooks(settings map[string]interface{}) []string {
	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
		settings["hooks"] = hooks
	}

	var added []string
	for _, h := range managedHooks {
		groups, _ := hooks[h.event].([]interface{})
		if hookCommandPresent(groups, h.command) {
			continue
		}
		groups = append(groups, map[string]interface{}{
			"hooks": []interface{}{
				map[string]interface{}{"type": "command", "command": h.command},
			},
		})
		hooks[h.event] = groups
		added = append(added, h.command)
	}
	return added
}

// hookCommandPresent reports whether command already appears in any group's hooks.
func hookCommandPresent(groups []interface{}, command string) bool {
	for _, g := range groups {
		gm, _ := g.(map[string]interface{})
		inner, _ := gm["hooks"].([]interface{})
		for _, hh := range inner {
			hm, _ := hh.(map[string]interface{})
			if c, _ := hm["command"].(string); c == command {
				return true
			}
		}
	}
	return false
}

// claudeSettingsPath returns the settings.json path for the requested scope.
func claudeSettingsPath(scope string) (string, error) {
	switch scope {
	case "user", "":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".claude", "settings.json"), nil
	case "project":
		return filepath.Join(".claude", "settings.json"), nil
	default:
		return "", fmt.Errorf("unknown scope %q (use user or project)", scope)
	}
}

func init() {
	rootCmd.AddCommand(hooksCmd)
	hooksCmd.AddCommand(hooksPrintCmd)
	hooksCmd.AddCommand(hooksInstallCmd)

	hooksCmd.PersistentFlags().StringVar(&hooksHarness, "harness", "claude-code", "Target harness (claude-code)")
	hooksInstallCmd.Flags().StringVar(&hooksScope, "scope", "user", "Where to install: user or project")
	hooksInstallCmd.Flags().BoolVar(&hooksDryRun, "dry-run", false, "Preview without writing")
}
