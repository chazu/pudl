package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/inference"
)

var (
	identityMigrateDryRun    bool
	identityMigrateRecompute bool
)

var identityMigrateCmd = &cobra.Command{
	Use:   "identity",
	Short: "Backfill or recompute resource identity tracking for existing entries",
	Long: `Compute and populate resource_id and identity_json for catalog entries.

By default this backfills entries with a NULL resource_id (e.g. data imported
before identity tracking was added): it loads each entry's data file, extracts
identity fields from the schema metadata, and computes resource_id.

With --recompute, every entry's resource_id and identity_json are recomputed and
versions are re-sequenced. Use this once after upgrading to root-of-family
identity namespacing, which changes every resource_id: identity is now keyed on
the inheritance-family root rather than the assigned leaf schema, so resources
stay stable under reinference and policy refinement. Recompute is idempotent.

Examples:
    pudl migrate identity                  # Backfill entries with NULL resource_id
    pudl migrate identity --recompute      # Recompute all resource_ids + versions
    pudl migrate identity --recompute --dry-run  # Preview the recompute`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runIdentityMigrate()
	},
}

func init() {
	migrateCmd.AddCommand(identityMigrateCmd)
	identityMigrateCmd.Flags().BoolVar(&identityMigrateDryRun, "dry-run", false, "Preview changes without applying")
	identityMigrateCmd.Flags().BoolVar(&identityMigrateRecompute, "recompute", false, "Recompute all resource_ids (family-root namespacing) and re-sequence versions")
}

func runIdentityMigrate() error {
	cfg, err := config.Load()
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to load configuration", err)
	}

	configDir := filepath.Dir(cfg.DataPath)

	catalogDB, err := database.NewCatalogDB(configDir)
	if err != nil {
		return errors.NewSystemError("Failed to initialize catalog database", err)
	}
	defer catalogDB.Close()

	inferrer, err := inference.NewSchemaInferrer(cfg.SchemaPath)
	if err != nil {
		return errors.NewSystemError("Failed to initialize schema inferrer", err)
	}

	graph := inferrer.GetInheritanceGraph()

	// Query all entries.
	result, err := catalogDB.QueryEntries(database.FilterOptions{}, database.QueryOptions{Limit: 0})
	if err != nil {
		return errors.NewSystemError("Failed to query catalog", err)
	}

	// Select entries to process: all entries in recompute mode, otherwise only
	// those still missing a resource_id.
	var targets []database.CatalogEntry
	for _, entry := range result.Entries {
		if identityMigrateRecompute || entry.ResourceID == nil {
			targets = append(targets, entry)
		}
	}

	if len(targets) == 0 {
		fmt.Println("All entries already have resource_id. Nothing to migrate.")
		return nil
	}

	if identityMigrateRecompute {
		fmt.Printf("Recomputing identity for %d entries\n\n", len(targets))
	} else {
		fmt.Printf("Found %d entries without resource_id\n\n", len(targets))
	}

	// Recompute resource_id + identity_json for each target, remembering the
	// previous resource_id for reporting.
	oldResourceIDs := make([]*string, len(targets))
	var failCount int
	processed := make([]bool, len(targets))
	for i := range targets {
		oldResourceIDs[i] = targets[i].ResourceID
		if err := migrateEntryIdentity(&targets[i], inferrer, graph); err != nil {
			fmt.Printf("  SKIP %s: %v\n", targets[i].ID[:16], err)
			failCount++
			continue
		}
		processed[i] = true
	}

	// In recompute mode, re-sequence versions: family-root namespacing can merge
	// previously-distinct resource_ids into one resource that needs a single
	// coherent monotonic history. Only processed entries participate.
	if identityMigrateRecompute {
		var versioned []database.CatalogEntry
		for i := range targets {
			if processed[i] {
				versioned = append(versioned, targets[i])
			}
		}
		assignVersions(versioned)
		// Map back by ID (assignVersions worked on a copy slice).
		versionByID := make(map[string]*int, len(versioned))
		for _, e := range versioned {
			versionByID[e.ID] = e.Version
		}
		for i := range targets {
			if processed[i] {
				targets[i].Version = versionByID[targets[i].ID]
			}
		}
	}

	// Apply (or preview) changes.
	var successCount int
	for i := range targets {
		if !processed[i] {
			continue
		}
		entry := targets[i]
		newRID := *entry.ResourceID

		if identityMigrateDryRun {
			fmt.Printf("  WOULD UPDATE %s: %s -> %s%s\n",
				entry.ID[:16], shortRID(oldResourceIDs[i]), newRID[:16], versionSuffix(entry.Version))
			successCount++
			continue
		}

		if err := catalogDB.UpdateEntry(entry); err != nil {
			fmt.Printf("  FAIL %s: %v\n", entry.ID[:16], err)
			failCount++
			continue
		}

		fmt.Printf("  OK %s: %s -> %s%s\n",
			entry.ID[:16], shortRID(oldResourceIDs[i]), newRID[:16], versionSuffix(entry.Version))
		successCount++
	}

	fmt.Printf("\nMigration complete: %d updated, %d failed\n", successCount, failCount)
	if identityMigrateDryRun {
		fmt.Println("(dry run — no changes applied)")
	}

	return nil
}

