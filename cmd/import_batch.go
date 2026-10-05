package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/ui"
)

// runBatchImport imports several files. Per-file progress goes to stderr; the
// result is a summary on stdout, or under --json one array entry per file.
func runBatchImport(cmd *cobra.Command, filePaths []string) error {
	progress := ui.FromCmd(cmd)
	progress.Progressf("🔄 Importing %d files...\n", len(filePaths))

	session, err := newImportSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.ctx = cmd.Context()
	session.limits = importIngestLimits()

	var importErrors []error
	outcomes := make([]importOutcome, 0, len(filePaths))
	successCount := 0
	totalRecords := 0
	totalSize := int64(0)

	origin := workspaceImportOrigin()

	// Import each file
	for i, filePath := range filePaths {
		progress.Progressf("📁 [%d/%d] Importing: %s", i+1, len(filePaths), filepath.Base(filePath))

		result, err := importOneWithEnvelope(session.imp, session.options(filePath, origin))
		outcomes = append(outcomes, newImportOutcome(filePath, result, err))
		if err != nil {
			importErrors = append(importErrors, fmt.Errorf("failed to import %s: %w", filepath.Base(filePath), err))
			progress.Progressf("   ❌ Failed: %v", err)
			continue
		}

		if importExplain && !jsonOutput {
			displayImportExplanation(result)
		}

		successCount++
		totalRecords += result.RecordCount
		totalSize += result.SizeBytes

		progress.Progressf("   ✅ Success: %s (ID: %s, Records: %d)", result.DetectedFormat, result.ID, result.RecordCount)
	}

	if jsonOutput {
		if err := writeImportJSON(outcomes); err != nil {
			return err
		}
	} else {
		displayBatchImportSummary(successCount, len(filePaths), totalRecords, totalSize, importErrors)
	}

	// If there were any errors, return the first one
	if len(importErrors) > 0 {
		return importErrors[0]
	}

	return nil
}

// displayBatchImportSummary shows a summary of batch import results
func displayBatchImportSummary(successCount, totalCount, totalRecords int, totalSize int64, importErrors []error) {
	fmt.Fprintln(outw())
	fmt.Fprintf(outw(), "📊 Batch Import Summary\n")
	fmt.Fprintf(outw(), "   Files processed: %d/%d\n", successCount, totalCount)
	fmt.Fprintf(outw(), "   Total records: %d\n", totalRecords)
	fmt.Fprintf(outw(), "   Total size: %d bytes\n", totalSize)

	if len(importErrors) > 0 {
		fmt.Fprintf(outw(), "   Errors: %d\n", len(importErrors))
		fmt.Fprintln(outw())
		fmt.Fprintln(outw(), "❌ Import Errors:")
		for _, err := range importErrors {
			fmt.Fprintf(outw(), "   - %v\n", err)
		}
	}

	if successCount > 0 {
		fmt.Fprintln(outw())
		fmt.Fprintln(outw(), "✅ Batch import completed!")
		if successCount < totalCount {
			fmt.Fprintf(outw(), "   %d files imported successfully, %d failed\n", successCount, totalCount-successCount)
		} else {
			fmt.Fprintf(outw(), "   All %d files imported successfully\n", successCount)
		}
	}
}
