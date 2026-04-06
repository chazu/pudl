// Package factstore provides public access to pudl's bitemporal fact store.
// This is the external API for consumers like nous.
package factstore

import (
	"pudl/internal/database"
)

// Fact is a typed assertion in the bitemporal store.
type Fact = database.Fact

// FactFilter specifies criteria for querying facts.
type FactFilter = database.FactFilter

// Store provides read/write access to the bitemporal fact store.
type Store struct {
	db *database.CatalogDB
}

// Open creates a Store backed by the pudl catalog at the given config directory.
func Open(pudlDir string) (*Store, error) {
	db, err := database.NewCatalogDB(pudlDir)
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// AddFact inserts a new fact.
func (s *Store) AddFact(f Fact) (Fact, error) {
	return s.db.AddFact(f)
}

// QueryFacts returns facts matching the filter with bitemporal scoping.
func (s *Store) QueryFacts(filter FactFilter) ([]Fact, error) {
	return s.db.QueryFacts(filter)
}

// RetractFact marks a fact as retracted (sets tx_end).
func (s *Store) RetractFact(id string) error {
	return s.db.RetractFact(id)
}

// InvalidateFact marks a fact as no longer valid (sets valid_end).
func (s *Store) InvalidateFact(id string) error {
	return s.db.InvalidateFact(id)
}

// DB returns the underlying CatalogDB for use with the evaluator.
func (s *Store) DB() *database.CatalogDB {
	return s.db
}
