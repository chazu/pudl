package doctor

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	"pudl/internal/config"
	"pudl/internal/database"
)

// CheckResult represents the result of a health check
type CheckResult struct {
	Status  string // "ok", "warning", "error"
	Message string
	Details string
	Fix     string
}

// HealthCheck represents a single health check
type HealthCheck struct {
	Name      string
	CheckFunc func() *CheckResult
}

// CheckWorkspaceStructure verifies that ~/.pudl directories exist
func CheckWorkspaceStructure() *CheckResult {
	pudlDir := config.GetPudlDir()

	// Check if pudl directory exists
	if _, err := os.Stat(pudlDir); os.IsNotExist(err) {
		return &CheckResult{
			Status:  "error",
			Message: "PUDL workspace not initialized",
			Details: fmt.Sprintf("Directory %s does not exist", pudlDir),
			Fix:     "Run 'pudl init' to initialize your workspace",
		}
	}

	// Check required subdirectories
	requiredDirs := []string{"schema", "data"}
	for _, dir := range requiredDirs {
		dirPath := filepath.Join(pudlDir, dir)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			return &CheckResult{
				Status:  "warning",
				Message: fmt.Sprintf("Missing directory: %s", dir),
				Details: fmt.Sprintf("Directory %s does not exist", dirPath),
				Fix:     "Run 'pudl init' to recreate missing directories",
			}
		}
	}

	// Check config file
	configPath := config.GetConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &CheckResult{
			Status:  "warning",
			Message: "Config file not found",
			Details: fmt.Sprintf("File %s does not exist", configPath),
			Fix:     "Run 'pudl init' to create config file",
		}
	}

	return &CheckResult{
		Status:  "ok",
		Message: "Workspace structure is valid",
		Details: fmt.Sprintf("All required directories exist at %s", pudlDir),
	}
}

// CheckDatabaseIntegrity runs PRAGMA integrity_check on the catalog database
func CheckDatabaseIntegrity() *CheckResult {
	pudlDir := config.GetPudlDir()
	dbPath := filepath.Join(pudlDir, "data", "sqlite", "catalog.db")

	// Check if database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return &CheckResult{
			Status:  "warning",
			Message: "Catalog database not found",
			Details: fmt.Sprintf("Database file %s does not exist", dbPath),
			Fix:     "Import some data to create the database",
		}
	}

	// Open database and run integrity check
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return &CheckResult{
			Status:  "error",
			Message: "Failed to open database",
			Details: err.Error(),
			Fix:     "Check file permissions and database integrity",
		}
	}
	defer db.Close()

	var result string
	err = db.QueryRow("PRAGMA integrity_check").Scan(&result)
	if err != nil {
		return &CheckResult{
			Status:  "error",
			Message: "Database integrity check failed",
			Details: err.Error(),
			Fix:     "Restore from backup or reinitialize database",
		}
	}

	if result != "ok" {
		return &CheckResult{
			Status:  "error",
			Message: "Database corruption detected",
			Details: result,
			Fix:     "Restore from backup or reinitialize database",
		}
	}

	return &CheckResult{
		Status:  "ok",
		Message: "Database integrity check passed",
		Details: "Catalog database is healthy",
	}
}

// CheckSchemaRepository verifies CUE module exists
func CheckSchemaRepository() *CheckResult {
	cfg, err := config.Load()
	if err != nil {
		return &CheckResult{
			Status:  "error",
			Message: "Failed to load configuration",
			Details: err.Error(),
			Fix:     "Check config file at " + config.GetConfigPath(),
		}
	}

	// Check if schema directory exists
	if _, err := os.Stat(cfg.SchemaPath); os.IsNotExist(err) {
		return &CheckResult{
			Status:  "error",
			Message: "Schema directory not found",
			Details: fmt.Sprintf("Directory %s does not exist", cfg.SchemaPath),
			Fix:     "Run 'pudl init' to create schema directory",
		}
	}

	// Check for cue.mod
	cueModPath := filepath.Join(cfg.SchemaPath, "cue.mod")
	if _, err := os.Stat(cueModPath); os.IsNotExist(err) {
		return &CheckResult{
			Status:  "warning",
			Message: "CUE module not initialized",
			Details: fmt.Sprintf("Directory %s does not exist", cueModPath),
			Fix:     "Run 'pudl init' to initialize CUE module",
		}
	}

	return &CheckResult{
		Status:  "ok",
		Message: "Schema repository is valid",
		Details: fmt.Sprintf("CUE module found at %s", cueModPath),
	}
}

