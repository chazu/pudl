package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/packs"
)

var packCmd = &cobra.Command{
	Use:   "pack",
	Short: "Install bundled content packs (gcp, aws) into this workspace",
}

var packInstallCmd = &cobra.Command{
	Use:       "install <name>",
	Short:     "Install a content pack: schemas, policies, collectors, checks and fixtures",
	ValidArgs: packs.Names(),
	Args:      cobra.ExactArgs(1),
	Long: `Install a bundled content pack in the current initialized PUDL workspace,
or with --global in the personal ~/.pudl workspace (after pudl init --global).
No source checkout or download is needed. pudl records what it wrote in
<workspace>/.packs/<name>.json.

Installing again upgrades the pack: files you have not edited since pudl
installed them are replaced with this version's, and new files are added.
Files you edited, and files installed without a record (including installs
made with pudl example install before packs had their own command), are
conflicts: nothing is written and each is listed. --overwrite replaces them,
keeping your versions as <file>.bak.

The gcp pack includes draft schemas for the core GCP inventory,
best-practice policies, gcloud and fixture collectors, and three check
families: network hygiene, data protection and GKE posture.

The aws pack does the same for AWS, with aws CLI and fixture collectors and
three check families: network exposure, data protection and identity
posture.

Example:
    pudl init
    pudl pack install gcp`,
	RunE: func(cmd *cobra.Command, args []string) error {
		wroteJSON, err := installFileset(cmd, packs.Kind, args[0], packs.Install)
		if err != nil || wroteJSON {
			return err
		}
		switch args[0] {
		case "aws":
			fmt.Fprintln(outw(), "Next: pudl check aws-fixture-vpcs aws-fixture-subnets aws-fixture-security-groups aws-fixture-instances aws-fixture-network-exposure-checks (fixture; no cloud access)")
		case "gcp":
			fmt.Fprintln(outw(), "Next: pudl check gcp-fixture-firewalls gcp-fixture-addresses gcp-fixture-routers gcp-fixture-network-hygiene-checks (fixture; no cloud access)")
		}
		return nil
	},
}

var packListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the bundled content packs and whether this workspace has them",
	Long: `List the content packs this pudl ships. In a workspace (or with --global),
each pack's state compares the workspace's install record with this pudl's
version: installed (current), outdated (pudl pack install upgrades it) or
not-installed. The record says what pudl wrote; edits made since are not
checked here.`,
	Example: `  pudl pack list
  pudl pack list --json`,
	Args: cobra.NoArgs,
	RunE: runPackList,
}

type packListEntry struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
	State   string `json:"state,omitempty"`
}

func runPackList(cmd *cobra.Command, args []string) error {
	// Outside a workspace the packs are still listed, without a state.
	root, _ := filesetInstallRoot(cmd, packs.Kind.Noun)
	entries := []packListEntry{}
	for _, p := range packs.List() {
		entry := packListEntry{Name: p.Name, Summary: p.Summary}
		if root != "" {
			state, err := packs.State(root, p.Name)
			if err != nil {
				return err
			}
			entry.State = string(state)
		}
		entries = append(entries, entry)
	}
	if jsonOutput {
		return printJSON(map[string]any{"root": root, "packs": entries})
	}
	w := tabwriter.NewWriter(outw(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PACK\tSTATE\tSUMMARY")
	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\t%s\n", e.Name, orDash(e.State), e.Summary)
	}
	return w.Flush()
}

func init() {
	rootCmd.AddCommand(packCmd)
	packCmd.AddCommand(packInstallCmd, packListCmd)
	packInstallCmd.Flags().Bool("overwrite", false, "Replace files that differ from this version, keeping yours as <file>.bak")
}
