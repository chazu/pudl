package cmd

import (
	"bytes"
	"context"
	"fmt"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/observe"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/validator"
)

// populateTargetName is the catalog target a model's populate phase records
// its observation under.
func populateTargetName(modelName string) string {
	return fmt.Sprintf("//models/%s:populate", modelName)
}

// runPopulate executes the populate phase: the model's #CommandObserve runs,
// ingested as one observation snapshot.
func runPopulate(ctx context.Context, cat *runCatalog, m *systemmodel.SystemModel, modelDir, runID, snapshotID string) (*PopulateReport, error) {
	if m.Populate.Kind() == systemmodel.KindNone {
		return nil, fmt.Errorf("model %q has no populate arm (checks-only)", m.Name)
	}
	return runCommandPopulate(cat, ctx, m, modelDir, runID, snapshotID)
}

// ingestPopulateOutput feeds wrapped observe results into the catalog as
// observe entries and records the snapshot's provenance: which run took it,
// for which model, in which workspace, and how it was produced.
//
// The snapshot ID is passed in rather than generated here, so the run owns the
// identifier for an observation it initiated and can name the snapshot a failed
// ingest would have produced.
func ingestPopulateOutput(cat *runCatalog, observeJSON []byte, in populateIngest) (int, string, error) {
	db, err := cat.required()
	if err != nil {
		return 0, "", err
	}

	cfg, err := loadEffectiveConfig()
	if err != nil {
		return 0, "", fmt.Errorf("load config: %w", err)
	}
	inferrer, err := inference.Shared(effectiveSchemaPaths(cfg)...)
	if err != nil {
		return 0, "", fmt.Errorf("init schema inferrer: %w", err)
	}
	var chain *validator.ChainValidator
	if in.manualSchema != "" {
		if chain, err = validator.NewChainValidator(effectiveSchemaPaths(cfg)...); err != nil {
			return 0, "", fmt.Errorf("load schemas for %s: %w", in.manualSchema, err)
		}
		if !chain.HasSchema(in.manualSchema) {
			return 0, "", fmt.Errorf("populate schema %q is not loaded (see: pudl schema list)", in.manualSchema)
		}
	}
	observation := systemmodel.Observation{}
	if in.observation != nil {
		observation = *in.observation
	}
	result, err := observe.IngestObserve(db, observe.ObserveIngest{
		Scope: observation.Scope, Complete: observation.Complete, Schemas: observation.Schemas,
		Reader:              bytes.NewReader(observeJSON),
		Context:             in.ctx,
		DataDir:             cfg.DataPath,
		Graph:               inferrer.GetInheritanceGraph(),
		Inferrer:            inferrer,
		SnapshotID:          in.snapshotID,
		RunID:               in.runID,
		Model:               in.model,
		Workspace:           effectiveWorkspaceName(),
		Origin:              "pudl-run",
		Source:              in.source,
		ManualSchema:        in.manualSchema,
		AllowSchemaFallback: in.allowSchemaFallback,
		Chain:               chain,
	})
	for _, w := range result.FactWarnings {
		fmt.Fprintf(errw(), "⚠️  %s\n", w)
	}
	if in.summary != nil {
		in.summary.SchemaMismatches = result.SchemaMismatches
	}
	return result.Records, result.SnapshotID, err
}

// populateIngest is the provenance a populate phase attaches to its snapshot.
type populateIngest struct {
	summary             *PopulateReport
	allowSchemaFallback bool
	observation         *systemmodel.Observation
	ctx                 context.Context
	snapshotID          string
	runID               string
	model               string
	source              string
	// manualSchema routes every record like `import --schema` (command arm).
	manualSchema string
}
