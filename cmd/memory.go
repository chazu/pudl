package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
)

var (
	memoryContextTask  string
	memoryContextLimit int
	memoryInitForce    bool
	memoryInitReflect  string
)

// defaultReflectCommand is the agent command the reflect target uses when none is
// configured. It must be a tool-calling coding agent (it calls pudl itself), not a
// raw completion API.
const defaultReflectCommand = "claude -p"

// memoryCmd groups the self-improvement loop commands: recall context for an
// agent (Generator), scaffold the mu cycle targets, and run the cycle.
var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Agent self-improvement loop: recall context and run the maturity cycle",
	Long: `Commands for the pudl agent-memory loop.

pudl is the substrate (it stores and scores facts); it never reflects or decides.
The loop's reasoning runs as mu pith targets that call back into this CLI:

  context   Print ranked promoted observations to inject into an agent (read-only).
  init      Write a starter mu cycle (~/.pudl/mu.cue): reflect -> curate.
  cycle     Run the mu memory cycle (mu -C <pudl dir> build //memory:cycle).

Wire these into your harness with 'pudl hooks'.`,
	Run: func(cmd *cobra.Command, args []string) { cmd.Help() },
}

var memoryContextCmd = &cobra.Command{
	Use:   "context",
	Short: "Print ranked promoted observations for agent context injection",
	Long: `Print promoted observations ranked for injection into an agent's context. With
--task, results are keyword-matched (FTS5) and ranked by decayed worth; without it,
the highest-decayed-worth promoted observations are returned.

This is the read side of the loop (the Generator). It makes no model calls. Use it
from a SessionStart / UserPromptSubmit hook (see 'pudl hooks').

Examples:
    pudl memory context
    pudl memory context --task "rate limiting auth" --limit 10
    pudl memory context --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := database.NewCatalogDB(config.GetPudlDir())
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()

		items, err := db.MemoryContext(memoryContextTask, memoryContextLimit)
		if err != nil {
			return err
		}

		if jsonOutput {
			out, _ := json.MarshalIndent(items, "", "  ")
			fmt.Println(string(out))
			return nil
		}
		if len(items) == 0 {
			return nil // nothing to inject; stay quiet so hooks add no noise
		}

		fmt.Println("# Relevant prior knowledge (pudl memory)")
		fmt.Println()
		for _, it := range items {
			var a map[string]interface{}
			_ = json.Unmarshal([]byte(it.Args), &a)
			kind, _ := a["kind"].(string)
			desc, _ := a["description"].(string)
			scope, _ := a["scope"].(string)
			line := "- "
			if kind != "" {
				line += "[" + kind + "] "
			}
			line += desc
			if scope != "" {
				line += " (" + scope + ")"
			}
			fmt.Printf("%s  _worth %.2f_\n", line, it.DecayedWorth)
		}
		return nil
	},
}

var memoryInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write the starter mu cycle targets to <pudl dir>/mu.cue",
	Long: `Write a starter mu workspace (` + "`mu.cue`" + `) into the pudl config directory
defining the memory cycle targets. Run it with 'pudl memory cycle'.

The cycle is reflect -> curate:
  //memory:reflect  invokes your coding agent to distill recent work into
                    observations (pudl facts observe ...)
  //memory:curate   deterministically advances maturity from feedback
                    (pudl facts curate)

The reflect agent command is configurable (precedence: --reflect-cmd > the
reflect_command config key > "claude -p"); the prompt is appended as a quoted
argument, so it must be a tool-calling coding agent that can run pudl, not a raw
completion API. The written mu.cue is yours to edit afterward. Use --force to
overwrite.

Examples:
    pudl memory init
    pudl memory init --reflect-cmd "aider --message"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := config.GetPudlDir()
		path := filepath.Join(dir, "mu.cue")
		if _, err := os.Stat(path); err == nil && !memoryInitForce {
			return fmt.Errorf("%s already exists (use --force to overwrite)", path)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}

		// Reflect command precedence: --reflect-cmd flag > config > default.
		reflectCmd := memoryInitReflect
		if reflectCmd == "" {
			if cfg, err := config.Load(); err == nil && cfg.ReflectCommand != "" {
				reflectCmd = cfg.ReflectCommand
			}
		}
		if reflectCmd == "" {
			reflectCmd = defaultReflectCommand
		}

		if err := os.WriteFile(path, []byte(memoryCycleWorkspace(reflectCmd)), 0o644); err != nil {
			return fmt.Errorf("failed to write %s: %w", path, err)
		}
		fmt.Printf("Wrote %s (reflect: %s)\n", path, reflectCmd)
		fmt.Println("Run the cycle with: pudl memory cycle")
		return nil
	},
}

