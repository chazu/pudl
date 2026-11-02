package checkreport

import (
	"fmt"
	"time"

	"github.com/chazu/pudl/internal/database"
)

// historyLimit bounds how many of a model's earlier reports classification
// reads looking for comparable evaluations.
const historyLimit = 50

// datedReport is a stored model report with when it was recorded and whether
// its own observation was complete.
type datedReport struct {
	runID     string
	at        time.Time
	report    *ModelReport
	populated bool
}

// RecordChanges classifies each check of report against the model's
// comparable earlier reports and records its findings and changes on the
// report. at is when the checks were evaluated.
//
// Stored reports are the baseline, not earlier snapshots evaluated again:
// they are the findings pudl already recorded and showed, under the rules in
// force at the time; reading them is cheap; and they survive snapshot
// retention. A rule change can therefore show as resolved and new findings.
func RecordChanges(db *database.CatalogDB, report *ModelReport, at time.Time) error {
	if len(report.Checks) == 0 {
		return nil
	}
	records, err := db.ModelRunReports(report.Model, historyLimit+1)
	if err != nil {
		return err
	}
	var earlier []datedReport
	for _, record := range records {
		if record.RunID == report.RunID {
			continue
		}
		// An earlier report that no longer decodes cannot be compared.
		if r, err := DecodeModelReport(record.Report); err == nil {
			earlier = append(earlier, datedReport{runID: record.RunID, at: record.CreatedAt, report: r, populated: populateComplete(db, r)})
		}
	}
	populated := populateComplete(db, report)
	for i := range report.Checks {
		current := NewObservation(report.RunID, observationMode(report), at, report.Checks[i], populated)
		report.Checks[i].Findings, report.Checks[i].Changes = Classify(current, comparableHistory(current, earlier))
	}
	return nil
}

// comparableHistory selects, newest first, the earlier evaluations of
// current's check that are comparable and reached a verdict, stopping at the
// newest complete one: before it, absence was already established.
func comparableHistory(current Observation, earlier []datedReport) []Observation {
	var history []Observation
	for _, d := range earlier {
		check, ok := findCheck(d.report, current.Check.Name)
		if !ok || check.Uncertain() {
			continue
		}
		o := NewObservation(d.runID, observationMode(d.report), d.at, check, d.populated)
		if !Comparable(current, o) {
			continue
		}
		history = append(history, o)
		if o.Complete {
			break
		}
	}
	return history
}

// observationMode is what a report observed: its mode, marked as a replay
// when an inventory check compared recorded records instead of populating.
// A replay is never compared with a live observation.
func observationMode(r *ModelReport) string {
	if r.Drift != nil && r.Populate == nil {
		return r.Mode + " replay"
	}
	return r.Mode
}

func findCheck(r *ModelReport, name string) (CheckResult, bool) {
	for _, c := range r.Checks {
		if c.Name == name {
			return c, true
		}
	}
	return CheckResult{}, false
}

// populateComplete reports whether a model report's own observation was
// complete: as its summary recorded it, else as the catalog holds it.
func populateComplete(db *database.CatalogDB, r *ModelReport) bool {
	if r.Populate == nil || r.Populate.SnapshotID == "" {
		return false
	}
	id := r.Populate.SnapshotID
	if r.Summary != nil {
		for _, e := range r.Summary.Evidence {
			if e.SnapshotID == id {
				return e.Complete
			}
		}
	}
	snapshot, err := db.GetObserveSnapshot(id)
	return err == nil && snapshot != nil && snapshot.Complete
}

// storedModelReports loads the model reports a stored report holds: each
// member of a check report, or the one model report.
func storedModelReports(db *database.CatalogDB, s *Stored) ([]datedReport, error) {
	ids := []string{s.RunID}
	if s.Check != nil {
		ids = ids[:0]
		for _, member := range s.Check.Members {
			ids = append(ids, member.RunID)
		}
	}
	var out []datedReport
	for _, id := range ids {
		record, err := db.GetRunReport(id)
		if err != nil {
			return nil, err
		}
		if record == nil {
			continue
		}
		r, err := DecodeModelReport(record.Report)
		if err != nil {
			return nil, fmt.Errorf("decode stored run report %q: %w", id, err)
		}
		out = append(out, datedReport{runID: id, at: record.CreatedAt, report: r, populated: populateComplete(db, r)})
	}
	return out, nil
}
