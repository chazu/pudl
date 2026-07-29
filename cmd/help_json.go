package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type commandHelpJSON struct {
	Use         string            `json:"use"`
	Short       string            `json:"short,omitempty"`
	Long        string            `json:"long,omitempty"`
	Args        string            `json:"args,omitempty"`
	Flags       []flagHelpJSON    `json:"flags,omitempty"`
	Subcommands []commandHelpJSON `json:"subcommands,omitempty"`
}

type flagHelpJSON struct {
	Name       string `json:"name"`
	Shorthand  string `json:"shorthand,omitempty"`
	Usage      string `json:"usage,omitempty"`
	Type       string `json:"type"`
	Default    string `json:"default,omitempty"`
	Persistent bool   `json:"persistent,omitempty"`
}

func commandTreeJSON(cmd *cobra.Command) commandHelpJSON {
	out := commandHelpJSON{Use: cmd.Use, Short: cmd.Short, Long: strings.TrimSpace(cmd.Long)}
	if cmd.Args != nil {
		out.Args = "custom"
	}
	flags := make(map[string]flagHelpJSON)
	addFlags := func(fs *pflag.FlagSet, persistent bool) {
		if fs == nil {
			return
		}
		fs.VisitAll(func(f *pflag.Flag) {
			flags[f.Name] = flagHelpJSON{
				Name: f.Name, Shorthand: f.Shorthand, Usage: f.Usage,
				Type: f.Value.Type(), Default: f.DefValue, Persistent: persistent,
			}
		})
	}
	addFlags(cmd.LocalNonPersistentFlags(), false)
	addFlags(cmd.PersistentFlags(), true)
	addFlags(cmd.InheritedFlags(), true)
	for _, f := range flags {
		out.Flags = append(out.Flags, f)
	}
	sort.Slice(out.Flags, func(i, j int) bool { return out.Flags[i].Name < out.Flags[j].Name })

	children := cmd.Commands()
	sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
	for _, child := range children {
		if child.Hidden || child.Name() == "help" {
			continue
		}
		out.Subcommands = append(out.Subcommands, commandTreeJSON(child))
	}
	return out
}

func findCommandPath(args []string) (*cobra.Command, error) {
	current := rootCmd
	for _, name := range args {
		var found *cobra.Command
		for _, child := range current.Commands() {
			if child.Name() == name || contains(child.Aliases, name) {
				found = child
				break
			}
		}
		if found == nil {
			return nil, fmt.Errorf("unknown command path %q", strings.Join(args, " "))
		}
		current = found
	}
	return current, nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

var helpJSONCmd = &cobra.Command{
	Use:   "help [command]",
	Short: "Show help or the command tree",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		target, err := findCommandPath(args)
		if err != nil {
			return err
		}
		if !jsonOutput {
			return target.Help()
		}
		b, err := json.MarshalIndent(commandTreeJSON(target), "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(helpJSONCmd)
}