var memoryCycleCmd = &cobra.Command{
	Use:   "cycle",
	Short: "Run the mu memory cycle (mu build --config <pudl dir>/mu.cue //memory:cycle)",
	Long: `Run the memory cycle by invoking mu rooted at the pudl config directory via
mu's --config flag (which roots the build at that mu.cue's directory):

    mu build --config <pudl dir>/mu.cue --no-cache //memory:cycle

Requires mu on PATH and a cycle workspace (run 'pudl memory init' first). The
cycle is non-hermetic (it reads session files and the live store), so it always
runs uncached.

Scheduling is out of scope: trigger this from a hook ('pudl hooks'), cron, or a
scheduled agent.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := config.GetPudlDir()
		muCue := filepath.Join(dir, "mu.cue")
		if _, err := os.Stat(muCue); err != nil {
			return fmt.Errorf("no cycle workspace at %s — run 'pudl memory init' first", muCue)
		}
		if _, err := exec.LookPath("mu"); err != nil {
			return fmt.Errorf("mu not found on PATH; install mu to run the cycle")
		}
		muArgs := []string{"build", "--config", muCue, "--no-cache", "//memory:cycle"}
		c := exec.Command("mu", muArgs...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin
		if err := c.Run(); err != nil {
			return fmt.Errorf("mu cycle failed: %w", err)
		}
		return nil
	},
}

// memoryCycleWorkspace returns the starter mu workspace written by
// `pudl memory init`, with reflectCmd as the reflect target's agent command (the
// prompt is appended as a quoted argument). The reflect step is agent-native (it
// shells out to a coding agent), so no external API key is required; curate is
// fully deterministic. The user owns the file and can customize it afterward.
func memoryCycleWorkspace(reflectCmd string) string {
	return `// pudl memory cycle — starter targets written by ` + "`pudl memory init`" + `.
//
// Run with: pudl memory cycle   (= mu build --config ~/.pudl/mu.cue --no-cache //memory:cycle)
//
// reflect -> curate. The reflect command is configurable via
// 'pudl memory init --reflect-cmd' or the reflect_command config key; edit it here
// freely (it must be a tool-calling coding agent that can run pudl, not a raw API).
package mu

targets: [{
	target:  "//memory:reflect"
	sources: []
	// Invoke the coding agent to distill recent work into observations. This is
	// the agent-native step: it reuses your existing agent, no API key.
	plan: [{
		id: "reflect"
		command: ["sh", "-c", """
			` + reflectCmd + ` 'Review my recent work in this repository. For each durable,
			reusable insight — a pattern, an obstacle, an anti-pattern, a lesson —
			record it with:
			  pudl facts observe "<concise description>" --kind <kind> --scope <repo:path> --source reflector
			Valid kinds: fact, obstacle, pattern, antipattern, suggestion, bug, opportunity.
			Only record insights worth remembering across sessions. Do nothing else.'
			"""]
		outputs: []
	}, "action/emit"]
}, {
	target:  "//memory:curate"
	sources: []
	deps: ["//memory:reflect"]
	// Deterministic maturity advancement from accumulated feedback. No LLM.
	plan: [{
		id: "curate"
		command: ["sh", "-c", "pudl facts curate"]
		outputs: []
	}, "action/emit"]
}, {
	target:  "//memory:cycle"
	sources: []
	deps: ["//memory:curate"]
	plan: [{
		id: "cycle"
		command: ["sh", "-c", "echo 'pudl memory cycle complete'"]
		outputs: []
	}, "action/emit"]
}]
`
}

func init() {
	rootCmd.AddCommand(memoryCmd)
	memoryCmd.AddCommand(memoryContextCmd)
	memoryCmd.AddCommand(memoryInitCmd)
	memoryCmd.AddCommand(memoryCycleCmd)

	memoryContextCmd.Flags().StringVar(&memoryContextTask, "task", "", "Keyword query to rank context by relevance")
	memoryContextCmd.Flags().IntVar(&memoryContextLimit, "limit", 10, "Maximum observations to surface (0 = no limit)")
	memoryInitCmd.Flags().BoolVar(&memoryInitForce, "force", false, "Overwrite an existing mu.cue")
	memoryInitCmd.Flags().StringVar(&memoryInitReflect, "reflect-cmd", "", "Agent command for the reflect target (default: reflect_command config, else \"claude -p\")")
}
