package init

import (
	"fmt"
	"os"

	"github.com/chazu/pudl/internal/artifacts"
	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/importer"
)

// repair brings an initialized global workspace's pudl-owned files up to this
// pudl, as repeating a local `pudl init` does, and keeps its configuration:
// the schema and data directories exist, the schema directory is a CUE module,
// and the built-in schemas, rules and #SystemModel are this pudl's.
func repair(pudlDir string, opts InitOptions) error {
	out := opts.output()
	cfg, err := config.LoadFrom(pudlDir)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	if err := os.MkdirAll(cfg.SchemaPath, 0755); err != nil {
		return fmt.Errorf("failed to create schema directory: %w", err)
	}
	if err := artifacts.MkdirPrivate(cfg.DataPath); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}
	if err := initCUEModule(cfg.SchemaPath, opts.Verbose, out); err != nil {
		return fmt.Errorf("failed to initialize CUE module: %w", err)
	}
	if err := importer.CopyBootstrapSchemas(cfg.SchemaPath); err != nil {
		return fmt.Errorf("failed to copy bootstrap schemas: %w", err)
	}
	if opts.Verbose {
		fmt.Fprintf(out, "Repaired PUDL workspace at %s (configuration kept)\n", pudlDir)
		fmt.Fprintln(out, "✅ Built-in schemas, rules, and #SystemModel refreshed")
	}
	return nil
}
