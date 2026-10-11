package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
)

var runReportCmd = &cobra.Command{
	Use:   "report [operation-id]",
	Short: "Read the latest or named standalone/set operation report",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := database.OpenCatalogDBReadOnly(effectivePudlDir())
		if err != nil {
			return err
		}
		defer db.Close()
		return readOperationReport(db, args)
	},
}

func readOperationReport(db *database.CatalogDB, args []string) error {
	var single *database.RunReportRecord
	var set *database.RunSetReportRecord
	var err error
	if len(args) == 1 {
		single, err = db.GetRunReport(args[0])
		if err == nil {
			set, err = db.GetRunSetReport(args[0])
		}
		if single != nil && set != nil {
			return fmt.Errorf("operation id %q is ambiguous", args[0])
		}
	} else {
		single, err = db.LatestRunReport()
		if err == nil {
			set, err = db.LatestRunSetReport()
		}
	}
	if err != nil {
		return err
	}
	if single == nil && set == nil {
		if len(args) == 1 {
			return fmt.Errorf("operation report %q not found", args[0])
		}
		return fmt.Errorf("no persisted operation reports")
	}
	if set != nil && (single == nil || !single.CreatedAt.After(set.CreatedAt)) {
		var report acute.RunSetReport
		if err := json.Unmarshal(set.Report, &report); err != nil {
			return fmt.Errorf("decode stored set report %q: %w", set.RunSetID, err)
		}
		evidence, err := operationSetEvidence(db, &report)
		if err != nil {
			return err
		}
		if jsonOutput {
			payload, err := reportWithEvidenceAvailability(set.Report, evidence)
			if err != nil {
				return err
			}
			fmt.Fprintln(outw(), string(payload))
			return nil
		}
		if err := printRunSetReport(&report); err != nil {
			return err
		}
		printEvidenceAvailability(evidence)
		return nil
	}
	refs, err := db.RunReportEvidence(single.RunID)
	if err != nil {
		return err
	}
	evidence := make([]operationEvidence, 0, len(refs))
	for _, ref := range refs {
		evidence = append(evidence, operationEvidence{ReportEvidence: ref})
	}
	if jsonOutput {
		payload, err := reportWithEvidenceAvailability(single.Report, evidence)
		if err != nil {
			return err
		}
		fmt.Fprintln(outw(), string(payload))
		return nil
	}
	var report RunReport
	decoder := json.NewDecoder(bytes.NewReader(single.Report))
	decoder.UseNumber()
	if err := decoder.Decode(&report); err != nil {
		return fmt.Errorf("decode stored run report %q: %w", single.RunID, err)
	}
	text, err := report.render(false)
	if err != nil {
		return err
	}
	fmt.Fprint(outw(), text)
	printEvidenceAvailability(evidence)
	return nil
}

// Availability is a current annotation alongside immutable recorded findings.
// RawMessage preserves unknown report fields and exact historical numbers.
type operationEvidence struct {
	RunID string `json:"run_id,omitempty"`
	database.ReportEvidence
}

func reportWithEvidenceAvailability(payload []byte, evidence []operationEvidence) ([]byte, error) {
	if len(evidence) == 0 {
		return payload, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, fmt.Errorf("decode report for evidence availability: %w", err)
	}
	if fields == nil {
		return nil, fmt.Errorf("report must be a JSON object")
	}
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return nil, err
	}
	fields["evidence_availability"] = encoded
	return json.MarshalIndent(fields, "", "  ")
}

func operationSetEvidence(db *database.CatalogDB, report *acute.RunSetReport) ([]operationEvidence, error) {
	var evidence []operationEvidence
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
			evidence = append(evidence, operationEvidence{RunID: member.RunID, ReportEvidence: ref})
		}
	}
	return evidence, nil
}

func printEvidenceAvailability(evidence []operationEvidence) {
	if len(evidence) == 0 {
		return
	}
	fmt.Fprintln(outw(), "\n## evidence availability")
	for _, ref := range evidence {
		owner := ""
		if ref.RunID != "" {
			owner = " (run " + ref.RunID + ")"
		}
		fmt.Fprintf(outw(), "- %s: %s%s\n", ref.SnapshotID, ref.Status, owner)
	}
}

func isRunSetOperation(id string) (bool, error) {
	db, err := database.OpenCatalogDBReadOnly(effectivePudlDir())
	if err != nil {
		return false, err
	}
	defer db.Close()
	single, err := db.GetRunReport(id)
	if err != nil {
		return false, err
	}
	set, err := db.GetRunSetReport(id)
	if err != nil {
		return false, err
	}
	if single != nil && set != nil {
		return false, fmt.Errorf("operation id %q is ambiguous", id)
	}
	return set != nil, nil
}

func persistRunReport(cat *runCatalog, report *RunReport, live bool) bool {
	if report == nil || report.RunID == "" {
		return false
	}
	if report.ReportVersion == 0 {
		report.ReportVersion = 2
	}
	db, err := cat.optional()
	if err != nil {
		if live {
			fmt.Fprintf(errw(), "warning: could not open catalog to persist run report: %v\n", err)
		}
		return false
	}
	report.Summary = summarizeRun(report, db)
	b, err := json.Marshal(report)
	if err != nil {
		if live {
			fmt.Fprintf(errw(), "warning: could not encode run report: %v\n", err)
		}
		return false
	}
	if err := db.SaveRunReport(report.RunID, report.Model, b); err != nil {
		if live {
			fmt.Fprintf(errw(), "warning: could not persist run report: %v\n", err)
		}
		return false
	}
	return true
}

func init() {
	runCmd.AddCommand(runReportCmd)
}
