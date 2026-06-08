// Package factstore provides public access to pudl's bitemporal fact store and
// Datalog query engine. This is the external API for consumers; it does not
// require importing any pudl/internal package.
package factstore

import (
	"pudl/internal/database"
	"pudl/internal/datalog"
)

// Fact is a typed assertion in the bitemporal store.
type Fact = database.Fact

// FactFilter specifies criteria for querying facts.
type FactFilter = database.FactFilter

// Rule is a Datalog inference rule. Re-exported so query-only consumers need
// only this package; load rules with pkg/eval.
type Rule = datalog.Rule

// Tuple is a ground fact returned by a query.
type Tuple = datalog.Tuple

// Store provides read/write and query access to a pudl data store.
type Store struct {
	db *database.CatalogDB
}

// Open creates a Store backed by the pudl catalog at the given config directory
// (e.g. factstore.GlobalDir() or a workspace's repo .pudl directory).
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

// QueryOptions specifies a Datalog query against the store.
type QueryOptions struct {
	// Relation is the head relation to query (required).
	Relation string

	// Constraints filter results by argument value (field=value pairs).
	Constraints map[string]interface{}

	// Rules are the Datalog rules to evaluate. Load them with
	// eval.LoadRulesFromPaths or eval.ParseRulesFromSource. A query against a
	// base relation with no derived rules returns matching facts directly.
	Rules []Rule

	// ValidAt and TxAt select bitemporal evaluation modes. Both nil evaluates
	// over current facts; setting either evaluates over the historical facts
	// table. Values are Unix timestamps.
	ValidAt *int64
	TxAt    *int64
}

// Query evaluates Datalog rules over the store and returns tuples for the
// requested relation. It runs the SQL evaluator for non-recursive rules and a
// recursive fixpoint fallback for recursive rules.
func (s *Store) Query(opts QueryOptions) ([]Tuple, error) {
	scope := datalog.TemporalScope{ValidAt: opts.ValidAt, TxAt: opts.TxAt}
	return datalog.Evaluate(s.db, opts.Rules, opts.Relation, opts.Constraints, scope)
}
