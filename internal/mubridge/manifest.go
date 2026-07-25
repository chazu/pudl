package mubridge

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/idgen"
)

// ManifestInput represents a mu build manifest.
type ManifestInput struct {
	Timestamp string           `json:"timestamp"`
	Summary   ManifestSummary  `json:"summary"`
	Actions   []ManifestAction `json:"actions"`
}

// ManifestSummary contains aggregate counts from a mu build.
type ManifestSummary struct {
	Total    int `json:"total"`
	Cached   int `json:"cached"`
	Executed int `json:"executed"`
	Failed   int `json:"failed"`
}

// ManifestAction represents a single action from a mu build manifest.
type ManifestAction struct {
	ID       string            `json:"id"`
	Target   string            `json:"target,omitempty"`
	Cached   bool              `json:"cached"`
	ExitCode int               `json:"exit_code"`
	Outputs  map[string]string `json:"outputs,omitempty"`
}

// IngestManifestResult contains summary information about the ingestion.
type IngestManifestResult struct {
	// RunID is the run that *owns* the manifest entry. When Skipped is true that
	// is the run which first recorded it, not the caller's — the manifest is
	// content-addressed and its association is first-writer-wins.
	RunID   string
	Total   int
	Cached  int
	Failed  int
	Skipped bool
	// StatusesRepaired counts per-action statuses a duplicate ingest filled in
	// because the original ingest never managed to write them.
	StatusesRepaired int
}

// IngestManifest processes a mu build manifest and stores results in the catalog.
// Returns the run_id assigned to this manifest and any error. When model is
// non-empty, each per-action entry is tagged with it (`tags.model`), so a later
// drift re-check can promote exactly that model's `converging` resources to `clean`
// (see CatalogDB.PromoteConvergingToCleanByModel) without reconstructing the
// resource→model mapping from desired records.
func IngestManifest(db *database.CatalogDB, reader io.Reader, origin, configDir, model string) (*IngestManifestResult, error) {
	return ingestManifestWithRunID(db, reader, origin, configDir, model, "")
}

// IngestManifestWithRunID attaches an existing PUDL run identity to the
// manifest and all of its action entries. An empty runID preserves the legacy
// deterministic manifest-run behavior.
func IngestManifestWithRunID(db *database.CatalogDB, reader io.Reader, origin, configDir, model, runID string) (*IngestManifestResult, error) {
	return ingestManifestWithRunID(db, reader, origin, configDir, model, runID)
}

