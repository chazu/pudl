package cmd

import (
	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/lister"
)

// effectiveDataDir is the raw-payload and metadata directory of the
// invocation's resolved workspace. config.yaml's data_path is not consulted:
// the global configuration records absolute paths, so a copied workspace would
// otherwise read and write the original's data.
func effectiveDataDir() string {
	return config.DataDir(effectivePudlDir())
}

// openWorkspaceCatalog validates the workspace configuration and opens the
// resolved workspace's catalog.
func openWorkspaceCatalog() (*database.CatalogDB, error) {
	if _, err := loadEffectiveConfig(); err != nil {
		return nil, err
	}
	return database.NewCatalogDB(effectivePudlDir())
}

// openWorkspaceLister validates the workspace configuration and opens a lister
// over the resolved workspace's catalog.
func openWorkspaceLister() (*lister.Lister, error) {
	if _, err := loadEffectiveConfig(); err != nil {
		return nil, err // Already a PUDLError from loadEffectiveConfig()
	}
	l, err := lister.New(effectiveDataDir())
	if err != nil {
		return nil, errors.NewSystemError("Failed to initialize lister", err)
	}
	return l, nil
}
