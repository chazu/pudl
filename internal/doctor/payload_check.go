package doctor

import (
	"context"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/database"
)

func CheckPayloadsAt(ctx context.Context, root string) *CheckResult {
	db, err := database.OpenCatalogDBReadOnly(root)
	if err != nil {
		return &CheckResult{Status: "error", Message: "Cannot verify payloads", Details: err.Error()}
	}
	defer db.Close()
	issues, err := db.VerifyPayloads(ctx)
	if err != nil {
		return &CheckResult{Status: "error", Message: "Payload verification failed", Details: err.Error()}
	}
	if len(issues) == 0 {
		return &CheckResult{Status: "ok", Message: "Referenced payload files and available SHA256 hashes verified"}
	}
	details := make([]string, 0, len(issues))
	for _, issue := range issues {
		details = append(details, fmt.Sprintf("%s (%s): %s", issue.EntryID, issue.Path, issue.Error))
	}
	return &CheckResult{Status: "error", Message: "Retained payload evidence is damaged or missing", Details: strings.Join(details, "\n"), Fix: "Restore verified evidence from a bundle or re-observe; hashes are never repaired automatically"}
}