// CheckGitRepository verifies git is initialized
func CheckGitRepository() *CheckResult {
	cfg, err := config.Load()
	if err != nil {
		return &CheckResult{
			Status:  "warning",
			Message: "Failed to load configuration",
			Details: err.Error(),
		}
	}

	gitDir := filepath.Join(cfg.SchemaPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return &CheckResult{
			Status:  "warning",
			Message: "Git repository not initialized",
			Details: fmt.Sprintf("Directory %s does not exist", gitDir),
			Fix:     "Run 'pudl init' to initialize git repository",
		}
	}

	return &CheckResult{
		Status:  "ok",
		Message: "Git repository is initialized",
		Details: fmt.Sprintf("Git repository found at %s", gitDir),
	}
}

// CheckDirectoryStructure validates the ~/.pudl/ directory structure,
// ensuring expected subdirectories exist and no unexpected top-level entries are present.
// Inspired by defn's manifest/manifest.cue close({}) pattern for exhaustive validation.
func CheckDirectoryStructure() *CheckResult {
	pudlDir := config.GetPudlDir()

	// Check if pudl directory exists
	if _, err := os.Stat(pudlDir); os.IsNotExist(err) {
		return &CheckResult{
			Status:  "error",
			Message: "PUDL workspace not initialized",
			Details: fmt.Sprintf("Directory %s does not exist", pudlDir),
			Fix:     "Run 'pudl init' to initialize your workspace",
		}
	}

	// Check data directory and its required subdirectories
	dataDir := filepath.Join(pudlDir, "data")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		return &CheckResult{
			Status:  "error",
			Message: "Missing data directory",
			Details: fmt.Sprintf("Directory %s does not exist", dataDir),
			Fix:     "Run 'pudl init' to recreate missing directories",
		}
	}

	for _, sub := range []string{"raw", "sqlite"} {
		subPath := filepath.Join(dataDir, sub)
		if _, err := os.Stat(subPath); os.IsNotExist(err) {
			return &CheckResult{
				Status:  "warning",
				Message: fmt.Sprintf("Missing data subdirectory: %s", sub),
				Details: fmt.Sprintf("Directory %s does not exist", subPath),
				Fix:     "Run 'pudl init' to recreate missing directories",
			}
		}
	}

	// Check catalog.db exists if any data has been imported
	rawDir := filepath.Join(dataDir, "raw")
	hasRawData := false
	if entries, err := os.ReadDir(rawDir); err == nil && len(entries) > 0 {
		hasRawData = true
	}
	if hasRawData {
		catalogPath := filepath.Join(dataDir, "sqlite", "catalog.db")
		if _, err := os.Stat(catalogPath); os.IsNotExist(err) {
			return &CheckResult{
				Status:  "warning",
				Message: "Catalog database missing but raw data exists",
				Details: fmt.Sprintf("Raw data found in %s but %s does not exist", rawDir, catalogPath),
				Fix:     "Re-import data or restore catalog database",
			}
		}
	}

	// Check schema directory structure
	schemaDir := filepath.Join(pudlDir, "schema")
	if _, err := os.Stat(schemaDir); os.IsNotExist(err) {
		return &CheckResult{
			Status:  "error",
			Message: "Missing schema directory",
			Details: fmt.Sprintf("Directory %s does not exist", schemaDir),
			Fix:     "Run 'pudl init' to recreate missing directories",
		}
	}

	cueModPath := filepath.Join(schemaDir, "cue.mod")
	if _, err := os.Stat(cueModPath); os.IsNotExist(err) {
		return &CheckResult{
			Status:  "warning",
			Message: "CUE module not initialized in schema directory",
			Details: fmt.Sprintf("Directory %s does not exist", cueModPath),
			Fix:     "Run 'pudl init' to initialize CUE module",
		}
	}

	corePath := filepath.Join(schemaDir, "pudl", "core")
	if _, err := os.Stat(corePath); os.IsNotExist(err) {
		return &CheckResult{
			Status:  "warning",
			Message: "Bootstrap schemas not found",
			Details: fmt.Sprintf("Directory %s does not exist", corePath),
			Fix:     "Run 'pudl init' to create bootstrap schemas",
		}
	}

	// Check for unexpected top-level directories (exhaustive validation)
	allowedTopLevel := map[string]bool{
		"data":        true,
		"schema":      true,
		"config.yaml": true,
	}
	unexpectedEntries := []string{}
	if entries, err := os.ReadDir(pudlDir); err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if !allowedTopLevel[name] {
				unexpectedEntries = append(unexpectedEntries, name)
			}
		}
	}
	if len(unexpectedEntries) > 0 {
		return &CheckResult{
			Status:  "warning",
			Message: "Unexpected entries in PUDL workspace",
			Details: fmt.Sprintf("Unexpected top-level entries in %s: %v", pudlDir, unexpectedEntries),
			Fix:     "Review and remove unexpected files/directories, or update workspace structure",
		}
	}

	// Validate raw data follows YYYY/MM/DD hierarchy
	if hasRawData {
		invalidPaths := validateRawDataHierarchy(rawDir)
		if len(invalidPaths) > 0 {
			return &CheckResult{
				Status:  "warning",
				Message: "Raw data does not follow expected hierarchy",
				Details: fmt.Sprintf("Expected YYYY/MM/DD structure under %s; found: %v", rawDir, invalidPaths),
				Fix:     "Re-import data using 'pudl import' to use correct directory structure",
			}
		}
	}

	return &CheckResult{
		Status:  "ok",
		Message: "Directory structure is valid",
		Details: fmt.Sprintf("All expected directories and structure verified at %s", pudlDir),
	}
}

