package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chazu/pudl/internal/bundle"
	"github.com/spf13/cobra"
)

var exportBundle, initBundle string
var exportBundleMaxBytes, initBundleMaxBytes int64

func init() {
	exportCmd.Flags().StringVar(&exportBundle, "bundle", "", "Write a verified portable workspace bundle")
	exportCmd.Flags().Int64Var(&exportBundleMaxBytes, "max-bundle-bytes", bundle.DefaultMaxBytes, "Maximum uncompressed bundle bytes")
	initCmd.Flags().StringVar(&initBundle, "from-bundle", "", "Restore verified history into a new local workspace; approvals require replanning")
	initCmd.Flags().Int64Var(&initBundleMaxBytes, "max-bundle-bytes", bundle.DefaultMaxBytes, "Maximum uncompressed restored bundle bytes")
}

func runExportBundle(cmd *cobra.Command) error {
	if exportID != "" || exportSchema != "" || exportOrigin != "" || exportOutput != "" || exportAllowPartial || cmd.Flags().Changed("format") {
		return fmt.Errorf("--bundle exports the whole workspace and cannot be combined with entry filters, --output, --format or --allow-partial")
	}
	root := effectivePudlDir()
	if err := bundle.Export(cmd.Context(), root, exportBundle, exportBundleMaxBytes); err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(map[string]any{"bundle": exportBundle, "version": bundle.Version, "complete": true})
	}
	fmt.Fprintf(outw(), "Workspace bundle written: %s\n", exportBundle)
	return nil
}

func runRestoreBundle(cmd *cobra.Command) error {
	if initForce || initGlobal {
		return fmt.Errorf("--from-bundle restores only into a new local workspace; --force and --global are not supported")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root := filepath.Join(cwd, ".pudl")
	if err := bundle.Restore(cmd.Context(), initBundle, root, initBundleMaxBytes); err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(map[string]any{"path": root, "mode": "workspace", "restored": true, "approvals_require_replanning": true})
	}
	fmt.Fprintf(outw(), "Workspace restored: %s\nPending approvals require replanning; live status is unknown until re-observed.\n", root)
	return nil
}
