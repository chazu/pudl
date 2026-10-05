package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chazu/pudl/internal/database"
)

// CheckResourceVersionsAt reports legacy duplicate versions without renumbering
// immutable historical evidence.
func CheckResourceVersionsAt(root string) *CheckResult {
	if _, err := os.Stat(filepath.Join(root, "data", "sqlite", "catalog.db")); os.IsNotExist(err) {
		return &CheckResult{Status: "ok", Message: "No catalog versions to check"}
	}
	db, err := database.OpenCatalogDBReadOnly(root)
	if err != nil {
		return &CheckResult{Status: "warning", Message: "Unable to inspect resource versions", Details: err.Error()}
	}
	defer db.Close()
	result, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	if err != nil {
		return &CheckResult{Status: "warning", Message: "Unable to inspect resource versions", Details: err.Error()}
	}
	seen := map[string]string{}
	var duplicates []string
	for _, entry := range result.Entries {
		if entry.ResourceID == nil || entry.Version == nil {
			continue
		}
		key := fmt.Sprintf("%s:%d", *entry.ResourceID, *entry.Version)
		if first, ok := seen[key]; ok {
			duplicates = append(duplicates, fmt.Sprintf("resource %s version %d: %s and %s", *entry.ResourceID, *entry.Version, first, entry.ID))
		} else {
			seen[key] = entry.ID
		}
	}
	if len(duplicates) > 0 {
		return &CheckResult{Status: "warning", Message: "Duplicate historical resource versions", Details: strings.Join(duplicates, "\n"), Fix: "Preserve these records; re-observe affected resources. Historical versions are not renumbered automatically."}
	}
	return &CheckResult{Status: "ok", Message: "Resource versions are unique"}
}
