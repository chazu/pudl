package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/errors"
)

var (
	exportID           string
	exportSchema       string
	exportOrigin       string
	exportFormat       string
	exportOutput       string
	exportPretty       bool
	exportAllowPartial bool
)

// exportCmd represents the export command
var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export data lake entries to various formats",
	Long: `Export data from the PUDL catalog to various formats.

You can export entries by ID, schema, or origin. The data is exported in the
specified format (JSON, YAML, CSV, or NDJSON).

Examples:
    pudl export --id babod-fakak                  # Export single entry by proquint ID
    pudl export --schema aws.#EC2Instance         # Export all EC2 instances
    pudl export --origin k8s-pods --format yaml   # Export K8s pods as YAML
    pudl export --id babod-fakak --output out.json  # Export to file`,
	RunE: pudlRunE(runExportCommand),
}

func init() {
	rootCmd.AddCommand(exportCmd)

	exportCmd.Flags().StringVar(&exportID, "id", "", "Export entry by proquint ID")
	exportCmd.Flags().StringVar(&exportSchema, "schema", "", "Export entries matching schema")
	exportCmd.Flags().StringVar(&exportOrigin, "origin", "", "Export entries from origin")
	exportCmd.Flags().StringVar(&exportFormat, "format", "json", "Output format: json, yaml, csv, ndjson")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Output file (default: stdout)")
	exportCmd.Flags().BoolVar(&exportPretty, "pretty", true, "Pretty-print output")

	exportCmd.Flags().BoolVar(&exportAllowPartial, "allow-partial", false, "Publish readable entries on input errors; report incomplete output with a nonzero exit")

	// Register completions
	exportCmd.RegisterFlagCompletionFunc("id", completeEntryIDs)
	exportCmd.RegisterFlagCompletionFunc("schema", completeSchemaNames)
	exportCmd.RegisterFlagCompletionFunc("origin", completeOrigins)
	exportCmd.RegisterFlagCompletionFunc("format", completeFormats)
}

func runExportCommand(cmd *cobra.Command, args []string) error {
	if exportBundle != "" {
		return runExportBundle(cmd)
	}
	// Validate that at least one filter is specified
	if exportID == "" && exportSchema == "" && exportOrigin == "" {
		return errors.NewMissingRequiredError("--id, --schema, or --origin")
	}

	if err := validateExportFormat(exportFormat); err != nil {
		return err
	}

	// Open catalog database
	configDir := effectivePudlDir()
	catalogDB, err := database.NewCatalogDB(configDir)
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to open catalog database", err)
	}
	defer catalogDB.Close()

	// Get entries to export
	var entries []database.CatalogEntry

	if exportID != "" {
		// Export single entry by ID
		entry, err := catalogDB.GetEntryByProquint(exportID)
		if err != nil {
			return err
		}
		entries = []database.CatalogEntry{*entry}
	} else {
		// Query entries by filters
		filters := database.FilterOptions{
			Schema: exportSchema,
			Origin: exportOrigin,
		}
		result, err := catalogDB.QueryEntries(filters, database.QueryOptions{})
		if err != nil {
			return err
		}
		entries = result.Entries
	}

	if len(entries) == 0 {
		return errors.NewInputError("No entries found matching the specified criteria", "", "")
	}

	// Stage one record at a time before publishing the output. This keeps reads
	// fail-closed and permits deterministic CSV headers without retaining records.
	spool, count, omitted, err := prepareExport(cmd.Context(), catalogDB, entries, exportAllowPartial)
	if err != nil {
		return err
	}
	defer os.Remove(spool.Name())
	defer spool.Close()
	if count == 0 {
		return fmt.Errorf("no exportable records found")
	}
	write := func(w io.Writer) error {
		return writeExportSpool(w, spool, count, strings.ToLower(exportFormat), exportPretty)
	}
	if exportOutput == "" {
		err = write(outw())
	} else {
		err = publishExport(exportOutput, write)
	}
	if err != nil {
		return err
	}
	if omitted > 0 {
		return fmt.Errorf("incomplete export: omitted %d unreadable entries (partial output published)", omitted)
	}
	return nil
}
