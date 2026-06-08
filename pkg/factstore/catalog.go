package factstore

import "github.com/chazu/pudl/internal/database"

// CatalogEntry is a catalog record describing an imported or derived artifact.
type CatalogEntry = database.CatalogEntry

// CatalogFilter selects catalog entries by field. Empty fields are ignored.
type CatalogFilter = database.FilterOptions

// CatalogQuery controls ordering and pagination of a catalog listing.
type CatalogQuery = database.QueryOptions

// CatalogResult is a page of catalog entries plus total/filtered counts.
type CatalogResult = database.QueryResult

// ListCatalog returns catalog entries matching the filter, with ordering and
// pagination from query. This is the typed catalog-access path; for ad-hoc
// joins against facts use Query with a rule referencing the catalog_entry
// relation.
func (s *Store) ListCatalog(filter CatalogFilter, query CatalogQuery) (*CatalogResult, error) {
	return s.db.QueryEntries(filter, query)
}
