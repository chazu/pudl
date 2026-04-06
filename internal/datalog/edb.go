package datalog

import (
	"encoding/json"
	"fmt"

	"pudl/internal/database"
)

// EDB provides access to base facts for the Datalog evaluator.
type EDB interface {
	// Scan returns all tuples for a given relation.
	Scan(relation string) ([]Tuple, error)
}

// MultiEDB combines multiple EDB sources. Queries are dispatched to
// all sources and results merged.
type MultiEDB struct {
	sources []EDB
}

// NewMultiEDB creates an EDB that reads from multiple sources.
func NewMultiEDB(sources ...EDB) *MultiEDB {
	return &MultiEDB{sources: sources}
}

func (m *MultiEDB) Scan(relation string) ([]Tuple, error) {
	var all []Tuple
	for _, src := range m.sources {
		tuples, err := src.Scan(relation)
		if err != nil {
			return nil, err
		}
		all = append(all, tuples...)
	}
	return all, nil
}

// FactsEDB reads from the bitemporal facts table (AsOfNow).
type FactsEDB struct {
	db *database.CatalogDB
}

// NewFactsEDB creates an EDB backed by the facts table.
func NewFactsEDB(db *database.CatalogDB) *FactsEDB {
	return &FactsEDB{db: db}
}

func (f *FactsEDB) Scan(relation string) ([]Tuple, error) {
	facts, err := f.db.QueryFacts(database.FactFilter{Relation: relation})
	if err != nil {
		return nil, fmt.Errorf("facts scan %s: %w", relation, err)
	}

	tuples := make([]Tuple, 0, len(facts))
	for _, fact := range facts {
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(fact.Args), &args); err != nil {
			continue // skip malformed JSON
		}
		tuples = append(tuples, Tuple{Relation: relation, Args: args})
	}
	return tuples, nil
}

// CatalogEDB exposes catalog_entries as a "catalog_entry" relation.
type CatalogEDB struct {
	db *database.CatalogDB
}

// NewCatalogEDB creates an EDB backed by the catalog_entries table.
func NewCatalogEDB(db *database.CatalogDB) *CatalogEDB {
	return &CatalogEDB{db: db}
}

func (c *CatalogEDB) Scan(relation string) ([]Tuple, error) {
	if relation != "catalog_entry" {
		return nil, nil // only serves this relation
	}

	result, err := c.db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	if err != nil {
		return nil, fmt.Errorf("catalog scan: %w", err)
	}

	tuples := make([]Tuple, 0, len(result.Entries))
	for _, e := range result.Entries {
		args := map[string]interface{}{
			"id":     e.ID,
			"schema": e.Schema,
			"origin": e.Origin,
			"format": e.Format,
		}
		if e.EntryType != nil {
			args["entry_type"] = *e.EntryType
		}
		if e.Definition != nil {
			args["definition"] = *e.Definition
		}
		if e.Status != nil {
			args["status"] = *e.Status
		}
		if e.ResourceID != nil {
			args["resource_id"] = *e.ResourceID
		}
		tuples = append(tuples, Tuple{Relation: "catalog_entry", Args: args})
	}
	return tuples, nil
}

// MemoryEDB is an in-memory EDB for testing.
type MemoryEDB struct {
	facts map[string][]Tuple
}

// NewMemoryEDB creates an in-memory EDB.
func NewMemoryEDB() *MemoryEDB {
	return &MemoryEDB{facts: make(map[string][]Tuple)}
}

// Add adds a tuple to the in-memory store.
func (m *MemoryEDB) Add(t Tuple) {
	m.facts[t.Relation] = append(m.facts[t.Relation], t)
}

func (m *MemoryEDB) Scan(relation string) ([]Tuple, error) {
	return m.facts[relation], nil
}
