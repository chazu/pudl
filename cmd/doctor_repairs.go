package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/projection"
)

type repairAction struct {
	Reason  string   `json:"reason"`
	Preview []string `json:"preview_argv"`
	Apply   []string `json:"apply_argv,omitempty"`
}

// suggestedRepairs only describes reviewable operations; doctor never changes
// classification or projection as a side effect of diagnosis.
func suggestedRepairs(catalog []catalogDiagnostic) ([]repairAction, error) {
	actions := []repairAction{}
	for _, entry := range catalog {
		if entry.Status == "mismatch" {
			actions = append(actions, repairAction{Reason: "schema inference changed for " + entry.ID, Preview: []string{"pudl", "schema", "reinfer", "--entry", entry.ID, "--dry-run"}, Apply: []string{"pudl", "schema", "reinfer", "--entry", entry.ID, "--force"}})
		}
	}
	if _, err := os.Stat(filepath.Join(effectivePudlDir(), "data", "sqlite", "catalog.db")); os.IsNotExist(err) {
		return actions, nil
	}
	db, err := database.OpenCatalogDBReadOnly(effectivePudlDir())
	if err != nil {
		return actions, err
	}
	defer db.Close()
	reg, err := projectionRegistry()
	if err != nil {
		return actions, err
	}
	reasons, err := projection.Stale(db, reg)
	if err != nil {
		return actions, err
	}
	if len(reasons) > 0 {
		actions = append(actions, repairAction{Reason: fmt.Sprintf("projected facts need review: %v", reasons), Preview: []string{"pudl", "facts", "reproject", "--dry-run"}, Apply: []string{"pudl", "facts", "reproject"}})
	}
	unresolved, err := db.ListUnresolvedItemSchemas("")
	if err != nil {
		return actions, err
	}
	if len(unresolved) > 0 {
		actions = append(actions, repairAction{Reason: fmt.Sprintf("%d item schema references remain unresolved; load the required definitions before reclassifying", len(unresolved)), Preview: []string{"pudl", "schema", "list", "--json"}, Apply: []string{"pudl", "reclassify"}})
	}
	return actions, nil
}
