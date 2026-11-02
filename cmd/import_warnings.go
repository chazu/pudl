package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/secretscan"
)

// importNotes are the lines an import summary adds after its counts: anything
// that happened to the records which the user would otherwise not see.
func importNotes(result *importer.ImportResult) []string {
	var notes []string
	if result.IdentityUnresolved > 0 {
		notes = append(notes, fmt.Sprintf("⚠️  identity unresolved for %d of %d records (%s); they are identified by content hash and will not form version chains",
			result.IdentityUnresolved, result.RecordCount, result.IdentityError))
	}
	if result.Reassigned > 0 {
		notes = append(notes, fmt.Sprintf("↪️  %d already-cataloged records moved to the --schema they now validate against", result.Reassigned))
	}
	if len(result.Facts) > 0 {
		notes = append(notes, "📐 facts projected: "+formatFactCounts(result.Facts))
	}
	for _, w := range result.FactWarnings {
		notes = append(notes, "⚠️  "+w)
	}
	if result.Redacted > 0 {
		notes = append(notes, fmt.Sprintf("🔒 %d sensitive values redacted before storage", result.Redacted))
	}
	return append(notes, secretNotes(result.SecretWarnings, result.SecretWarningsTotal)...)
}

// secretNotes names each likely secret outside sensitive-field coverage by
// record, path and kind (never the value), then says how to resolve them.
func secretNotes(warnings []secretscan.Warning, total int) []string {
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

// printImportNotes writes importNotes to the result stream.
func printImportNotes(result *importer.ImportResult) {
	for _, note := range importNotes(result) {
		fmt.Fprintf(outw(), "   %s\n", note)
	}
}

// formatFactCounts renders per-relation fact counts, sorted by relation.
func formatFactCounts(counts map[string]int) string {
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = fmt.Sprintf("%s=%d", name, counts[name])
	}
	return strings.Join(parts, " ")
}
