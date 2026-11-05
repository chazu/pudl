package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chazu/pudl/internal/lister"
)

// printShowSection prints a titled section of `pudl show` text output.
func printShowSection(title string) {
	separator := strings.Repeat("=", 60)
	fmt.Fprintf(outw(), "\n%s\n%s\n%s\n", separator, title, separator)
}

// printEntryData prints the stored record: JSON indented with its values and
// key order unchanged, YAML re-rendered, other formats verbatim. Sensitive
// fields were redacted when the record was stored.
func printEntryData(entry lister.ListEntry) {
	printShowSection("RAW DATA")
	content, err := os.ReadFile(entry.StoredPath)
	if err != nil {
		fmt.Fprintf(outw(), "Error reading raw data: %v\n", err)
		return
	}
	fmt.Fprintf(outw(), "%s\n", bytes.TrimRight(formatStoredData(content, entry.Format), "\n"))
}

// formatStoredData renders stored content for reading, falling back to the
// content itself when it does not parse as its format.
func formatStoredData(content []byte, format string) []byte {
	switch strings.ToLower(format) {
	case "json":
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, content, "", "  "); err == nil {
			return pretty.Bytes()
		}
	case "yaml":
		var data any
		if err := yaml.Unmarshal(content, &data); err == nil {
			if pretty, err := yaml.Marshal(data); err == nil {
				return pretty
			}
		}
	}
	return content
}
