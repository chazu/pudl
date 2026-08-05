package wiring

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/chazu/pudl/internal/systemmodel"
)

// SnapshotCatalog is the read-only catalog boundary used while elaborating one
// consumer. Selection is pinned before any record is inspected.
type SnapshotCatalog interface {
	LatestSuccessfulObserveSnapshot(model, workspace string) (*Snapshot, error)
	SuccessfulObserveSnapshotForRun(model, workspace, runID string) (*Snapshot, error)
	ObserveSnapshotByIDForRun(snapshotID, model, workspace, runID string) (*Snapshot, error)
	SnapshotRecordEntries(snapshotID string) ([]CatalogEntry, error)
}

// Snapshot and CatalogEntry are aliases that keep the resolver interface small
// while allowing *database.CatalogDB to satisfy it directly.
type Snapshot = databaseSnapshot
type CatalogEntry = databaseEntry

// ResolveRequest supplies run policy rather than binding-level configuration.
type ResolveRequest struct {
	Workspace               string
	MaxObservationAge       *time.Duration
	EvaluationTime          time.Time
	CurrentProducerRuns     map[string]ProducerRun
	PinnedProducerSnapshots map[string]PinnedProducerSnapshot
}

type ProducerRun struct {
	Model string
	RunID string
}

type PinnedProducerSnapshot struct {
	Model      string
	RunID      string
	SnapshotID string
}

// Elaboration is the concrete runtime model plus reproducible plain-binding
// evidence. Inputs retain their exact JSON scalar representation.
type Elaboration struct {
	Model    *systemmodel.SystemModel
	Inputs   map[string]json.RawMessage
	Evidence []BindingEvidence
}

type BindingEvidence struct {
	Input          string          `json:"input"`
	ProducerModel  string          `json:"producer_model"`
	ProducerRunID  string          `json:"producer_run_id"`
	SnapshotID     string          `json:"snapshot_id"`
	Workspace      string          `json:"workspace"`
	Schema         string          `json:"schema"`
	Identity       map[string]any  `json:"identity"`
	Path           string          `json:"path"`
	Selection      string          `json:"selection"`
	ObservedAt     time.Time       `json:"observed_at"`
	EvaluatedAt    time.Time       `json:"evaluated_at"`
	Age            time.Duration   `json:"age"`
	MaxAge         *time.Duration  `json:"max_age,omitempty"`
	Value          json.RawMessage `json:"value"`
	ValueType      string          `json:"value_type"`
	ScalarSHA256   string          `json:"scalar_sha256"`
	ResolutionCode string          `json:"resolution_status"`
}

// BindingIssue is durable, value-free evidence for a binding that could not be
// resolved. It carries the authored selector and a stable code, but never a
// guessed value or fallback snapshot.
type BindingIssue struct {
	Input         string         `json:"input"`
	ProducerModel string         `json:"producer_model,omitempty"`
	Schema        string         `json:"schema,omitempty"`
	Identity      map[string]any `json:"identity,omitempty"`
	Path          string         `json:"path,omitempty"`
	Code          string         `json:"code"`
	Message       string         `json:"message"`
}

// ResolutionError is safe for dry-run/JSON diagnostics and identifies the
// exact failed input without substituting a fallback value.
type ResolutionError struct {
	Input string `json:"input"`
	Code  string `json:"code"`
	Cause error  `json:"-"`
}

func (e *ResolutionError) Error() string {
	return fmt.Sprintf("resolve binding %q: %s: %v", e.Input, e.Code, e.Cause)
}

func bindingError(input, code string, err error) error {
	return &ResolutionError{Input: input, Code: code, Cause: err}
}