func ingestManifestWithRunID(db *database.CatalogDB, reader io.Reader, origin, configDir, model, runIDOverride string) (*IngestManifestResult, error) {
	// Read entire JSON from reader
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest data: %w", err)
	}

	var manifest ManifestInput
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest JSON: %w", err)
	}

	// Generate deterministic content hash from raw data
	hash := sha256.Sum256(data)
	contentHash := fmt.Sprintf("%x", hash)

	// One convergence step, recorded as one transaction: the manifest, its
	// per-action entries and the statuses those actions imply commit together.
	// Failing between the entries and the statuses used to leave an apply on
	// record whose resources still read `unknown` — the hole
	// repairMissingActionStatuses exists to patch up afterwards.
	//
	// The dedup check runs inside the same transaction, which also closes the
	// race it had on its own: two concurrent ingests of the same manifest could
	// both find nothing and both insert. BEGIN IMMEDIATE takes the write lock
	// before the check, so the second one now sees the first's manifest.
	var result *IngestManifestResult
	err = db.WithCatalogTx(func(tx *database.CatalogTx) error {
		var stepErr error
		result, stepErr = ingestManifestIn(tx, data, manifest, contentHash, origin, configDir, model, runIDOverride)
		return stepErr
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ingestManifestIn records one manifest and everything that belongs with it.
// Called inside a CatalogTx, so either all of it lands or none of it does.
func ingestManifestIn(
	db database.CatalogWriter,
	data []byte,
	manifest ManifestInput,
	contentHash, origin, configDir, model, runIDOverride string,
) (*IngestManifestResult, error) {
	// Content-hash dedup: skip if manifest with same content hash already ingested
	existing, err := db.FindByContentHash(contentHash)
	if err != nil {
		return nil, fmt.Errorf("failed to check for duplicate manifest: %w", err)
	}
	if existing != nil {
		// The manifest's content hash covers its own `timestamp`, so two distinct
		// applies never collide: a duplicate is the same apply being recorded a
		// second time. The entry therefore keeps the run that first recorded it —
		// rewriting run_id to runIDOverride here would be the same last-writer-wins
		// association that made observe records unqueryable by their own run. The
		// returned RunID names that owning run, and Skipped says it is not the
		// caller's.
		runID := ""
		if existing.RunID != nil {
			runID = *existing.RunID
		}
		repaired, err := repairMissingActionStatuses(db, manifest)
		if err != nil {
			return nil, err
		}
		return &IngestManifestResult{
			RunID:            runID,
			Total:            manifest.Summary.Total,
			Cached:           manifest.Summary.Cached,
			Failed:           manifest.Summary.Failed,
			Skipped:          true,
			StatusesRepaired: repaired,
		}, nil
	}

	// Generate a bridge-local run_id from timestamp + content hash unless the
	// enclosing PUDL run supplied its audit identity.
	runID := runIDOverride
	if runID == "" {
		runIDSource := manifest.Timestamp + ":" + contentHash
		runIDHash := sha256.Sum256([]byte(runIDSource))
		runID = fmt.Sprintf("%x", runIDHash)
	}

	// Store the raw manifest JSON in the data directory
	manifestStoredPath, err := storeRawData(configDir, data, "manifest.json")
	if err != nil {
		return nil, fmt.Errorf("failed to store manifest data: %w", err)
	}

	// Use the content hash as the catalog entry ID
	manifestID := idgen.ComputeContentID(data)

	// Create the manifest catalog entry
	entryType := "manifest"
	format := "json"
	schema := "pudl/mu.#Manifest"
	now := time.Now()

	manifestEntry := database.CatalogEntry{
		ID:              manifestID,
		StoredPath:      manifestStoredPath,
		MetadataPath:    manifestStoredPath + ".meta",
		ImportTimestamp: now,
		Format:          format,
		Origin:          origin,
		Schema:          schema,
		Confidence:      1.0,
		RecordCount:     1,
		SizeBytes:       int64(len(data)),
		ContentHash:     &contentHash,
		EntryType:       &entryType,
		RunID:           &runID,
	}

	if err := db.AddEntry(manifestEntry); err != nil {
		return nil, fmt.Errorf("failed to add manifest entry: %w", err)
	}

	// Create per-action entries
	actionEntryType := "manifest-action"
	actionSchema := "pudl/mu.#ManifestAction"

	for _, action := range manifest.Actions {
		targetName := actionTargetName(action)
		// Filesystem-safe variant for the stored-action filename ("/" and ":"
		// are not usable as path segments).
		safeName := strings.NewReplacer("/", "_", ":", "_").Replace(targetName)

		// Build tags JSON
		tagMap := map[string]interface{}{
			"exit_code": action.ExitCode,
			"cached":    action.Cached,
		}
		if model != "" {
			tagMap["model"] = model
		}
		tags, err := json.Marshal(tagMap)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal action tags: %w", err)
		}
		tagsStr := string(tags)

		// Store action JSON in raw data directory
		actionData, err := json.Marshal(action)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal action data: %w", err)
		}

		actionStoredPath, err := storeRawData(configDir, actionData, safeName+"_action.json")
		if err != nil {
			return nil, fmt.Errorf("failed to store action data: %w", err)
		}

		// Generate action entry ID from action data content
		actionID := idgen.ComputeContentID(append([]byte(runID+":"), actionData...))
		actionContentHash := idgen.ComputeContentID(actionData)

		actionEntry := database.CatalogEntry{
			ID:              actionID,
			StoredPath:      actionStoredPath,
			MetadataPath:    actionStoredPath + ".meta",
			ImportTimestamp: now,
			Format:          format,
			Origin:          origin,
			Schema:          actionSchema,
			Confidence:      1.0,
			RecordCount:     1,
			SizeBytes:       int64(len(actionData)),
			ContentHash:     &actionContentHash,
			EntryType:       &actionEntryType,
			Target:          &targetName,
			RunID:           &runID,
			Tags:            &tagsStr,
		}

		if err := db.AddEntry(actionEntry); err != nil {
			return nil, fmt.Errorf("failed to add action entry for %s: %w", targetName, err)
		}

		// Status from the action exit code (see actionStatus): "converging" means
		// applied, pending verification — only the drift re-check writes the
		// verified in-sync status "clean" (build-spec §5).
		//
		// A failure here aborts the step rather than warning past it. Warning was
		// what produced an apply on record whose resources still read `unknown`;
		// now the manifest and its statuses land together or not at all, and the
		// caller is told the step failed instead of discovering it later.
		if err := db.UpdateStatus(targetName, actionStatus(action)); err != nil {
			return nil, fmt.Errorf("failed to update status for %s: %w", targetName, err)
		}
	}

	return &IngestManifestResult{
		RunID:  runID,
		Total:  manifest.Summary.Total,
		Cached: manifest.Summary.Cached,
		Failed: manifest.Summary.Failed,
	}, nil
}

