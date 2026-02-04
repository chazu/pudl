package ui

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaNewOutput(t *testing.T) {
	t.Run("JSON serialization for single schema", func(t *testing.T) {
		output := SchemaNewOutput{
			Success:                true,
			FilePath:               "/path/to/schema/aws/ec2/instance.cue",
			PackageName:            "ec2",
			DefinitionName:         "Instance",
			FieldCount:             11,
			InferredIdentityFields: []string{"ImageId", "InstanceId"},
			IsCollection:           false,
		}

		data, err := json.Marshal(output)
		require.NoError(t, err)

		// Verify it can be unmarshaled back
		var parsed SchemaNewOutput
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, output.Success, parsed.Success)
		assert.Equal(t, output.FilePath, parsed.FilePath)
		assert.Equal(t, output.PackageName, parsed.PackageName)
		assert.Equal(t, output.DefinitionName, parsed.DefinitionName)
		assert.Equal(t, output.FieldCount, parsed.FieldCount)
		assert.Equal(t, output.InferredIdentityFields, parsed.InferredIdentityFields)
		assert.Equal(t, output.IsCollection, parsed.IsCollection)
		assert.Nil(t, parsed.NewItemSchemas)
		assert.Nil(t, parsed.ExistingSchemaRefs)
	})

	t.Run("JSON serialization for collection schema", func(t *testing.T) {
		output := SchemaNewOutput{
			Success:        true,
			FilePath:       "/path/to/schema/aws/ec2/instancecollection.cue",
			PackageName:    "ec2",
			DefinitionName: "InstanceCollection",
			FieldCount:     2,
			IsCollection:   true,
			NewItemSchemas: []SchemaNewItemOutput{
				{
					FilePath:       "/path/to/schema/aws/ec2/instance.cue",
					DefinitionName: "Instance",
					FieldCount:     12,
				},
			},
			ExistingSchemaRefs: []string{"pudl.schemas/aws/s3@v0:#Bucket"},
		}

		data, err := json.Marshal(output)
		require.NoError(t, err)

		// Verify it can be unmarshaled back
		var parsed SchemaNewOutput
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.True(t, parsed.IsCollection)
		assert.Len(t, parsed.NewItemSchemas, 1)
		assert.Equal(t, "Instance", parsed.NewItemSchemas[0].DefinitionName)
		assert.Equal(t, 12, parsed.NewItemSchemas[0].FieldCount)
		assert.Len(t, parsed.ExistingSchemaRefs, 1)
		assert.Equal(t, "pudl.schemas/aws/s3@v0:#Bucket", parsed.ExistingSchemaRefs[0])
	})

	t.Run("omitempty fields are excluded when empty", func(t *testing.T) {
		output := SchemaNewOutput{
			Success:        true,
			FilePath:       "/path/to/schema.cue",
			PackageName:    "test",
			DefinitionName: "Test",
			FieldCount:     5,
			IsCollection:   false,
			// Leave optional fields empty
		}

		data, err := json.Marshal(output)
		require.NoError(t, err)

		// Verify omitempty fields are not present
		var raw map[string]interface{}
		err = json.Unmarshal(data, &raw)
		require.NoError(t, err)

		_, hasIdentity := raw["inferred_identity_fields"]
		_, hasItemSchemas := raw["new_item_schemas"]
		_, hasExistingRefs := raw["existing_schema_refs"]

		assert.False(t, hasIdentity, "inferred_identity_fields should be omitted when empty")
		assert.False(t, hasItemSchemas, "new_item_schemas should be omitted when empty")
		assert.False(t, hasExistingRefs, "existing_schema_refs should be omitted when empty")
	})
}

func TestOutputWriter(t *testing.T) {
	t.Run("WriteJSON outputs formatted JSON", func(t *testing.T) {
		var buf bytes.Buffer
		writer := &OutputWriter{
			Format: OutputFormatJSON,
			Writer: &buf,
			Pretty: true,
		}

		data := SchemaNewOutput{
			Success:        true,
			FilePath:       "/path/to/schema.cue",
			PackageName:    "test",
			DefinitionName: "Test",
			FieldCount:     5,
		}

		err := writer.WriteJSON(data)
		require.NoError(t, err)

		// Verify output is valid JSON
		var parsed SchemaNewOutput
		err = json.Unmarshal(buf.Bytes(), &parsed)
		require.NoError(t, err)
		assert.Equal(t, data.Success, parsed.Success)
		assert.Equal(t, data.DefinitionName, parsed.DefinitionName)

		// Verify it's pretty-printed (contains newlines)
		assert.Contains(t, buf.String(), "\n")
	})
}

