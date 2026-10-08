package cmd

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/database"
)

var (
	snapshotModel     string
	snapshotLimit     int
	snapshotRelease   bool
	snapshotKeep      int
	snapshotOlderThan time.Duration
	snapshotDryRun    bool
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Inspect and retain observation snapshots",
	Long: `Observation snapshots are what a run saw.

Each populate or observe ingest records one snapshot: which model it was taken
for, by which run, in which workspace, from which source, and how many records it
holds. A snapshot ID is what "pudl run --from-catalog --catalog-scope <id>"
replays against.

Snapshots accumulate — one per run, plus one raw file per record — so they need
pruning. Pruning is explicit and never happens as a side effect of a run.

Examples:
    pudl snapshot list --model k8sPolicy
    pudl snapshot current k8sPolicy
    pudl snapshot retain snap_kisut-hipol
    pudl snapshot prune --model k8sPolicy --keep 5 --older-than 720h --dry-run`,
	Run: func(cmd *cobra.Command, args []string) { cmd.Help() },
}

// snapshotCatalog opens the catalog for a snapshot subcommand.
func snapshotCatalog() (*database.CatalogDB, error) {
	db, err := database.NewCatalogDB(effectivePudlDir())
	if err != nil {
		return nil, fmt.Errorf("open catalog: %w", err)
	}
	return db, nil
}

var snapshotListCmd = &cobra.Command{
	Use:          "list",
	Short:        "List observation snapshots, newest first",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := snapshotCatalog()
		if err != nil {
			return err
		}
		defer db.Close()

		snapshots, err := db.ListObserveSnapshots(snapshotModel, snapshotLimit)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(snapshots)
		}
		if len(snapshots) == 0 {
			fmt.Fprintln(outw(), "no snapshots recorded")
			return nil
		}
		w := tabwriter.NewWriter(outw(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SNAPSHOT\tMODEL\tSOURCE\tRECORDS\tRUN\tCREATED\tRETAINED")
		for _, s := range snapshots {
			retained := ""
			if s.Retained {
				retained = "yes"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
				s.SnapshotID, orDash(s.Model), orDash(s.Source), s.RecordCount,
				orDash(s.RunID), s.CreatedAt.Format(time.RFC3339), retained)
		}
		return w.Flush()
	},
}

var snapshotShowCmd = &cobra.Command{
	Use:          "show <snapshot-id>",
	Short:        "Show one snapshot's provenance and contents",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := snapshotCatalog()
		if err != nil {
			return err
		}
		defer db.Close()

		snapshot, err := db.GetObserveSnapshot(args[0])
		if err != nil {
			return err
		}
		entries, err := db.SnapshotRecordEntries(args[0])
		if err != nil {
			return err
		}
		if snapshot == nil && len(entries) == 0 {
			return fmt.Errorf("no such snapshot: %s", args[0])
		}

		pins, err := db.SnapshotPins(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(map[string]any{
				"snapshot": snapshot,
				"records":  len(entries),
				"pins":     pins,
			})
		}
		if snapshot == nil {
			// Predates the snapshot contract: still a valid replay scope, but its
			// model and workspace were never recorded and cannot be invented now.
			fmt.Fprintf(outw(), "%s\n  (recorded before snapshot provenance existed — no model, source or retention)\n", args[0])
		} else {
			fmt.Fprintf(outw(), "%s\n", snapshot.SnapshotID)
			fmt.Fprintf(outw(), "  model:     %s\n", orDash(snapshot.Model))
			fmt.Fprintf(outw(), "  run:       %s\n", orDash(snapshot.RunID))
			fmt.Fprintf(outw(), "  workspace: %s\n", orDash(snapshot.Workspace))
			fmt.Fprintf(outw(), "  source:    %s\n", orDash(snapshot.Source))
			fmt.Fprintf(outw(), "  origin:    %s\n", orDash(snapshot.Origin))
			fmt.Fprintf(outw(), "  targets:   %v\n", snapshot.Targets)
			fmt.Fprintf(outw(), "  created:   %s\n", snapshot.CreatedAt.Format(time.RFC3339))
			fmt.Fprintf(outw(), "  retained:  %t\n", snapshot.Retained)
			for _, pin := range pins {
				fmt.Fprintf(outw(), "  pin:       %s %s", pin.OwnerKind, pin.OwnerID)
				if pin.ExpiresAt != nil {
					fmt.Fprintf(outw(), " until %s", pin.ExpiresAt.Format(time.RFC3339))
				}
				fmt.Fprintln(outw())
			}
		}
		fmt.Fprintf(outw(), "  records:   %d\n", len(entries))
		return nil
	},
}

