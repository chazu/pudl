package pithdriver

import (
	"github.com/chazu/pith"

	"pudl/internal/database"
)

// catalogWords returns the catalog driver vocabulary.
func catalogWords(db *database.CatalogDB) map[string]pith.Word {
	return map[string]pith.Word{
		"query": wordCatalogQuery(db),
		"get":   wordCatalogGet(db),
		"count": wordCatalogCount(db),
	}
}

// catalog/query ( filters -- [entries] )
func wordCatalogQuery(db *database.CatalogDB) pith.Word {
	return func(vm *pith.VM) error {
		m, err := popMap(vm)
		if err != nil {
			return err
		}
		filters, err := mapToStruct[database.FilterOptions](m)
		if err != nil {
			return err
		}
		result, err := db.QueryEntries(filters, database.QueryOptions{})
		if err != nil {
			return err
		}
		entries, err := structsToMaps(result.Entries)
		if err != nil {
			return err
		}
		vm.Push(entries)
		return nil
	}
}

// catalog/get ( id -- entry )
func wordCatalogGet(db *database.CatalogDB) pith.Word {
	return func(vm *pith.VM) error {
		id, err := popString(vm)
		if err != nil {
			return err
		}
		entry, err := db.GetEntryByProquint(id)
		if err != nil {
			entry, err = db.GetEntry(id)
			if err != nil {
				return err
			}
		}
		m, err := structToMap(entry)
		if err != nil {
			return err
		}
		vm.Push(m)
		return nil
	}
}

// catalog/count ( filters -- n )
func wordCatalogCount(db *database.CatalogDB) pith.Word {
	return func(vm *pith.VM) error {
		m, err := popMap(vm)
		if err != nil {
			return err
		}
		filters, err := mapToStruct[database.FilterOptions](m)
		if err != nil {
			return err
		}
		result, err := db.QueryEntries(filters, database.QueryOptions{Limit: 0})
		if err != nil {
			return err
		}
		vm.Push(result.FilteredCount)
		return nil
	}
}
