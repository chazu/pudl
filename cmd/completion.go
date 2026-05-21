package cmd

import (
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/idgen"
	"pudl/internal/schema"
	"pudl/internal/vault"
)

// completeProquintIDs returns a completion function for proquint entry IDs
func completeProquintIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	// dataPath is ~/.pudl/data, config dir (for database) is ~/.pudl
	configDir := filepath.Dir(cfg.DataPath)

	catalogDB, err := database.NewCatalogDB(configDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer catalogDB.Close()

	// Get all entries
	result, err := catalogDB.QueryEntries(database.FilterOptions{}, database.QueryOptions{Limit: 100})
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	for _, entry := range result.Entries {
		proquint := idgen.HashToProquint(entry.ID)
		// Filter by prefix if user has started typing
		if toComplete == "" || strings.HasPrefix(proquint, toComplete) {
			// Add description for better UX
			desc := entry.Schema
			if entry.CollectionType != nil && *entry.CollectionType == "collection" {
				desc += " 📦"
			}
			completions = append(completions, proquint+"\t"+desc)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeSchemaNames returns a completion function for schema names
func completeSchemaNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	manager := schema.NewManager(cfg.SchemaPath)
	schemasByPackage, err := manager.ListSchemas()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	for _, schemas := range schemasByPackage {
		for _, s := range schemas {
			// Filter by prefix if user has started typing
			if toComplete == "" || strings.HasPrefix(s.FullName, toComplete) {
				completions = append(completions, s.FullName)
			}
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeFormats returns a completion function for file formats
func completeFormats(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	formats := []string{"json", "yaml", "csv", "ndjson"}
	var completions []string
	for _, f := range formats {
		if toComplete == "" || strings.HasPrefix(f, toComplete) {
			completions = append(completions, f)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeSchemaPackages returns a completion function for schema package names
func completeSchemaPackages(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	manager := schema.NewManager(cfg.SchemaPath)
	packages, err := manager.GetPackages()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	for _, pkg := range packages {
		if toComplete == "" || strings.HasPrefix(pkg, toComplete) {
			completions = append(completions, pkg)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeSortByOptions returns a completion function for sort-by options
func completeSortByOptions(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	options := []string{
		"timestamp\tSort by import time",
		"size\tSort by file size",
		"records\tSort by record count",
		"schema\tSort by schema name",
		"origin\tSort by data origin",
	}
	var completions []string
	for _, opt := range options {
		parts := strings.Split(opt, "\t")
		if toComplete == "" || strings.HasPrefix(parts[0], toComplete) {
			completions = append(completions, opt)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeOrigins returns a completion function for data origins
func completeOrigins(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	// dataPath is ~/.pudl/data, config dir (for database) is ~/.pudl
	configDir := filepath.Dir(cfg.DataPath)

	catalogDB, err := database.NewCatalogDB(configDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer catalogDB.Close()

	// Get distinct origins
	origins, err := catalogDB.GetUniqueValues("origin")
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	for _, origin := range origins {
		// Filter by prefix if user has started typing
		if toComplete == "" || strings.HasPrefix(origin, toComplete) {
			completions = append(completions, origin)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeEntryIDs returns a completion function for recent entry IDs
func completeEntryIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	// dataPath is ~/.pudl/data, config dir (for database) is ~/.pudl
	configDir := filepath.Dir(cfg.DataPath)

	catalogDB, err := database.NewCatalogDB(configDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer catalogDB.Close()

	// Get recent entries (limit to 50 for performance)
	result, err := catalogDB.QueryEntries(database.FilterOptions{}, database.QueryOptions{
		Limit:   50,
		SortBy:  "timestamp",
		Reverse: true,
	})
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	for _, entry := range result.Entries {
		proquint := idgen.HashToProquint(entry.ID)
		// Filter by prefix if user has started typing
		if toComplete == "" || strings.HasPrefix(proquint, toComplete) {
			// Add description for better UX
			desc := entry.Schema
			completions = append(completions, proquint+"\t"+desc)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeRelations returns a completion function for fact relation names
func completeRelations(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	configDir := filepath.Dir(cfg.DataPath)

	catalogDB, err := database.NewCatalogDB(configDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer catalogDB.Close()

	relations, err := catalogDB.GetDistinctRelations()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	for _, r := range relations {
		if toComplete == "" || strings.HasPrefix(r, toComplete) {
			completions = append(completions, r)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeSources returns a completion function for fact source names
func completeSources(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	configDir := filepath.Dir(cfg.DataPath)

	catalogDB, err := database.NewCatalogDB(configDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer catalogDB.Close()

	sources, err := catalogDB.GetDistinctSources()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	for _, s := range sources {
		if toComplete == "" || strings.HasPrefix(s, toComplete) {
			completions = append(completions, s)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeVaultPaths returns a completion function for vault secret paths
func completeVaultPaths(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	v, err := vault.New(cfg.VaultBackend, config.GetPudlDir())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer v.Close()

	paths, err := v.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	for _, p := range paths {
		if toComplete == "" || strings.HasPrefix(p, toComplete) {
			completions = append(completions, p)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeFactIDs returns a completion function for fact IDs (hex prefix)
func completeFactIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	configDir := filepath.Dir(cfg.DataPath)

	catalogDB, err := database.NewCatalogDB(configDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer catalogDB.Close()

	rows, err := catalogDB.DB().Query(
		"SELECT id, relation FROM current_facts ORDER BY valid_start DESC LIMIT 50")
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer rows.Close()

	var completions []string
	for rows.Next() {
		var id, relation string
		if err := rows.Scan(&id, &relation); err != nil {
			continue
		}
		short := id[:12]
		if toComplete == "" || strings.HasPrefix(short, toComplete) || strings.HasPrefix(id, toComplete) {
			completions = append(completions, short+"\t"+relation)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeObservationKinds returns known observation kinds for completion
func completeObservationKinds(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	kinds := []string{
		"bug\tSoftware bug",
		"obstacle\tBlocking issue",
		"pattern\tRecurring pattern",
		"decision\tArchitectural decision",
		"risk\tIdentified risk",
		"debt\tTechnical debt",
		"opportunity\tImprovement opportunity",
	}
	var completions []string
	for _, k := range kinds {
		parts := strings.Split(k, "\t")
		if toComplete == "" || strings.HasPrefix(parts[0], toComplete) {
			completions = append(completions, k)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

