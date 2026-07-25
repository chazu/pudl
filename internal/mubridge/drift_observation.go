package mubridge

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/identity"
)

// DriftObservationEntryType is the catalog entry_type for a persisted drift
// observation.
//
// Deliberately *not* "observe": inventory drift loads observed records with
// entry_type='observe' AND collection_type='item', and a drift observation is a
// verdict artifact rather than an observed record. Filing it under "observe"
// would make it visible to `loadObservedRecords` and let a drift report satisfy
// a desired resource.
const DriftObservationEntryType = "drift-observation"

// DriftObservationSchema is the schema assigned to persisted drift observations.
const DriftObservationSchema = "pudl/mu.#DriftObservation"

// DriftObservation is what a differential observation concluded, stored so that
// a `clean` verdict has evidence behind it.
//
// Differential observation (the k8s-style path) previously wrote nothing at all:
// `mu observe` output was parsed into a verdict, the verdict decided whether
// resources were promoted to `clean`, and the observation itself was discarded.
// The claim was therefore unfalsifiable after the fact — nothing in the catalog
// recorded what had been seen, or when.
type DriftObservation struct {
	ObservationID string           `json:"observation_id"`
	Timestamp     string           `json:"timestamp"`
	Origin        string           `json:"origin"`
	Target        string           `json:"target"`
	RunID         string           `json:"run_id,omitempty"`
	Clean         bool             `json:"clean"`
	DriftedCount  int              `json:"drifted_count"`
	Drifted       []map[string]any `json:"drifted,omitempty"`
	// Raw is mu's observe output verbatim, so the verdict can be re-derived.
	Raw json.RawMessage `json:"raw,omitempty"`
}

// RecordDriftObservation persists one differential drift observation and returns
// its catalog entry ID. The ID belongs on the drift result so the verdict and its
// evidence can be joined later.
//
// Best-effort is the caller's decision, not this function's: it reports every
// failure so a caller that must not claim an unrecorded observation can act on it.
func RecordDriftObservation(
	db *database.CatalogDB,
	observation DriftObservation,
	dataDir string,
) (string, error) {
	if db == nil {
		return "", fmt.Errorf("record drift observation: nil catalog")
	}
	if observation.Target == "" {
		return "", fmt.Errorf("record drift observation: empty target")
	}

	now := time.Now()
	if observation.ObservationID == "" {
		observation.ObservationID = fmt.Sprintf("drift_%s", now.Format("20060102_150405.000000000"))
	}
	if observation.Timestamp == "" {
		observation.Timestamp = now.Format(time.RFC3339)
	}
	if observation.Origin == "" {
		observation.Origin = "pudl-run"
	}

	payload, err := json.Marshal(observation)
	if err != nil {
		return "", fmt.Errorf("marshal drift observation: %w", err)
	}
	hash := sha256.Sum256(payload)
	contentHash := fmt.Sprintf("%x", hash)

	rawDir := filepath.Join(dataDir, "raw", now.Format("2006"), now.Format("01"), now.Format("02"))
	if err := os.MkdirAll(rawDir, 0755); err != nil {
		return "", fmt.Errorf("create raw directory: %w", err)
	}
	storedPath := filepath.Join(rawDir, observation.ObservationID+"_drift.json")
	if err := os.WriteFile(storedPath, payload, 0644); err != nil {
		return "", fmt.Errorf("write drift observation: %w", err)
	}

	// Two observations with identical content are the same observation; reuse the
	// existing entry rather than minting a duplicate.
	if existing, err := db.GetEntry(contentHash); err == nil && existing != nil {
		return contentHash, nil
	}

	resourceID := identity.ComputeResourceID(
		DriftObservationSchema,
		map[string]any{"observation_id": observation.ObservationID},
		contentHash,
	)
	entryType := DriftObservationEntryType
	target := observation.Target
	var runIDPtr *string
	if observation.RunID != "" {
		runID := observation.RunID
		runIDPtr = &runID
	}

	entry := database.CatalogEntry{
		ID:              contentHash,
		StoredPath:      storedPath,
		ImportTimestamp: now,
		Format:          "json",
		Origin:          observation.Origin,
		Schema:          DriftObservationSchema,
		Confidence:      1.0,
		RecordCount:     observation.DriftedCount,
		SizeBytes:       int64(len(payload)),
		EntryType:       &entryType,
		Target:          &target,
		ResourceID:      &resourceID,
		ContentHash:     &contentHash,
		RunID:           runIDPtr,
	}
	if err := db.AddEntry(entry); err != nil {
		return "", fmt.Errorf("add drift observation entry: %w", err)
	}
	return contentHash, nil
}
