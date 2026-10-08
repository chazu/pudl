package database

import (
	"fmt"
	"time"

	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/schemaname"
)

// Reassignment moves an existing entry to another schema, with the identity
// that schema derives for the same content.
type Reassignment struct {
	ID           string
	Schema       string
	Confidence   float64
	ResourceID   string
	IdentityJSON *string
}

// ReassignEntry applies r inside the transaction. When the resource ID
// changes the entry joins the new resource's version chain at its end, so a
// chain never holds two entries with one version number. It returns the
// entry's version afterwards.
func (t *CatalogTx) ReassignEntry(r Reassignment) (int, error) {
	current, err := getEntryIn(t.q, r.ID)
	if err != nil {
		return 0, err
	}
	version := 1
	if current.Version != nil {
		version = *current.Version
	}
	if current.ResourceID == nil || *current.ResourceID != r.ResourceID {
		latest, err := getLatestVersionIn(t.q, r.ResourceID)
		if err != nil {
			return 0, err
		}
		version = latest + 1
	}
	res, err := t.q.Exec(`UPDATE catalog_entries SET schema = ?, confidence = ?, resource_id = ?, identity_json = ?, version = ?, updated_at = ? WHERE id = ?`,
		schemaname.Normalize(r.Schema), r.Confidence, r.ResourceID, r.IdentityJSON, version, formatCatalogTime(time.Now()), r.ID)
	if err != nil {
		return 0, errors.WrapError(errors.ErrCodeDatabaseError, "failed to reassign catalog entry", err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return 0, fmt.Errorf("reassign %s: entry not found", r.ID)
	}
	return version, nil
}
