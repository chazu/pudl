package doctor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/doctor"
	"github.com/chazu/pudl/internal/repo"
)

func TestScopedChecksUseRepositoryPudlRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := repo.Init(repo.InitOptions{Dir: root}); err != nil {
		t.Fatal(err)
	}
	pudlDir := filepath.Join(root, ".pudl")
	db, err := database.NewCatalogDB(pudlDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	checks := map[string]*doctor.CheckResult{
		"workspace": doctor.CheckWorkspaceStructureAt(pudlDir),
		"database":  doctor.CheckDatabaseIntegrityAt(pudlDir),
		"schema":    doctor.CheckSchemaRepositoryAt(pudlDir),
		"git":       doctor.CheckGitRepositoryAt(pudlDir),
		"layout":    doctor.CheckDirectoryStructureAt(pudlDir),
		"namespace": doctor.CheckPudlNamespaceSchemasAt(pudlDir),
		"identity":  doctor.CheckIdentityFieldConsistencyAt(pudlDir),
		"orphans":   doctor.CheckOrphanedFilesAt(pudlDir),
	}
	for name, result := range checks {
		if result.Status != "ok" {
			t.Errorf("%s check = %s: %s (%s)", name, result.Status, result.Message, result.Details)
		}
	}
}

func TestDirectoryStructureAcceptsPopulatorAuthoringDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := repo.Init(repo.InitOptions{Dir: root}); err != nil {
		t.Fatal(err)
	}
	pudlDir := filepath.Join(root, ".pudl")
	if err := os.MkdirAll(filepath.Join(pudlDir, "populators", "fixture"), 0o755); err != nil {
		t.Fatal(err)
	}

	result := doctor.CheckDirectoryStructureAt(pudlDir)
	if result.Status != "ok" {
		t.Fatalf("populators directory rejected: %s (%s)", result.Message, result.Details)
	}
}