// actionTargetName is the catalog `target` key for one manifest action. mu's
// build manifest carries the identifier under `id` (e.g.
// "//models/<m>:drift:apply"); the older `target` field may be empty, so fall
// back to `id`. The leading "//" is stripped.
func actionTargetName(action ManifestAction) string {
	actionRef := action.Target
	if actionRef == "" {
		actionRef = action.ID
	}
	return normalizeTarget(actionRef)
}

// actionStatus is the status one action implies: exit 0 means the apply COMMAND
// ran, not that observed==desired, so it is "converging" (applied, pending
// verification) rather than "clean". Only the drift re-check writes "clean".
func actionStatus(action ManifestAction) string {
	if action.ExitCode != 0 {
		return "failed"
	}
	return "converging"
}

// repairMissingActionStatuses fills in per-action statuses that were never
// recorded, and touches nothing else.
//
// The first ingest treats an UpdateStatus failure as a warning rather than an
// error, so an action's apply can be recorded while its resource is left sitting
// at the default `unknown`. Re-ingesting the same manifest is the natural repair
// path for that, and it is the only thing a re-ingest may safely write: a
// duplicate is the *same apply* recorded twice, so rewriting statuses wholesale
// would knock a resource the drift re-check has since promoted to `clean` back
// to `converging`, undoing a verification with information older than it.
func repairMissingActionStatuses(db database.CatalogWriter, manifest ManifestInput) (int, error) {
	statuses, err := db.GetTargetStatuses()
	if err != nil {
		return 0, fmt.Errorf("failed to read target statuses: %w", err)
	}
	current := make(map[string]string, len(statuses))
	for _, s := range statuses {
		current[s.Target] = s.Status
	}

	repaired := 0
	for _, action := range manifest.Actions {
		targetName := actionTargetName(action)
		// Absent means the action has no catalog row to carry a status; only a row
		// still at the default `unknown` is one the first ingest failed to write.
		if status, ok := current[targetName]; !ok || status != "unknown" {
			continue
		}
		if err := db.UpdateStatus(targetName, actionStatus(action)); err != nil {
			return repaired, fmt.Errorf("failed to repair status for %s: %w", targetName, err)
		}
		repaired++
	}
	return repaired, nil
}

// normalizeTarget converts a mu target name to the catalog `target` key,
// stripping the leading "//" prefix if present.
func normalizeTarget(target string) string {
	return strings.TrimPrefix(target, "//")
}

// storeRawData writes data to the raw data directory using the standard
// YYYY/MM/DD/ date-based path structure.
func storeRawData(configDir string, data []byte, filename string) (string, error) {
	now := time.Now()
	dateDir := filepath.Join(configDir, "data", "raw",
		fmt.Sprintf("%d", now.Year()),
		fmt.Sprintf("%02d", now.Month()),
		fmt.Sprintf("%02d", now.Day()))

	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create raw data directory: %w", err)
	}

	// Use timestamp + filename to avoid collisions
	storedName := fmt.Sprintf("%s_%s", now.Format("20060102_150405"), filename)
	storedPath := filepath.Join(dateDir, storedName)

	if err := os.WriteFile(storedPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write raw data file: %w", err)
	}

	return storedPath, nil
}
