package cmd

import (
	"fmt"
	"sort"

	"github.com/chazu/pudl/internal/importer"
)

// displayImportPreview prints a --dry-run result.
func displayImportPreview(result *importer.ImportResult) {
	p := result.Preview
	fmt.Fprintf(outw(), "🔍 Dry run (nothing written): %s\n", result.SourcePath)
	fmt.Fprintf(outw(), "   Format: %s\n", result.DetectedFormat)
	fmt.Fprintf(outw(), "   Records: %d\n", result.RecordCount)
	schemas := make([]string, 0, len(p.Schemas))
	for s := range p.Schemas {
		schemas = append(schemas, s)
	}
	sort.Strings(schemas)
	for _, s := range schemas {
		fmt.Fprintf(outw(), "   Schema %s: %d\n", s, p.Schemas[s])
	}
	if p.AlreadyCataloged > 0 {
		fmt.Fprintf(outw(), "   Already cataloged: %d\n", p.AlreadyCataloged)
	}
	if p.WouldReassign > 0 {
		fmt.Fprintf(outw(), "   Would move to --schema: %d\n", p.WouldReassign)
	}
	if p.ValidationFailures > 0 {
		fmt.Fprintf(outw(), "   ❌ Records failing --schema: %d\n", p.ValidationFailures)
		for _, issue := range p.ValidationIssues {
			fmt.Fprintf(outw(), "      - %s\n", issue)
		}
	}
	if p.Ambiguous > 0 {
		fmt.Fprintf(outw(), "   ⚠️  %d records matched more than one schema family (e.g. %s); route them with --schema or pin a discriminating field in the schema\n", p.Ambiguous, p.AmbiguousExample)
	}
	printImportNotes(result)
}
