package operation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/observe"
	"github.com/chazu/pudl/internal/populate"
	"github.com/chazu/pudl/internal/secretscan"
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
func (r *modelRun) runPopulate() (*checkreport.Populate, error) {
	m := r.model
	if m.Populate.Kind() == systemmodel.KindNone {
		return nil, fmt.Errorf("model %q has no populate arm (checks-only)", m.Name)
	}
	p := m.Populate
	records, err := populate.Run(r.ctx, p, populate.Request{
		ModelDir: r.modelDir, Limits: r.ws.IngestLimits, Stderr: r.out.Err, TrackTempDir: r.ws.TrackTempDir,
	})
	if err != nil {
		return nil, err
	}

	target := populateTargetName(m.Name)
	summary := &checkreport.Populate{Target: target, RequestedSchema: p.Schema}
	if p.Schema != "" {
		summary.SchemaPolicy = "strict"
		if p.AllowSchemaFallback {
			summary.SchemaPolicy = "permissive"
		}
	}
	wrapped, err := json.Marshal([]observe.ObserveResult{{Target: target, Current: map[string]any{"records": records}}})
	if err != nil {
		return nil, fmt.Errorf("marshal observe results: %w", err)
	}
	count, snapshotID, err := r.ingest(wrapped, populateIngest{observation: m.Observation,
		summary:             summary,
		ctx:                 r.ctx,
		snapshotID:          r.session.SnapshotID,
		model:               m.Name,
		source:              database.SnapshotSourceCommand,
		manualSchema:        p.Schema,
		allowSchemaFallback: p.AllowSchemaFallback,
	})
	if err != nil {
		return nil, err
	}
	summary.Records, summary.SnapshotID = count, snapshotID
	return summary, nil
}

// recordModelInstance upserts the run's #SystemModel instance into the catalog
// (schema pudl/systemmodel.#SystemModel, identity = name) so every model that
// has been checked is inventoriable via `pudl list`/`query`. It reuses the
// observe ingester — the instance lands as an ordinary catalog record.
func (r *modelRun) recordModelInstance() error {
	m := r.model
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	var rec map[string]any
	if err := json.Unmarshal(b, &rec); err != nil {
		return err
	}
	rec["name"] = m.Name
	rec["_schema"] = "systemmodel.system_model" // -> pudl/systemmodel.#SystemModel

	results := []observe.ObserveResult{{
		Target:  systemmodel.InstanceDefinition(m.Name),
		Current: map[string]any{"records": []any{rec}},
	}}
	wrapped, err := json.Marshal(results)
	if err != nil {
		return err
	}
	// Recorded under its own source: this is a registration, not an observation
	// of the live system, and it must not be picked up as the model's current
	// snapshot in place of what populate actually observed.
	_, _, err = r.ingest(wrapped, populateIngest{
		model:  m.Name,
		source: database.SnapshotSourceModelInstance,
	})
	return err
}

// ingest feeds wrapped observe results into the catalog as observe entries
// and records the snapshot's provenance: which run took it, for which model,
// in which workspace, and how it was produced.
//
// The snapshot ID is passed in rather than generated here, so the run owns the
// identifier for an observation it initiated and can name the snapshot a failed
// ingest would have produced.
func (r *modelRun) ingest(observeJSON []byte, in populateIngest) (int, string, error) {
	db, err := r.cat.required()
	if err != nil {
		return 0, "", err
	}

	cfg, err := r.ws.LoadConfig()
	if err != nil {
		return 0, "", fmt.Errorf("load config: %w", err)
	}
	inferrer, err := inference.Shared(r.ws.SchemaPaths(cfg)...)
	if err != nil {
		return 0, "", fmt.Errorf("init schema inferrer: %w", err)
	}
	var chain *validator.ChainValidator
	if in.manualSchema != "" {
		if chain, err = validator.NewChainValidator(r.ws.SchemaPaths(cfg)...); err != nil {
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
		RunID:               r.session.RunID,
		Model:               in.model,
		Workspace:           r.ws.Name,
		Origin:              "pudl-run",
		Source:              in.source,
		ManualSchema:        in.manualSchema,
		AllowSchemaFallback: in.allowSchemaFallback,
		Chain:               chain,
	})
	for _, w := range result.FactWarnings {
		fmt.Fprintf(r.out.Err, "⚠️  %s\n", w)
	}
	for _, note := range secretscan.Notes(result.SecretWarnings, result.SecretWarningsTotal) {
		fmt.Fprintf(r.out.Err, "%s\n", note)
	}
	if in.summary != nil {
		in.summary.SchemaMismatches = result.SchemaMismatches
		in.summary.SecretWarnings, in.summary.SecretWarningsTotal = result.SecretWarnings, result.SecretWarningsTotal
	}
	return result.Records, result.SnapshotID, err
}

// populateIngest is the provenance a populate phase attaches to its snapshot.
type populateIngest struct {
	summary             *checkreport.Populate
	allowSchemaFallback bool
	observation         *systemmodel.Observation
	ctx                 context.Context
	snapshotID          string
	model               string
	source              string
	// manualSchema routes every record like `import --schema` (command arm).
	manualSchema string
}