var snapshotCurrentCmd = &cobra.Command{
	Use:          "current <model>",
	Short:        "Show the newest observation snapshot for a model",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := snapshotCatalog()
		if err != nil {
			return err
		}
		defer db.Close()

		snapshot, err := db.CurrentObserveSnapshot(args[0])
		if err != nil {
			return err
		}
		if snapshot == nil {
			return fmt.Errorf("model %q has no recorded observation", args[0])
		}
		if jsonOutput {
			return printJSON(snapshot)
		}
		fmt.Fprintf(outw(), "%s  (%s, %d record(s), %s)\n",
			snapshot.SnapshotID, snapshot.Source, snapshot.RecordCount,
			snapshot.CreatedAt.Format(time.RFC3339))
		return nil
	},
}

var snapshotRetainCmd = &cobra.Command{
	Use:   "retain <snapshot-id>",
	Short: "Pin a snapshot against pruning (--release to unpin)",
	Long: `Add a manual pin so no retention policy removes this snapshot.
--release removes only the manual pin; approval and retained-report owners remain.
Reports preserve referenced snapshot evidence for 30 days after their last save.

Snapshots recorded before snapshot provenance existed cannot be pinned — they
carry no contract row — but they are never pruned either, for the same reason.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := snapshotCatalog()
		if err != nil {
			return err
		}
		defer db.Close()

		if err := db.RetainObserveSnapshot(args[0], !snapshotRelease); err != nil {
			return err
		}
		if jsonOutput {
			snapshot, err := db.GetObserveSnapshot(args[0])
			if err != nil {
				return err
			}
			return printJSON(map[string]any{"snapshot_id": args[0], "manual_pin": !snapshotRelease, "retained": snapshot.Retained})
		}
		if snapshotRelease {
			fmt.Fprintf(outw(), "released %s\n", args[0])
		} else {
			fmt.Fprintf(outw(), "retained %s\n", args[0])
		}
		return nil
	},
}

var snapshotPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove snapshots a retention policy no longer wants",
	Long: `Remove old observation snapshots, their records and their raw files.

--keep and --older-than are an AND: a snapshot goes only if it is outside the
newest N for its model AND older than the cutoff. That means --keep 0 alone
removes nothing, which is deliberate — a prune that empties a model's history
because a flag defaulted to zero is the more expensive surprise.

Records are shared between snapshots by content, so a record is removed only
when no remaining snapshot cites it. Retained snapshots, and snapshots recorded
before the snapshot contract existed, are never removed.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := snapshotCatalog()
		if err != nil {
			return err
		}
		defer db.Close()

		cfg, err := loadEffectiveConfig()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		opts := database.PruneOptions{
			Model:   snapshotModel,
			Keep:    snapshotKeep,
			DryRun:  snapshotDryRun,
			DataDir: cfg.DataPath,
		}
		if snapshotOlderThan > 0 {
			opts.OlderThan = time.Now().Add(-snapshotOlderThan)
		}

		result, err := db.PruneObserveSnapshotsContext(cmd.Context(), opts)
		if err != nil {
			return err
		}
		if !snapshotDryRun && result.Records > 0 {
			syncProjectionsQuietly(cmd.Context(), db)
		}
		if jsonOutput {
			return printJSON(result)
		}

		verb := "removed"
		if snapshotDryRun {
			verb = "would remove"
		}
		fmt.Fprintf(outw(), "%s %d snapshot(s), %d record(s), %d raw file(s)\n",
			verb, len(result.Snapshots), result.Records, result.FilesRemoved)
		for _, id := range result.Snapshots {
			fmt.Fprintf(outw(), "  - %s\n", id)
		}
		for _, path := range result.FilesSkipped {
			fmt.Fprintf(outw(), "  ! left in place (outside the data dir's raw/ tree): %s\n", path)
		}
		return nil
	},
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func init() {
	rootCmd.AddCommand(snapshotCmd)
	snapshotCmd.AddCommand(snapshotListCmd, snapshotShowCmd, snapshotCurrentCmd,
		snapshotRetainCmd, snapshotPruneCmd)

	snapshotListCmd.Flags().StringVar(&snapshotModel, "model", "", "only this model's snapshots")
	snapshotListCmd.Flags().IntVar(&snapshotLimit, "limit", 20, "maximum rows (0 = no limit)")

	snapshotRetainCmd.Flags().BoolVar(&snapshotRelease, "release", false, "unpin instead of pinning")

	snapshotPruneCmd.Flags().StringVar(&snapshotModel, "model", "", "only this model's snapshots")
	snapshotPruneCmd.Flags().IntVar(&snapshotKeep, "keep", 10, "always keep this many newest per model")
	snapshotPruneCmd.Flags().DurationVar(&snapshotOlderThan, "older-than", 0, "only remove snapshots older than this (e.g. 720h)")
	snapshotPruneCmd.Flags().BoolVar(&snapshotDryRun, "dry-run", false, "report what would be removed, remove nothing")
}