// migrateEntryIdentity recomputes entry.ResourceID and entry.IdentityJSON,
// namespacing identity by the assigned schema's inheritance-family root. Unlike
// the reinfer path's recomputeEntryIdentity, it loads the entry's data file
// itself (only when the schema has identity fields) and reports read failures.
func migrateEntryIdentity(entry *database.CatalogEntry, inferrer *inference.SchemaInferrer, graph *inference.InheritanceGraph) error {
	var identityFields []string
	if meta, found := inferrer.GetSchemaMetadata(entry.Schema); found {
		identityFields = meta.IdentityFields
	}

	contentHash := entry.ID
	if entry.ContentHash != nil {
		contentHash = *entry.ContentHash
	}

	// Only entries with identity fields need their data file; catchall entries
	// are identified by content hash.
	var identityValues map[string]interface{}
	if len(identityFields) > 0 {
		dataBytes, err := os.ReadFile(entry.StoredPath)
		if err != nil {
			return fmt.Errorf("cannot read data file: %w", err)
		}
		var data interface{}
		if err := json.Unmarshal(dataBytes, &data); err == nil {
			identityValues, _ = identity.ExtractFieldValues(data, identityFields)
		}
	}

	namespace := graph.IdentityRoot(entry.Schema)
	resourceID := identity.ComputeResourceID(namespace, identityValues, contentHash)
	entry.ResourceID = &resourceID

	if len(identityValues) > 0 {
		if canonical, err := identity.CanonicalIdentityJSON(identityValues); err == nil {
			entry.IdentityJSON = &canonical
		}
	} else {
		entry.IdentityJSON = nil
	}
	return nil
}

// assignVersions groups entries by resource_id and assigns a monotonic version
// (1..N) within each group, ordered by import time (tie-broken by created time,
// then id) so the sequence is deterministic and idempotent.
func assignVersions(entries []database.CatalogEntry) {
	groups := make(map[string][]int)
	for i := range entries {
		if entries[i].ResourceID == nil {
			continue
		}
		rid := *entries[i].ResourceID
		groups[rid] = append(groups[rid], i)
	}

	for _, idxs := range groups {
		sort.SliceStable(idxs, func(a, b int) bool {
			ea, eb := entries[idxs[a]], entries[idxs[b]]
			if !ea.ImportTimestamp.Equal(eb.ImportTimestamp) {
				return ea.ImportTimestamp.Before(eb.ImportTimestamp)
			}
			if !ea.CreatedAt.Equal(eb.CreatedAt) {
				return ea.CreatedAt.Before(eb.CreatedAt)
			}
			return ea.ID < eb.ID
		})
		for v, idx := range idxs {
			version := v + 1
			entries[idx].Version = &version
		}
	}
}

// shortRID renders the first 16 chars of a resource_id pointer, or "(none)".
func shortRID(rid *string) string {
	if rid == nil {
		return "(none)"
	}
	if len(*rid) <= 16 {
		return *rid
	}
	return (*rid)[:16]
}

// versionSuffix renders " v<N>" for a version pointer, or "".
func versionSuffix(v *int) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf(" v%d", *v)
}
