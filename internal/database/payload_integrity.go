package database

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/chazu/pudl/internal/artifacts"
	"github.com/chazu/pudl/internal/idgen"
)

type PayloadIssue struct {
	EntryID string `json:"entry_id"`
	Path    string `json:"path"`
	Error   string `json:"error"`
}

// VerifyPayloads checks stored bytes against retained content hashes. Streamed
// items hash decoded JSON with sorted object keys; document/receipt hashes use
// their exact source bytes. It never rewrites or reclassifies evidence.
func (c *CatalogDB) VerifyPayloads(ctx context.Context) ([]PayloadIssue, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	entries, err := c.QueryEntriesContext(ctx, FilterOptions{}, QueryOptions{})
	if err != nil {
		return nil, err
	}
	var issues []PayloadIssue
	for _, entry := range entries.Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.StoredPath == "" {
			continue
		}
		actual, _, err := artifacts.HashContext(ctx, entry.StoredPath)
		expected := ""
		if entry.ContentHash != nil {
			if bytes, decodeErr := hex.DecodeString(*entry.ContentHash); decodeErr == nil && len(bytes) == 32 {
				expected = hex.EncodeToString(bytes)
			}
		}
		if err == nil && expected != "" && actual != expected {
			if entry.CollectionType != nil && *entry.CollectionType == "item" && entry.Format == "json" {
				body, readErr := os.ReadFile(entry.StoredPath)
				if readErr != nil {
					err = readErr
				} else if value, decodeErr := idgen.DecodeJSONExact(body); decodeErr != nil {
					err = decodeErr
				} else if canonical, marshalErr := json.Marshal(value); marshalErr != nil {
					err = marshalErr
				} else {
					actual = idgen.ComputeContentID(canonical)
				}
			}
			if err == nil && actual != expected {
				err = fmt.Errorf("content hash mismatch (expected %s, found %s)", expected, actual)
			}
		}
		if err != nil {
			issues = append(issues, PayloadIssue{EntryID: entry.ID, Path: entry.StoredPath, Error: err.Error()})
		}
	}
	return issues, nil
}
