package database

import "encoding/json"

// retireGuessedDependencies withdraws only assertions with the exact ownership
// convention of the retired model-dependency heuristic. Retraction preserves
// transaction-time history and leaves declared, binding and custom sources alone.
func (c *CatalogDB) retireGuessedDependencies() error {
	return c.WithFactTx(func(tx *FactTx) error {
		facts, err := tx.QueryFacts(FactFilter{Relation: "model_depends_on"})
		if err != nil {
			return err
		}
		for _, f := range facts {
			var args map[string]string
			if json.Unmarshal([]byte(f.Args), &args) != nil || len(args) != 2 || args["from"] == "" || args["to"] == "" || f.Source != "derived:"+args["from"] {
				continue
			}
			if err := tx.RetractFact(f.ID); err != nil {
				return err
			}
		}
		return nil
	})
}
