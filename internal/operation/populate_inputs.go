package operation

import (
	"encoding/json"
	"fmt"

	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/observe"
	"github.com/chazu/pudl/internal/populate"
)

// ingestPerInput ingests a populate whose observation scope is per input:
// one snapshot per enumerated input, scoped by the input and holding only
// its records, so completeness and what is marked gone are decided per
// input (docs/design/2026-11-16-per-input-scope.md). An input whose run
// failed is recorded as an incomplete snapshot of its scope; the other
// inputs are unaffected. base carries the provenance every input shares.
func (r *modelRun) ingestPerInput(summary *checkreport.Populate, res populate.Result, base populateIngest) error {
	db, err := r.cat.required()
	if err != nil {
		return err
	}
	failed := map[string][]populate.FailedInput{}
	for _, f := range res.Failed {
		failed[f.Item] = append(failed[f.Item], f)
	}
	for i, input := range res.Inputs {
		observation := *r.model.Observation
		observation.Scope = observation.ScopeFor(input)
		records := res.InputRecords[i]
		if records == nil {
			records = []any{}
		}
		results := []observe.ObserveResult{{Target: summary.Target, Current: map[string]any{"records": records}}}
		wrapped, err := json.Marshal(append(results, failedInputResults(summary.Target, failed[input])...))
		if err != nil {
			return fmt.Errorf("marshal observe results of input %s: %w", input, err)
		}
		in := base
		in.observation, in.summary, in.input = &observation, summary, input
		in.snapshotID = inputSnapshotID(r.session.SnapshotID, i)
		count, snapshotID, err := r.ingest(wrapped, in)
		if err != nil {
			return fmt.Errorf("input %s: %w", input, err)
		}
		snapshot, err := db.GetObserveSnapshot(snapshotID)
		if err != nil {
			return err
		}
		summary.Records += count
		summary.Observations = append(summary.Observations, checkreport.InputObservation{
			Input: input, Scope: observation.Scope, SnapshotID: snapshotID, Records: count,
			Complete: snapshot != nil && snapshot.Complete,
		})
	}
	return nil
}

// inputSnapshotID is the snapshot ID of the input at index i: the run's
// snapshot ID and the input's 1-based position, known before the ingest.
func inputSnapshotID(runSnapshotID string, i int) string {
	return fmt.Sprintf("%s_%d", runSnapshotID, i+1)
}
