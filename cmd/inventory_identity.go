package cmd

import "github.com/chazu/pudl/internal/inventory"

// inventoryPolicy loads how the workspace's schemas identify inventory
// records (see inventory.LoadPolicy).
func inventoryPolicy() (inventory.Policy, error) {
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return inventory.Policy{}, err
	}
	return inventory.LoadPolicy(effectiveSchemaPaths(cfg))
}
