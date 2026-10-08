package importer

import (
	"encoding/json"
	"io"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/projection"
	"github.com/chazu/pudl/internal/schemaname"
)

// Re-importing data that is already cataloged used to be a silent no-op for
// --schema: dedup by content hash kept the first classification. With an
// explicit schema the record satisfies, the existing entry now moves to it.

// reobserve handles a record whose content is already cataloged: under an
// explicit --schema it may move the entry (see reassignExisting); and when the
// entry ends up under this record's schema it is again the latest observation
// of its resource, so its facts become current — a resource that changed and
// changed back is not left reporting the intermediate state.
func (c *collectionStream) reobserve(tx *database.CatalogTx, existing *database.CatalogEntry, item preparedItem) error {
	moved, err := reassignExisting(tx, existing, item.Validated, item.Entry)
	if err != nil {
		return err
	}
	if moved {
		c.reassigned++
	}
	if moved || existing.Schema == item.Entry.Schema {
		return projection.Observe(tx, item.Projection)
	}
	return nil
}

// reassignExisting applies the reassignment rule: only a record that
// satisfied the explicit --schema itself moves, and only when its schema
// differs from the stored one. want carries the new schema and identity.
func reassignExisting(tx *database.CatalogTx, existing *database.CatalogEntry, validated bool, want database.CatalogEntry) (bool, error) {
	if !validated || existing.Schema == want.Schema || want.ResourceID == nil {
		return false, nil
	}
	_, err := tx.ReassignEntry(database.Reassignment{
		ID: existing.ID, Schema: want.Schema, Confidence: want.Confidence,
		ResourceID: *want.ResourceID, IdentityJSON: want.IdentityJSON,
	})
	return err == nil, err
}

// reobserveAll handles a re-import of a whole collection file that is already
// cataloged: nothing new is stored, but its items may move to --schema and are
// the latest observation of their resources again.
func (c *collectionStream) reobserveAll() (int, error) {
	if _, err := c.spool.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	err := c.importer.catalogDB.WithCatalogTxContext(c.opts.Context, func(tx *database.CatalogTx) error {
		dec := json.NewDecoder(c.spool)
		dec.UseNumber()
		for {
			var item preparedItem
			if err := dec.Decode(&item); err == io.EOF {
				return nil
			} else if err != nil {
				return err
			}
			existing, err := tx.FindByContentHash(*item.Entry.ContentHash)
			if err != nil {
				return err
			}
			if existing == nil {
				continue
			}
			if err := c.reobserve(tx, existing, item); err != nil {
				return err
			}
		}
	})
	return c.reassigned, err
}

// reobserveDocument handles a re-import of a single document that is already
// cataloged: it may move to --schema, and it is again its resource's latest
// observation.
func (e *EnhancedImporter) reobserveDocument(opts ImportOptions, existing *database.CatalogEntry, sourcePath, format, origin string) (int, error) {
	data, _, err := e.analyzeData(sourcePath, format)
	if err != nil {
		return 0, err
	}
	var assigned schemaAssignment
	if opts.assignments != nil {
		assigned, err = opts.assignments.read(0)
	} else {
		assigned, err = e.assignSchema(data, opts, inference.InferenceHints{Origin: origin, Format: format})
	}
	if err != nil {
		return 0, err
	}
	want, resolved := e.identifiedEntry(data, assigned, *existing.ContentHash)
	moved := false
	err = e.catalogDB.WithCatalogTxContext(opts.Context, func(tx *database.CatalogTx) error {
		var err error
		if moved, err = reassignExisting(tx, existing, assigned.Validated, want); err != nil {
			return err
		}
		if !moved && existing.Schema != want.Schema {
			return nil
		}
		prepared, _ := projection.Prepare(e.projector, want.Schema, existing.ID, *want.ResourceID, resolved, data)
		return projection.Observe(tx, prepared)
	})
	if moved {
		return 1, err
	}
	return 0, err
}

// identifiedEntry is the schema and identity the importer derives for data
// under an assignment: only the fields reassignment needs.
func (e *EnhancedImporter) identifiedEntry(data any, assigned schemaAssignment, contentHash string) (database.CatalogEntry, bool) {
	values, err := identity.ExtractFieldValues(data, e.getSchemaIdentityFields(assigned.Schema))
	if err != nil {
		values = nil
	}
	rid := identity.ComputeResourceID(e.identityNamespace(assigned.Schema), values, contentHash)
	entry := database.CatalogEntry{Schema: schemaname.Normalize(assigned.Schema), Confidence: assigned.Confidence, ResourceID: &rid}
	if len(values) > 0 {
		if canonical, err := identity.CanonicalIdentityJSON(values); err == nil {
			entry.IdentityJSON = &canonical
		}
	}
	return entry, len(values) > 0
}
