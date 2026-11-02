package policyfacts

import (
	"fmt"

	"github.com/chazu/pudl/internal/database"
)

// Stale reports, without writing, why a Refresh would change stored results.
// A result is stale when its policy digest differs from the loaded policy's.
// Reasons are empty when every result is current.
func (e *Evaluator) Stale(db *database.CatalogDB) ([]string, error) {
	states, err := db.ListPolicyState("", "")
	if err != nil {
		return nil, err
	}
	entries, err := db.CurrentResourceEntries()
	if err != nil {
		return nil, err
	}
	current := map[string]database.CatalogEntry{}
	for _, entry := range entries {
		current[*entry.ResourceID] = entry
	}
	have := map[string]bool{}
	var digest, unloaded, superseded int
	for _, s := range states {
		have[s.ResourceID+"\x00"+s.Policy] = true
		entry, found := current[s.ResourceID]
		_, loaded := e.lookup(s.Policy)
		switch {
		case !found || entry.ID != s.EntryID || entry.Schema != s.Type:
			superseded++
		case e.IsStale(s):
			digest++
		case !loaded && !e.broken[s.Policy]:
			unloaded++
		}
	}
	missing := 0
	for _, entry := range entries {
		for _, p := range e.Applicable(entry.Schema) {
			if !have[*entry.ResourceID+"\x00"+p.Name] {
				missing++
			}
		}
	}
	var reasons []string
	for _, r := range []struct {
		n      int
		format string
	}{
		{digest, "%d policy results are stale: evaluated with an older version of their policy"},
		{unloaded, "%d policy results belong to policies that are no longer loaded"},
		{superseded, "%d policy results describe entries that were deleted, re-identified, reassigned or superseded"},
		{missing, "%d resource and policy pairs have never been evaluated"},
	} {
		if r.n > 0 {
			reasons = append(reasons, fmt.Sprintf(r.format, r.n))
		}
	}
	return reasons, nil
}
