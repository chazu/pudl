package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

// runBatchImport handles importing multiple files and provides summary output
func runBatchImport(cmd *cobra.Command, filePaths []string) error {
	fmt.Printf("🔄 Importing %d files...\n\n", len(filePaths))

	session, err := newImportSession()
	if err != nil {
		return err
	}
	defer session.Close()

	var importErrors []error
	successCount := 0
	totalRecords := 0
	totalSize := int64(0)

	origin := workspaceImportOrigin()

	// Import each file
	for i, filePath := range filePaths {
		fmt.Printf("📁 [%d/%d] Importing: %s\n", i+1, len(filePaths), filepath.Base(filePath))

		result, err := importOneWithEnvelope(session.imp, session.options(filePath, origin))
		if err != nil {
			importErrors = append(importErrors, fmt.Errorf("failed to import %s: %w", filepath.Base(filePath), err))
			fmt.Printf("   ❌ Failed: %v\n", err)
			continue
		}

		successCount++
		totalRecords += result.RecordCount
		totalSize += result.SizeBytes

		fmt.Printf("   ✅ Success: %s (ID: %s, Records: %d)\n", result.DetectedFormat, result.ID, result.RecordCount)
	}

	// Display summary
	displayBatchImportSummary(successCount, len(filePaths), totalRecords, totalSize, importErrors)

	// If there were any errors, return the first one
	if len(importErrors) > 0 {
		return importErrors[0]
	}

	return nil
}

// displayBatchImportSummary shows a summary of batch import results
func displayBatchImportSummary(successCount, totalCount, totalRecords int, totalSize int64, importErrors []error) {
	fmt.Println()
	fmt.Printf("📊 Batch Import Summary\n")
	fmt.Printf("   Files processed: %d/%d\n", successCount, totalCount)
	fmt.Printf("   Total records: %d\n", totalRecords)
	fmt.Printf("   Total size: %d bytes\n", totalSize)

	if len(importErrors) > 0 {
		fmt.Printf("   Errors: %d\n", len(importErrors))
		fmt.Println()
		fmt.Println("❌ Import Errors:")
		for _, err := range importErrors {
			fmt.Printf("   - %v\n", err)
		}
	}

	if successCount > 0 {
		fmt.Println()
		fmt.Println("✅ Batch import completed!")
		if successCount < totalCount {
			fmt.Printf("   %d files imported successfully, %d failed\n", successCount, totalCount-successCount)
		} else {
			fmt.Printf("   All %d files imported successfully\n", successCount)
		}
	}
}
