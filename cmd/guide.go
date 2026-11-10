package cmd

import (
	"embed"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// guideText holds one plain-text file per topic, cmd/guide/<topic>.txt.
//
//go:embed guide/*.txt
var guideText embed.FS

type guideTopic struct {
	name    string
	summary string
}

// guideTopics is the topic index, in reading order.
var guideTopics = []guideTopic{
	{"overview", "What pudl is, the loop, the terms, where state lives"},
	{"observe", "Collectors, import, fan-out, scope and completeness, snapshots"},
	{"schemas", "Types, identity, sensitive fields, ownership, review"},
	{"policies", "Per-resource compliance rules and their gotchas"},
	{"checks", "Checks, findings, waivers, exit codes and CI"},
	{"query", "Facts, Datalog rules and pudl query"},
	{"doctor", "Diagnose and repair the workspace"},
	{"packs", "The bundled GCP, AWS, Pulumi and tutorial packs"},
	{"agents", "JSON contracts and conventions for agents"},
}

var guideCmd = &cobra.Command{
	Use:   "guide [topic]",
	Short: "Quick-reference guide for agents and humans",
	Long: `Print usage guides for pudl features and concepts.

Run 'pudl guide' with no arguments to see all available topics.
Run 'pudl guide <topic>' to read a specific guide.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			printGuideIndex()
			return
		}
		text, err := guideTopicText(args[0])
		if err != nil {
			fmt.Fprintf(errw(), "pudl guide: unknown topic %q\n", args[0])
			fmt.Fprintln(errw(), "Run 'pudl guide' for a list of topics.")
			os.Exit(2)
		}
		fmt.Fprint(outw(), text)
	},
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		names := make([]string, 0, len(guideTopics))
		for _, t := range guideTopics {
			names = append(names, t.name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	},
}

func init() {
	rootCmd.AddCommand(guideCmd)
}

// guideTopicText returns a listed topic's text.
func guideTopicText(name string) (string, error) {
	for _, t := range guideTopics {
		if t.name == name {
			b, err := guideText.ReadFile("guide/" + name + ".txt")
			return string(b), err
		}
	}
	return "", fmt.Errorf("unknown topic %q", name)
}

func printGuideIndex() {
	fmt.Fprintln(outw(), "pudl guide — quick reference for agents and humans")
	fmt.Fprintln(outw())
	for _, t := range guideTopics {
		fmt.Fprintf(outw(), "  pudl guide %-10s %s\n", t.name, t.summary)
	}
	fmt.Fprintln(outw())
	fmt.Fprintln(outw(), "Agents: start with 'pudl prime'. Commands and flags: 'pudl help --json'.")
}
