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

