package operation

import (
	"io"
	"path/filepath"

	"github.com/chazu/pudl/internal/config"
)

// testWorkspace is a global-mode workspace rooted at pudlDir, as a check sees
// it outside a repository: models and rules under pudlDir/schema.
func testWorkspace(pudlDir string) Workspace {
	rules := filepath.Join(pudlDir, "schema", "pudl", "rules")
	return Workspace{
		PudlDir:   pudlDir,
		Name:      "global",
		ModelDirs: []string{filepath.Join(pudlDir, "schema")},
		RulePaths: []string{rules},
		ModelRulePaths: func(modelDir string) []string {
			return []string{rules, filepath.Join(modelDir, "rules")}
		},
		LoadConfig:  func() (*config.Config, error) { return config.LoadFrom(pudlDir) },
		SchemaPaths: func(cfg *config.Config) []string { return []string{cfg.SchemaPath} },
	}
}

// quietOutput is machine output that discards everything.
func quietOutput() Output {
	return Output{Out: io.Discard, Err: io.Discard}
}