// validateRawDataHierarchy checks that immediate children of rawDir follow
// a YYYY/MM/DD date hierarchy. Returns a list of paths that violate the pattern.
func validateRawDataHierarchy(rawDir string) []string {
	var invalid []string

	years, err := os.ReadDir(rawDir)
	if err != nil {
		return nil
	}

	for _, year := range years {
		if !year.IsDir() {
			invalid = append(invalid, year.Name())
			continue
		}
		if !isNumericDir(year.Name(), 4) {
			invalid = append(invalid, year.Name())
			continue
		}

		yearPath := filepath.Join(rawDir, year.Name())
		months, err := os.ReadDir(yearPath)
		if err != nil {
			continue
		}
		for _, month := range months {
			if !month.IsDir() {
				invalid = append(invalid, filepath.Join(year.Name(), month.Name()))
				continue
			}
			if !isNumericDir(month.Name(), 2) {
				invalid = append(invalid, filepath.Join(year.Name(), month.Name()))
				continue
			}

			monthPath := filepath.Join(yearPath, month.Name())
			days, err := os.ReadDir(monthPath)
			if err != nil {
				continue
			}
			for _, day := range days {
				if !day.IsDir() {
					// Files directly in month dir are fine (the actual data)
					continue
				}
				if !isNumericDir(day.Name(), 2) {
					invalid = append(invalid, filepath.Join(year.Name(), month.Name(), day.Name()))
				}
			}
		}
	}

	return invalid
}

// isNumericDir checks if a directory name is a numeric string of the expected length.
func isNumericDir(name string, expectedLen int) bool {
	if len(name) != expectedLen {
		return false
	}
	for _, c := range name {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// CheckOrphanedFiles finds files not in catalog
func CheckOrphanedFiles() *CheckResult {
	pudlDir := config.GetPudlDir()
	dataDir := filepath.Join(pudlDir, "data", "raw")

	// Check if data directory exists
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		return &CheckResult{
			Status:  "ok",
			Message: "No data directory found",
			Details: "No orphaned files to check",
		}
	}

	// Initialize catalog database
	catalogDB, err := database.NewCatalogDB(pudlDir)
	if err != nil {
		return &CheckResult{
			Status:  "warning",
			Message: "Failed to access catalog database",
			Details: err.Error(),
			Fix:     "Check database integrity",
		}
	}
	defer catalogDB.Close()

	// Get all catalog entries
	result, err := catalogDB.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	if err != nil {
		return &CheckResult{
			Status:  "warning",
			Message: "Failed to query catalog",
			Details: err.Error(),
		}
	}

	// Build map of stored paths
	catalogedPaths := make(map[string]bool)
	for _, entry := range result.Entries {
		catalogedPaths[entry.StoredPath] = true
	}

	// Count orphaned files
	orphanedCount := 0
	filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !catalogedPaths[path] {
			orphanedCount++
		}
		return nil
	})

	if orphanedCount > 0 {
		return &CheckResult{
			Status:  "warning",
			Message: fmt.Sprintf("Found %d orphaned files", orphanedCount),
			Details: fmt.Sprintf("Files in %s not referenced in catalog", dataDir),
			Fix:     "Review and delete orphaned files manually",
		}
	}

	return &CheckResult{
		Status:  "ok",
		Message: "No orphaned files found",
		Details: "All data files are properly cataloged",
	}
}

