package cmd

import (
	"fmt"

	"github.com/chazu/pudl/internal/doctor"
)

// printDoctorSecrets lists likely secrets in stored data by entry or fact,
// path and kind (never the value), capped at maxListedChanges.
func printDoctorSecrets(report *doctor.SecretReport) {
	if report == nil {
		return
	}
	scanned := fmt.Sprintf("%d payloads", report.Entries)
	if report.FactsScanned {
		scanned += fmt.Sprintf(", %d facts", report.Facts)
	}
	if report.Total == 0 {
		fmt.Fprintf(outw(), "Secrets: no likely secrets outside schema coverage (%s scanned)\n", scanned)
		return
	}
	fmt.Fprintf(outw(), "⚠️  Secrets: %d likely secrets outside schema coverage (%s scanned)\n", report.Total, scanned)
	for i, f := range report.Findings {
		if i == maxListedChanges {
			break
		}
		if f.Subject == "fact" {
			fmt.Fprintf(outw(), "   fact %s (%s): %s (%s)\n", f.ID, f.Relation, f.Path, f.Kind)
		} else {
			fmt.Fprintf(outw(), "   entry %s [%s]: %s (%s)\n", f.Proquint, f.Schema, f.Path, f.Kind)
		}
	}
	if listed := min(len(report.Findings), maxListedChanges); report.Total > listed {
		fmt.Fprintf(outw(), "   ... %d more (--json lists up to %d)\n", report.Total-listed, doctor.MaxSecretFindings)
	}
	fmt.Fprintln(outw(), "   Fix: declare an entry's path in its schema's _pudl.sensitive_fields (a future scrub, pudl-m10,")
	fmt.Fprintln(outw(), "   will redact the stored plaintext), or add it to _pudl.secret_scan_ignore if it is not a secret;")
	fmt.Fprintln(outw(), "   route catchall records to a schema of their own with import --schema.")
	fmt.Fprintln(outw(), "   Fact args have no schema: rotate the credential and retract the fact. Findings do not fail doctor.")
}
