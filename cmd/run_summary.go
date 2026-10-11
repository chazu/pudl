package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
)

func summarizeRun(r *RunReport, db *database.CatalogDB) *acute.VerdictSummary {
	s := &acute.VerdictSummary{Execution: r.CompletionStatus, Conformity: "not-checked", Checks: "not-checked", Verification: "not-verified", Scope: "model"}
	if len(r.ResourceScope) > 0 {
		s.Scope = "selected-resources"
	}
	if r.PendingApproval {
		s.Execution = "pending-approval"
	}
	if r.Drift != nil {
		s.Conformity = "clean"
		if !r.Drift.Clean {
			s.Conformity = "drifted"
		}
		if r.Drift.Uncertain {
			s.Conformity = "unknown"
		}
		s.DriftCount = len(r.Drift.Drifted)
		s.Verification = "recorded"
		if r.Drift.Verified {
			s.Verification = "live"
		}
	}
	if r.Converge != nil {
		s.Conformity = "unknown"
		if r.Converge.Outcome == string(outcomeClean) {
			s.Conformity = "clean"
			s.Verification = "live"
		}
		if r.Converge.NeedsVerification {
			s.Verification = "needs-verification"
		}
	}
	seen := map[string]bool{}
	add := func(e acute.ObservationEvidence) {
		key := e.SnapshotID
		if key == "" {
			key = e.ObservationID
		}
		if !seen[key] {
			s.Evidence = append(s.Evidence, e)
			seen[key] = true
		}
	}
	if len(r.Checks) > 0 {
		s.Checks = "pass"
	}
	allLiveChecks := len(r.Checks) > 0
	for _, c := range r.Checks {
		if len(c.Evidence) == 0 {
			allLiveChecks = false
		}
		if c.Outcome == "unknown" || c.Outcome == "error" {
			s.UnknownChecks++
		} else if !c.Passed {
			s.FailedChecks++
		}
		if r.Drift == nil && r.Converge == nil {
			s.Scope = c.Scope
			s.Verification = "recorded"
		}
		for _, e := range c.Evidence {
			if r.Populate == nil || e.SnapshotID != r.Populate.SnapshotID {
				allLiveChecks = false
			}
			add(e)
		}
	}
	if allLiveChecks && r.Drift == nil && r.Converge == nil {
		s.Verification = "live"
	}
	if s.FailedChecks > 0 {
		s.Checks = "fail"
	}
	if s.UnknownChecks > 0 {
		s.Checks = "unknown"
		if s.Verification != "needs-verification" {
			s.Verification = "not-verified"
		}
	}
	var snapshotIDs []string
	if r.Populate != nil {
		snapshotIDs = append(snapshotIDs, r.Populate.SnapshotID)
	}
	if r.Drift != nil {
		snapshotIDs = append(snapshotIDs, r.Drift.SnapshotID)
	}
	for _, binding := range r.Bindings {
		snapshotIDs = append(snapshotIDs, binding.SnapshotID)
	}
	for _, id := range snapshotIDs {
		if db != nil && id != "" {
			if snapshot, err := db.GetObserveSnapshot(id); err == nil && snapshot != nil {
				add(acute.ObservationEvidence{SnapshotID: snapshot.SnapshotID, Scope: snapshot.Scope, Source: snapshot.Source, ObservedAt: snapshot.CreatedAt, Age: time.Since(snapshot.CreatedAt).String(), Complete: snapshot.Complete})
			}
		}
	}
	observation := r.Drift
	if r.Converge != nil && r.Converge.Observation != nil {
		observation = r.Converge.Observation
	}
	if observation != nil && observation.ObservationID != "" && observation.ObservedAt != nil {
		add(acute.ObservationEvidence{ObservationID: observation.ObservationID, Source: "differential-observe", Scope: s.Scope, ObservedAt: *observation.ObservedAt, Age: time.Since(*observation.ObservedAt).String(), Complete: true})
	}
	if r.RunID != "" {
		s.NextActions = append(s.NextActions, "pudl run report "+r.RunID+" --json")
	}
	for _, e := range s.Evidence {
		if e.SnapshotID != "" {
			s.NextActions = append(s.NextActions, "pudl snapshot show "+e.SnapshotID+" --json")
		} else if e.ObservationID != "" {
			s.NextActions = append(s.NextActions, "pudl show "+e.ObservationID+" --json")
		}
	}
	if s.UnknownChecks > 0 {
		s.NextActions = append(s.NextActions, "pudl doctor --json")
	}
	return s
}

func enrichSetSummary(db *database.CatalogDB, report *acute.RunSetReport) error {
	s := &acute.VerdictSummary{Execution: report.Status, Conformity: "not-checked", Checks: "not-checked", Verification: "not-verified", Scope: "exact-set"}
	conformity, checks, verification := map[string]bool{}, map[string]bool{}, map[string]bool{}
	seen := map[string]bool{}
	for i := range report.Members {
		member := &report.Members[i]
		stored, err := db.GetRunReport(member.RunID)
		if err != nil {
			return err
		}
		if stored != nil {
			var r RunReport
			decoder := json.NewDecoder(bytes.NewReader(stored.Report))
			decoder.UseNumber()
			if err := decoder.Decode(&r); err != nil {
				return fmt.Errorf("read member %s summary: %w", member.Model, err)
			}
			member.Summary = r.Summary
			if member.Summary == nil {
				member.Summary = summarizeRun(&r, db)
			}
		}
		if member.Summary == nil {
			member.Summary = &acute.VerdictSummary{Conformity: "unknown", Checks: "unknown", Verification: "not-verified", Scope: "model"}
		}
		m := member.Summary
		m.Execution = member.Result
		conformity[m.Conformity] = true
		checks[m.Checks] = true
		verification[m.Verification] = true
		s.DriftCount += m.DriftCount
		s.FailedChecks += m.FailedChecks
		s.UnknownChecks += m.UnknownChecks
		for _, e := range m.Evidence {
			key := e.SnapshotID
			if key == "" {
				key = e.ObservationID
			}
			if !seen[key] {
				s.Evidence = append(s.Evidence, e)
				seen[key] = true
			}
		}
	}
	if conformity["drifted"] {
		s.Conformity = "drifted"
	} else if conformity["unknown"] || (conformity["clean"] && conformity["not-checked"]) {
		s.Conformity = "unknown"
	} else if conformity["clean"] {
		s.Conformity = "clean"
	}
	if checks["unknown"] {
		s.Checks = "unknown"
	} else if checks["fail"] {
		s.Checks = "fail"
	} else if checks["pass"] {
		s.Checks = "pass"
	}
	if verification["needs-verification"] {
		s.Verification = "needs-verification"
	} else if verification["not-verified"] {
		s.Verification = "not-verified"
	} else if verification["recorded"] {
		s.Verification = "recorded"
	} else if verification["live"] {
		s.Verification = "live"
	}
	s.NextActions = []string{"pudl run report " + report.RunSetID + " --json"}
	report.Summary = s
	return nil
}
