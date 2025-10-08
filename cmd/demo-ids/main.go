package main

import (
	"fmt"
	"os"
	"strings"

	"pudl/internal/idgen"
)

func main() {
	fmt.Println("PUDL Human-Friendly ID Generation Demo")
	fmt.Println("=====================================")
	fmt.Println()

	// Show current vs new ID formats
	demonstrateFormats()
	fmt.Println()

	// Show context-aware ID generation
	demonstrateContextAware()
	fmt.Println()

	// Show collection item IDs
	demonstrateCollectionItems()
	fmt.Println()

	// Show display formatting
	demonstrateDisplay()
}

func demonstrateFormats() {
	fmt.Println("🎯 Human-Friendly ID Formats")
	fmt.Println("----------------------------")

	// Human-friendly formats
	formats := []struct {
		name        string
		format      idgen.IDFormat
		prefix      string
		description string
	}{
		{"Short Code", idgen.FormatShortCode, "", "Compact alphanumeric codes"},
		{"Short Code (AWS)", idgen.FormatShortCode, "aws", "With context prefix"},
		{"Proquint", idgen.FormatProquint, "", "Pronounceable quintuplets"},
		{"Proquint (Collections)", idgen.FormatProquint, "col", "Easy to communicate verbally"},
		{"Readable", idgen.FormatReadable, "", "Human-memorable words"},
		{"Readable (Collections)", idgen.FormatReadable, "col", "For collections"},
		{"Compact", idgen.FormatCompact, "", "Date (MMDDYY) + short code"},
		{"Compact (K8s)", idgen.FormatCompact, "k8s", "Kubernetes context"},
		{"Sequential", idgen.FormatSequential, "data", "Ordered numbering"},
	}

	for _, f := range formats {
		gen := idgen.NewIDGenerator(f.format, f.prefix)
		id := gen.Generate()
		fmt.Printf("%-20s %s\n", f.name+":", id)
		fmt.Printf("%-20s %s (Length: %d)\n", "", f.description, len(id))
		fmt.Println()
	}
}

func demonstrateContextAware() {
	fmt.Println("🧠 Context-Aware ID Generation")
	fmt.Println("------------------------------")

	origins := []string{
		"aws-ec2-describe-instances",
		"kubectl-get-pods",
		"unknown-data-source",
		"collection-import",
	}

	for _, origin := range origins {
		manager := idgen.NewImporterIDManagerFromOrigin(origin)
		id := manager.GenerateMainID("/path/to/data.json", origin)
		fmt.Printf("Origin: %-25s → ID: %s\n", origin, id)
	}
}

func demonstrateCollectionItems() {
	fmt.Println("📦 Collection Item IDs")
	fmt.Println("---------------------")

	// Create a collection with different ID formats
	formats := []idgen.IDFormat{
		idgen.FormatProquint,
		idgen.FormatShortCode,
		idgen.FormatReadable,
		idgen.FormatCompact,
	}

	sampleItems := []map[string]interface{}{
		{"id": "user-123", "name": "John Doe"},
		{"externalId": "ext-456", "name": "Jane Smith"},
		{"name": "Bob Wilson"}, // No explicit ID
	}

	for _, format := range formats {
		fmt.Printf("\n%s Format:\n", strings.Title(string(format)))

		manager := idgen.NewImporterIDManager(idgen.IDConfig{
			Format: format,
			Prefix: "col",
		})

		collectionID := manager.GenerateCollectionID("/data/users.ndjson", "user-collection")
		fmt.Printf("  Collection: %s\n", collectionID)

		for i, item := range sampleItems {
			itemID := manager.GenerateItemID(collectionID, i, item)
			fmt.Printf("  Item %d:     %s\n", i, itemID)
		}
	}
}

func demonstrateDisplay() {
	fmt.Println("🎨 Display Formatting")
	fmt.Println("--------------------")

	displayHelper := idgen.NewIDDisplayHelper()

	testIDs := []string{
		"20241207_143052_aws-ec2-describe-instances",
		"abc123",
		"aws-def456",
		"lusab-babad",
		"col-gutih-tugad",
		"blue-cat-42",
		"col-fast-tree-91",
		"100724-a1b",
		"k8s-100824-x9z",
		"data-001",
	}

	fmt.Printf("%-40s %-12s %s\n", "Original ID", "Type", "Display Format")
	fmt.Println(strings.Repeat("-", 65))

	for _, id := range testIDs {
		idType := displayHelper.GetIDType(id)
		displayFormat := displayHelper.FormatForDisplay(id)
		fmt.Printf("%-40s %-12s %s\n", id, idType, displayFormat)
	}
}

func init() {
	// Ensure we can run this demo
	if len(os.Args) > 1 && os.Args[1] == "--help" {
		fmt.Println("PUDL ID Generation Demo")
		fmt.Println()
		fmt.Println("This demo shows different ID generation formats available in PUDL:")
		fmt.Println("  • Legacy: Current timestamp-based format (long)")
		fmt.Println("  • Proquint: Pronounceable quintuplets (e.g., lusab-babad)")
		fmt.Println("  • Short Code: 6-character alphanumeric codes")
		fmt.Println("  • Readable: Human-memorable adjective-noun-number format")
		fmt.Println("  • Compact: Date (MMDDYY) + 3-character code")
		fmt.Println("  • Sequential: Ordered numbering with prefixes")
		fmt.Println()
		fmt.Println("Usage: go run cmd/demo-ids/main.go")
		os.Exit(0)
	}
}
