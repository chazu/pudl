package pithdriver

import (
	"encoding/json"
	"fmt"

	"github.com/chazu/pith"

	"pudl/internal/database"
)

// factWords returns the fact driver vocabulary.
func factWords(db *database.CatalogDB) map[string]pith.Word {
	return map[string]pith.Word{
		"query":   wordFactQuery(db),
		"assert":  wordFactAssert(db),
		"retract": wordFactRetract(db),
	}
}

// fact/query ( pattern -- [facts] )
func wordFactQuery(db *database.CatalogDB) pith.Word {
	return func(vm *pith.VM) error {
		m, err := popMap(vm)
		if err != nil {
			return err
		}
		filter, err := mapToStruct[database.FactFilter](m)
		if err != nil {
			return err
		}
		facts, err := db.QueryFacts(filter)
		if err != nil {
			return err
		}
		results, err := structsToMaps(facts)
		if err != nil {
			return err
		}
		vm.Push(results)
		return nil
	}
}

// fact/assert ( subj pred obj -- )
func wordFactAssert(db *database.CatalogDB) pith.Word {
	return func(vm *pith.VM) error {
		obj, err := vm.Pop()
		if err != nil {
			return err
		}
		pred, err := popString(vm)
		if err != nil {
			return err
		}
		subj, err := popString(vm)
		if err != nil {
			return err
		}
		argsMap := map[string]any{
			"subject": subj,
			"object":  obj,
		}
		argsJSON, err := json.Marshal(argsMap)
		if err != nil {
			return fmt.Errorf("marshal args: %w", err)
		}
		f := database.Fact{
			Relation: pred,
			Args:     string(argsJSON),
			Source:   "pith",
		}
		_, err = db.AddFact(f)
		return err
	}
}

// fact/retract ( id -- )
func wordFactRetract(db *database.CatalogDB) pith.Word {
	return func(vm *pith.VM) error {
		id, err := popString(vm)
		if err != nil {
			return err
		}
		return db.RetractFact(id)
	}
}
