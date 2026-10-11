package database

import (
	"encoding/json"
	"fmt"
)

func (c *CatalogDB) ensureObservationCompleteness() error {
	for _, col := range []struct{ name, typ string }{{"scope", "TEXT NOT NULL DEFAULT ''"}, {"complete", "INTEGER NOT NULL DEFAULT 0"}, {"schemas", "TEXT NOT NULL DEFAULT '[]'"}} {
		exists, err := c.columnExists("observe_snapshots", col.name)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := c.db.Exec("ALTER TABLE observe_snapshots ADD COLUMN " + col.name + " " + col.typ); err != nil {
				return err
			}
		}
	}
	return nil
}

func snapshotSchemasJSON(schemas []string) string {
	if schemas == nil {
		return "[]"
	}
	b, _ := json.Marshal(schemas)
	return string(b)
}

// CurrentScopeSnapshot resolves an explicitly named population. Multiple source
// owners are an ambiguity, never an invitation to mix inventories silently.
func (c *CatalogDB) CurrentScopeSnapshot(scope string) (*ObserveSnapshot, error) {
	snapshots, err := c.ListObserveSnapshots("", 0)
	if err != nil {
		return nil, err
	}
	var selected *ObserveSnapshot
	for _, s := range snapshots {
		if s.Scope != scope {
			continue
		}
		if selected == nil {
			copy := s
			selected = &copy
			continue
		}
		if s.Model != selected.Model || s.Source != selected.Source || s.Workspace != selected.Workspace || s.Origin != selected.Origin {
			return nil, fmt.Errorf("scope %q has multiple observation owners; select exact snapshot IDs", scope)
		}
	}
	return selected, nil
}
