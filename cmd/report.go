package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
)

var reportCmd = &cobra.Command{
	Use:   "report [ID]",
	Short: "Read the latest or named check report",
	Long: `Read a stored report: the newest one, or the one ID names.

ID is a check ID, or the run ID of one model within a check. Reports recorded
by the retired "pudl run" and "pudl run set" commands read the same way;
fields those reports carry that current reports do not are shown as recorded
under "legacy fields".

Examples:
    pudl report
    pudl report check_... --json`,
	Args: cobra.MaximumNArgs(1),
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
		if err := printCheckReport(&report, true); err != nil {
			return err
		}
		if err := printSetLegacyFields(set.Report); err != nil {
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
	if err := printLegacyFields(single.Report, report); err != nil {
		return err
	}
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

// printCheckReport writes a check report: under --json the report itself;
// otherwise the outcome of each model and, with members, each model's full
// report first (a live check has already printed those as they concluded).
func printCheckReport(report *acute.RunSetReport, members bool) error {
	if jsonOutput {
		payload, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(outw(), string(payload))
		return err
	}
	if members {
		for _, member := range report.Members {
			if err := printMemberReport(member.Report); err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(outw(), "\ncheck %s: %s\n", report.RunSetID, report.Status)
	if s := report.Summary; s != nil {
		fmt.Fprintf(outw(), "conformity: %s | checks: %s | verification: %s\n", s.Conformity, s.Checks, s.Verification)
	}
	for _, member := range report.Members {
		line := fmt.Sprintf("  %s: %s (%s)", member.Model, member.Result, member.RunID)
		if s := member.Summary; s != nil {
			line += fmt.Sprintf(" | conformity: %s | checks: %s | verification: %s | scope: %s", s.Conformity, s.Checks, s.Verification, s.Scope)
		}
		if member.Error != "" {
			line += ": " + member.Error
		}
		fmt.Fprintln(outw(), strings.TrimSpace(line))
		if member.Summary != nil {
			for _, e := range member.Summary.Evidence {
				fmt.Fprintf(outw(), "    evidence: %s%s | age at check: %s | complete: %t\n", e.SnapshotID, e.ObservationID, e.Age, e.Complete)
			}
		}
	}
	return nil
}

// printMemberReport renders one embedded member report as markdown.
func printMemberReport(payload json.RawMessage) error {
	if len(payload) == 0 {
		return nil
	}
	var report RunReport
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&report); err != nil {
		return fmt.Errorf("decode member report: %w", err)
	}
	text, err := report.render(false)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(outw(), "\n"+text)
	return err
}

func init() {
	rootCmd.AddCommand(reportCmd)
}
