// Package operation executes `pudl check`: it resolves the named models,
// orders them producers first, runs each through populate, ingest, inventory
// drift and checks, concludes each model's verdict, and records one check
// report. The workspace it works in, its output streams and its progress
// callback are injected; it reads no command-line state.
package operation

import (
	"fmt"
	"io"
	"strings"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/ingestprep"
	"github.com/chazu/pudl/internal/inventory"
	"github.com/chazu/pudl/internal/projection"
	"github.com/chazu/pudl/internal/systemmodel"
)

// Options is the complete input of one check.
type Options struct {
	// Models are the registered models to check, by name. An ad-hoc check
	// (PopulateSpec) names none.
	Models []string
	// PopulateSpec is `command:<cmdline>`: check an unregistered observer.
	PopulateSpec string
	// FromCatalog replays already-ingested records instead of observing;
	// CatalogScope names them (a snapshot ID, or their origin).
	FromCatalog  bool
	CatalogScope string
	// CheckUpstream warns when a transitive upstream model is drifted or
	// failed.
	CheckUpstream bool
}

// ValidateOptions enforces that a catalog replay names the records it
// replays. There is no way to infer which already-ingested records belong to a
// model: records carry whatever target their observer reported, so an
// unscoped replay would set-diff `desired` against every observation in the
// catalog — every model, every host, all time — and could report clean off
// another model's records.
func ValidateOptions(o Options) error {
	if o.FromCatalog && strings.TrimSpace(o.CatalogScope) == "" {
		return fmt.Errorf("--from-catalog requires --catalog-scope (an observe snapshot ID, or the origin the records were ingested under)")
	}
	if !o.FromCatalog && strings.TrimSpace(o.CatalogScope) != "" {
		return fmt.Errorf("--catalog-scope requires --from-catalog")
	}
	return nil
}

// Workspace is where a check runs: its persistence root, its search paths and
// how to load its configuration. Configuration loads on demand, so a check
// that never needs schemas never loads them.
type Workspace struct {
	// PudlDir is the persistence root: catalog, raw data and reports.
	PudlDir string
	// Name is recorded on the snapshots a check takes.
	Name string
	// ModelDirs is the model search order (project first).
	ModelDirs []string
	// RulePaths is the shipped rule search order (the upstream advisory).
	RulePaths []string
	// ModelRulePaths is the rule search order for one model's checks.
	ModelRulePaths func(modelDir string) []string
	// LoadConfig loads the validated configuration; SchemaPaths is the schema
	// search order under it.
	LoadConfig  func() (*config.Config, error)
	SchemaPaths func(*config.Config) []string
	// IngestLimits bounds populate output.
	IngestLimits ingestprep.Limits
	// TrackTempDir registers a populate spool directory so an interrupt can
	// remove it, and returns its release. Nil removes it on return only.
	TrackTempDir func(dir string) (release func())
}

// Projections resolves `_pudl.facts` specs over the workspace's schemas.
func (w Workspace) Projections() (*projection.Registry, error) {
	cfg, err := w.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	inferrer, err := inference.Shared(w.SchemaPaths(cfg)...)
	if err != nil {
		return nil, fmt.Errorf("init schema inferrer: %w", err)
	}
	return projection.NewRegistry(inferrer, nil), nil
}

// CheckModelSchemas applies systemmodel.CheckSchemaContracts against the
// workspace's schemas. A model that names no schema does not load them.
func (w Workspace) CheckModelSchemas(m *systemmodel.SystemModel) error {
	if m.Populate.Schema == "" && len(m.Desired) == 0 {
		return nil
	}
	cfg, err := w.LoadConfig()
	if err != nil {
		return err
	}
	schemas, err := inference.Shared(w.SchemaPaths(cfg)...)
	if err != nil {
		return fmt.Errorf("load schemas for model %q: %w", m.Name, err)
	}
	return systemmodel.CheckSchemaContracts(m, schemas)
}

// InventoryPolicy loads how the workspace's schemas identify inventory
// records.
func (w Workspace) InventoryPolicy() (inventory.Policy, error) {
	cfg, err := w.LoadConfig()
	if err != nil {
		return inventory.Policy{}, err
	}
	return inventory.LoadPolicy(w.SchemaPaths(cfg))
}

// Output is where a check reports as it runs.
type Output struct {
	// Out receives results; Err receives diagnostics and each populate
	// command's stderr.
	Out, Err io.Writer
	// Live is human output: each model's plan and report print as it runs,
	// with best-effort warnings. Machine output writes only the final report.
	Live bool
	// Owner, when set, limits the findings live output prints to one
	// resource owner (checkreport.ModelReport.ForOwner). The recorded report
	// is complete.
	Owner string
	// Progress receives phase events (phase, subject, state). Nil drops them.
	Progress func(phase, subject, state string)
	// Concluded receives the check report once it is final and saved, to
	// render it. An error it returns fails the check.
	Concluded func(*acute.RunSetReport) error
}

func (o Output) progress(phase, subject, state string) {
	if o.Progress != nil {
		o.Progress(phase, subject, state)
	}
}

// warnf writes a best-effort warning when output is live.
func (o Output) warnf(format string, args ...any) {
	if o.Live {
		fmt.Fprintf(o.Err, format, args...)
	}
}
