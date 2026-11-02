package observe

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/inference"
)

// A collector without a schema routes records by _schema, else to the generic
// observe schema; a record that claims a type by @pudl(match) is inferred
// instead, and rejected when it fails the type.
func TestObserveRouteInfersClaimedRecords(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cue.mod"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"), []byte("module: \"test.schemas\"\nlanguage: version: \"v0.14.0\"\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas", "fw.cue"), []byte(`package schemas

#Firewall: {
	_pudl: {
		schema_type: "base"
		resource_type: "x.firewall"
		identity_fields: ["name"]
	}
	kind: "compute#firewall" @pudl(match)
	project: =~"^[a-z][a-z0-9-]{4,28}[a-z0-9]$"
	name: string
	...
}
`), 0o644))
	inferrer, err := inference.NewSchemaInferrer(dir)
	require.NoError(t, err)
	route, err := newObserveRoute(ObserveIngest{Inferrer: inferrer})
	require.NoError(t, err)
	graph := inferrer.GetInheritanceGraph()

	got, err := route.resolve(map[string]any{"kind": "compute#firewall", "project": "my-project", "name": "fw"}, graph, inferrer)
	require.NoError(t, err)
	assert.Equal(t, "schemas.#Firewall", got)

	_, err = route.resolve(map[string]any{"kind": "compute#firewall", "name": "fw"}, graph, inferrer)
	var claimErr *inference.ClaimError
	assert.True(t, errors.As(err, &claimErr), "want ClaimError, got %v", err)

	got, err = route.resolve(map[string]any{"name": "other"}, graph, inferrer)
	require.NoError(t, err)
	assert.Equal(t, genericObserveSchema, got)
}
