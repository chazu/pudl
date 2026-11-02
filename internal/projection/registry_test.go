package projection

import (
	"testing"

	"github.com/chazu/pudl/internal/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A policy inherits its type's facts along base_schema but is never assigned
// to a record, so it projects nothing and is never reported as disabled.
func TestRegistrySkipsPolicies(t *testing.T) {
	src := hostSource(`{"host": {"args": {"name": "name"}}}`)
	src["x.#HostBestPractice"] = validator.SchemaMetadata{SchemaType: validator.SchemaTypePolicy, BaseSchema: "x.#Host"}
	reg := NewRegistry(src, nil)

	assert.Nil(t, reg.For("x.#HostBestPractice"))
	assert.Equal(t, []string{"x.#Host"}, reg.Schemas())
	spec := reg.For("x.#Host")
	require.NotNil(t, spec)
	assert.NoError(t, spec.Err)
}
