package wiring

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"cuelang.org/go/cue"

	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/systemmodel"
)

type SchemaLookup interface {
	GetSchemaValue(schemaName string) (cue.Value, bool)
}

type Resolver struct {
	Catalog SnapshotCatalog
	Schemas SchemaLookup
}

func (r Resolver) Elaborate(template *systemmodel.ModelTemplate, request ResolveRequest) (*Elaboration, error) {
	if r.Catalog == nil || r.Schemas == nil {
		return nil, fmt.Errorf("plain binding resolver requires catalog and schema lookup")
	}
	evaluatedAt := request.EvaluationTime
	if evaluatedAt.IsZero() {
		evaluatedAt = time.Now().UTC()
	}

	names := make([]string, 0, len(template.Bindings))
	for name := range template.Bindings {
		names = append(names, name)
	}
	sort.Strings(names)

	inputs := make(map[string]json.RawMessage, len(names))
	evidence := make([]BindingEvidence, 0, len(names))
	for _, name := range names {
		value, proof, err := r.resolve(name, template.Bindings[name], request, evaluatedAt)
		if err != nil {
			return nil, err
		}
		inputs[name] = value
		evidence = append(evidence, proof)
	}

	model, err := template.ElaborateJSON(inputs)
	if err != nil {
		return nil, bindingError("*", "type-mismatch", err)
	}
	return &Elaboration{Model: model, Inputs: inputs, Evidence: evidence}, nil
}

func (r Resolver) resolve(input string, binding systemmodel.ValueBinding, request ResolveRequest, evaluatedAt time.Time) (json.RawMessage, BindingEvidence, error) {
	schemaName := schemaname.Normalize(binding.Source.Schema)
	schema, ok := r.Schemas.GetSchemaValue(schemaName)
	if !ok {
		return nil, BindingEvidence{}, bindingError(input, "schema-unavailable", fmt.Errorf("schema %q is not loaded", schemaName))
	}
	segments := pointerSegments(binding.Path)
	selectors := make([]cue.Selector, len(segments))
	for i, segment := range segments {
		selectors[i] = cue.Str(segment)
	}
	field := schema.LookupPath(cue.MakePath(selectors...))
	if !field.Exists() {
		return nil, BindingEvidence{}, bindingError(input, "projection-invalid", fmt.Errorf("schema field %q does not exist", binding.Path))
	}
	class, err := systemmodel.ClassifyBinding(field)
	if err != nil {
		return nil, BindingEvidence{}, bindingError(input, "classification-invalid", err)
	}
	if class != systemmodel.BindingPlain {
		return nil, BindingEvidence{}, bindingError(input, "projection-denied", fmt.Errorf("schema field %q must declare @pudl(binding=plain)", binding.Path))
	}

	selection := "reused"
	producerModel := binding.Source.Model
	var snapshot *Snapshot
	if producer := request.PinnedProducerSnapshots[binding.Source.Model]; producer.SnapshotID != "" {
		selection = "current-run"
		producerModel = producer.Model
		snapshot, err = r.Catalog.ObserveSnapshotByIDForRun(
			producer.SnapshotID, producer.Model, request.Workspace, producer.RunID,
		)
	} else if producer := request.CurrentProducerRuns[binding.Source.Model]; producer.RunID != "" {
		selection = "current-run"
		producerModel = producer.Model
		snapshot, err = r.Catalog.SuccessfulObserveSnapshotForRun(producer.Model, request.Workspace, producer.RunID)
	} else {
		snapshot, err = r.Catalog.LatestSuccessfulObserveSnapshot(binding.Source.Model, request.Workspace)
	}
	if err != nil {
		return nil, BindingEvidence{}, bindingError(input, "snapshot-query-failed", err)
	}
	if snapshot == nil {
		return nil, BindingEvidence{}, bindingError(input, "source-absent", fmt.Errorf("no eligible successful snapshot for model %q in workspace %q", binding.Source.Model, request.Workspace))
	}
	age := evaluatedAt.Sub(snapshot.CreatedAt)
	if age < 0 {
		age = 0
	}
	if request.MaxObservationAge != nil && age > *request.MaxObservationAge {
		return nil, BindingEvidence{}, bindingError(input, "source-too-old", fmt.Errorf("snapshot %q age %s exceeds %s", snapshot.SnapshotID, age, *request.MaxObservationAge))
	}

	identityJSON, err := identity.CanonicalIdentityJSON(binding.Source.Identity)
	if err != nil {
		return nil, BindingEvidence{}, bindingError(input, "identity-invalid", err)
	}
	entries, err := r.Catalog.SnapshotRecordEntries(snapshot.SnapshotID)
	if err != nil {
		return nil, BindingEvidence{}, bindingError(input, "snapshot-read-failed", err)
	}
	var matches []CatalogEntry
	for _, entry := range entries {
		if schemaname.Normalize(entry.Schema) != schemaName || entry.IdentityJSON == nil || *entry.IdentityJSON != identityJSON {
			continue
		}
		matches = append(matches, entry)
	}
	if len(matches) == 0 {
		return nil, BindingEvidence{}, bindingError(input, "source-absent", fmt.Errorf("snapshot %q has no matching %s resource", snapshot.SnapshotID, schemaName))
	}
	if len(matches) > 1 {
		return nil, BindingEvidence{}, bindingError(input, "source-ambiguous", fmt.Errorf("snapshot %q has %d matching resources", snapshot.SnapshotID, len(matches)))
	}

	value, valueType, err := projectScalar(matches[0].StoredPath, segments)
	if err != nil {
		return nil, BindingEvidence{}, bindingError(input, "projection-invalid", err)
	}
	digest := sha256.Sum256(value)
	proof := BindingEvidence{
		Input: input, ProducerModel: producerModel, ProducerRunID: snapshot.RunID,
		SnapshotID: snapshot.SnapshotID, Workspace: snapshot.Workspace, Schema: schemaName,
		Identity: binding.Source.Identity, Path: binding.Path, Selection: selection,
		ObservedAt: snapshot.CreatedAt, EvaluatedAt: evaluatedAt, Age: age,
		MaxAge: request.MaxObservationAge, Value: value, ValueType: valueType,
		ScalarSHA256: hex.EncodeToString(digest[:]), ResolutionCode: "resolved",
	}
	return value, proof, nil
}

func pointerSegments(pointer string) []string {
	raw := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	for i := range raw {
		raw[i] = strings.ReplaceAll(strings.ReplaceAll(raw[i], "~1", "/"), "~0", "~")
	}
	return raw
}

func projectScalar(path string, segments []string) (json.RawMessage, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read catalog resource: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var current any
	if err := decoder.Decode(&current); err != nil {
		return nil, "", fmt.Errorf("decode catalog resource: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, "", fmt.Errorf("catalog resource contains multiple JSON values")
		}
		return nil, "", fmt.Errorf("decode trailing catalog resource data: %w", err)
	}
	for _, segment := range segments {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, "", fmt.Errorf("path traverses a non-object at %q", segment)
		}
		current, ok = object[segment]
		if !ok {
			return nil, "", fmt.Errorf("path field %q is missing", segment)
		}
	}

	valueType := ""
	switch current.(type) {
	case nil:
		valueType = "null"
	case bool:
		valueType = "boolean"
	case string:
		valueType = "string"
	case json.Number:
		valueType = "number"
	default:
		return nil, "", fmt.Errorf("path must resolve to a scalar leaf")
	}
	value, err := json.Marshal(current)
	return value, valueType, err
}
