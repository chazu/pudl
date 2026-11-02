package checkreport

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
)

// Evidence is the current availability of a snapshot a stored report rests
// on: an annotation alongside the immutable recorded findings. RunID names the
// member that recorded it in a check report.
type Evidence struct {
	RunID string `json:"run_id,omitempty"`
	database.ReportEvidence
}

// Stored is a report read back from the catalog: a check report (Check is
// set) or one model's report, as recorded, with the current availability of
// the evidence it rests on.
type Stored struct {
	// Payload is the report exactly as recorded; unknown fields and exact
	// numbers survive.
	Payload  []byte
	Check    *acute.RunSetReport
	RunID    string
	Evidence []Evidence
}

// Find reads the report id names: a check ID or a member's run ID.
func Find(db *database.CatalogDB, id string) (*Stored, error) {
	single, err := db.GetRunReport(id)
	var set *database.RunSetReportRecord
	if err == nil {
		set, err = db.GetRunSetReport(id)
	}
	if single != nil && set != nil {
		return nil, fmt.Errorf("operation id %q is ambiguous", id)
	}
	if err != nil {
		return nil, err
	}
	if single == nil && set == nil {
		return nil, fmt.Errorf("operation report %q not found", id)
	}
	return newer(db, single, set)
}

// Latest reads the newest report of either kind.
func Latest(db *database.CatalogDB) (*Stored, error) {
	single, err := db.LatestRunReport()
	var set *database.RunSetReportRecord
	if err == nil {
		set, err = db.LatestRunSetReport()
	}
	if err != nil {
		return nil, err
	}
	if single == nil && set == nil {
		return nil, fmt.Errorf("no persisted operation reports")
	}
	return newer(db, single, set)
}

// newer loads whichever of a model report and a check report is newer; a
// check report wins a tie.
func newer(db *database.CatalogDB, single *database.RunReportRecord, set *database.RunSetReportRecord) (*Stored, error) {
	if set != nil && (single == nil || !single.CreatedAt.After(set.CreatedAt)) {
		var report acute.RunSetReport
		if err := json.Unmarshal(set.Report, &report); err != nil {
			return nil, fmt.Errorf("decode stored set report %q: %w", set.RunSetID, err)
		}
		evidence, err := checkEvidence(db, &report)
		if err != nil {
			return nil, err
		}
		return &Stored{Payload: set.Report, Check: &report, Evidence: evidence}, nil
	}
	refs, err := db.RunReportEvidence(single.RunID)
	if err != nil {
		return nil, err
	}
	evidence := make([]Evidence, 0, len(refs))
	for _, ref := range refs {
		evidence = append(evidence, Evidence{ReportEvidence: ref})
	}
	return &Stored{Payload: single.Report, RunID: single.RunID, Evidence: evidence}, nil
}

// checkEvidence gathers the evidence of every member of a check report.
func checkEvidence(db *database.CatalogDB, report *acute.RunSetReport) ([]Evidence, error) {
	var evidence []Evidence
	seen := map[string]bool{}
	for _, member := range report.Members {
		if member.RunID == "" || seen[member.RunID] {
			continue
		}
		seen[member.RunID] = true
		refs, err := db.RunReportEvidence(member.RunID)
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			evidence = append(evidence, Evidence{RunID: member.RunID, ReportEvidence: ref})
		}
	}
	return evidence, nil
}

// JSON is the stored report as recorded, with its evidence availability added
// as `evidence_availability` when it has any.
func (s *Stored) JSON() ([]byte, error) {
	if len(s.Evidence) == 0 {
		return s.Payload, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(s.Payload, &fields); err != nil {
		return nil, fmt.Errorf("decode report for evidence availability: %w", err)
	}
	if fields == nil {
		return nil, fmt.Errorf("report must be a JSON object")
	}
	encoded, err := json.Marshal(s.Evidence)
	if err != nil {
		return nil, err
	}
	fields["evidence_availability"] = encoded
	return json.MarshalIndent(fields, "", "  ")
}

// WriteText renders the stored report for humans: the report, any fields a
// retired workflow recorded that current reports do not know, and the
// evidence availability.
func (s *Stored) WriteText(w io.Writer) error {
	if s.Check != nil {
		if err := WriteCheck(w, s.Check, true); err != nil {
			return err
		}
		if err := writeCheckLegacyFields(w, s.Payload); err != nil {
			return err
		}
		writeEvidence(w, s.Evidence)
		return nil
	}
	report, err := DecodeModelReport(s.Payload)
	if err != nil {
		return fmt.Errorf("decode stored run report %q: %w", s.RunID, err)
	}
	text, err := report.Render(false)
	if err != nil {
		return err
	}
	fmt.Fprint(w, text)
	if err := writeModelLegacyFields(w, s.Payload, *report); err != nil {
		return err
	}
	writeEvidence(w, s.Evidence)
	return nil
}

func writeEvidence(w io.Writer, evidence []Evidence) {
	if len(evidence) == 0 {
		return
	}
	fmt.Fprintln(w, "\n## evidence availability")
	for _, ref := range evidence {
		owner := ""
		if ref.RunID != "" {
			owner = " (run " + ref.RunID + ")"
		}
		fmt.Fprintf(w, "- %s: %s%s\n", ref.SnapshotID, ref.Status, owner)
	}
}
