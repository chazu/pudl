package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/errors"
	"pudl/internal/idgen"
)

var (
	exportID     string
	exportSchema string
	exportOrigin string
	exportFormat string
	exportOutput string
	exportPretty bool
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
	Run: func(cmd *cobra.Command, args []string) {
		errorHandler := errors.NewCLIErrorHandler(true)

		if err := runExportCommand(cmd, args); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(exportCmd)

	exportCmd.Flags().StringVar(&exportID, "id", "", "Export entry by proquint ID")
	exportCmd.Flags().StringVar(&exportSchema, "schema", "", "Export entries matching schema")
	exportCmd.Flags().StringVar(&exportOrigin, "origin", "", "Export entries from origin")
	exportCmd.Flags().StringVar(&exportFormat, "format", "json", "Output format: json, yaml, csv, ndjson")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Output file (default: stdout)")
	exportCmd.Flags().BoolVar(&exportPretty, "pretty", true, "Pretty-print output")

	// Register completions
	exportCmd.RegisterFlagCompletionFunc("id", completeEntryIDs)
	exportCmd.RegisterFlagCompletionFunc("schema", completeSchemaNames)
	exportCmd.RegisterFlagCompletionFunc("origin", completeOrigins)
	exportCmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "yaml", "csv", "ndjson"}, cobra.ShellCompDirectiveNoFileComp
	})
}

func runExportCommand(cmd *cobra.Command, args []string) error {
	// Validate that at least one filter is specified
	if exportID == "" && exportSchema == "" && exportOrigin == "" {
		return errors.NewMissingRequiredError("--id, --schema, or --origin")
	}

	// Open catalog database
	configDir := config.GetPudlDir()
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

	// Collect data from entries
	var exportData []map[string]interface{}
	for _, entry := range entries {
		data, err := loadEntryData(entry.StoredPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to load data for %s: %v\n",
				idgen.HashToProquint(entry.ID), err)
			continue
		}
		exportData = append(exportData, data)
	}

	if len(exportData) == 0 {
		return errors.NewInputError("No data could be loaded from matching entries", "", "")
	}

	// Set up output writer
	var writer io.Writer = os.Stdout
	if exportOutput != "" {
		file, err := os.Create(exportOutput)
		if err != nil {
			return errors.WrapError(errors.ErrCodeFileSystem, "Failed to create output file", err)
		}
		defer file.Close()
		writer = file
	}

	// Export in specified format
	return writeExportData(writer, exportData, exportFormat, exportPretty)
}

// loadEntryData loads the stored data from an entry's path
func loadEntryData(storedPath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(storedPath)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		// Try YAML
		if yamlErr := yaml.Unmarshal(data, &result); yamlErr != nil {
			return nil, fmt.Errorf("failed to parse as JSON or YAML: %v", err)
		}
	}

	return result, nil
}

// writeExportData writes export data in the specified format
func writeExportData(w io.Writer, data []map[string]interface{}, format string, pretty bool) error {
	switch strings.ToLower(format) {
	case "json":
		return writeJSON(w, data, pretty)
	case "yaml":
		return writeYAML(w, data)
	case "ndjson":
		return writeNDJSON(w, data)
	case "csv":
		return writeCSV(w, data)
	default:
		return errors.NewInputError(fmt.Sprintf("Unknown format: %s", format),
			"Use one of: json, yaml, csv, ndjson", "")
	}
}

func writeJSON(w io.Writer, data []map[string]interface{}, pretty bool) error {
	encoder := json.NewEncoder(w)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	// If single entry, output just that object; otherwise output array
	if len(data) == 1 {
		return encoder.Encode(data[0])
	}
	return encoder.Encode(data)
}

func writeYAML(w io.Writer, data []map[string]interface{}) error {
	encoder := yaml.NewEncoder(w)
	defer encoder.Close()

	for _, item := range data {
		if err := encoder.Encode(item); err != nil {
			return err
		}
	}
	return nil
}

func writeNDJSON(w io.Writer, data []map[string]interface{}) error {
	encoder := json.NewEncoder(w)
	for _, item := range data {
		if err := encoder.Encode(item); err != nil {
			return err
		}
	}
	return nil
}

func writeCSV(w io.Writer, data []map[string]interface{}) error {
	if len(data) == 0 {
		return nil
	}

	// Collect all unique keys for headers
	keySet := make(map[string]bool)
	for _, item := range data {
		for key := range item {
			keySet[key] = true
		}
	}

	// Sort keys for consistent output
	var headers []string
	for key := range keySet {
		headers = append(headers, key)
	}

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write header
	if err := writer.Write(headers); err != nil {
		return err
	}

	// Write rows
	for _, item := range data {
		row := make([]string, len(headers))
		for i, header := range headers {
			if val, ok := item[header]; ok {
				row[i] = fmt.Sprintf("%v", val)
			}
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

