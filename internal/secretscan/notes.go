package secretscan

import "fmt"

// Notes names each likely secret outside sensitive-field coverage by record,
// path and kind (never the value), then says how to resolve them. total is
// every warning found; warnings may hold only the first of them.
func Notes(warnings []Warning, total int) []string {
	if total == 0 {
		return nil
	}
	notes := []string{fmt.Sprintf("🔑 %d likely secrets outside the schema's sensitive fields:", total)}
	for _, w := range warnings {
		notes = append(notes, fmt.Sprintf("   record %d (%s): %s", w.Record, w.Schema, w.Finding))
	}
	if more := total - len(warnings); more > 0 {
		notes = append(notes, fmt.Sprintf("   … and %d more", more))
	}
	return append(notes,
		"   Fix: add the path to the schema's _pudl.sensitive_fields to redact it before storage,",
		"   or, if it is not a secret, to _pudl.secret_scan_ignore. Route catchall records with --schema.")
}
