package schemagen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaExistsError(t *testing.T) {
	t.Run("error message format", func(t *testing.T) {
		err := &SchemaExistsError{
			FilePath:       "/path/to/schema/aws/ec2/instance.cue",
			DefinitionName: "Instance",
			PackagePath:    "ec2",
		}

		assert.Equal(t, "schema file already exists: /path/to/schema/aws/ec2/instance.cue", err.Error())
	})

	t.Run("error is detectable by type assertion", func(t *testing.T) {
		var err error = &SchemaExistsError{
			FilePath:       "/path/to/schema.cue",
			DefinitionName: "Test",
			PackagePath:    "test",
		}

		existsErr, ok := err.(*SchemaExistsError)
		assert.True(t, ok, "should be able to type assert to *SchemaExistsError")
		assert.Equal(t, "Test", existsErr.DefinitionName)
		assert.Equal(t, "test", existsErr.PackagePath)
	})
}

func TestWriteSchema(t *testing.T) {
	t.Run("creates new schema file", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "pudl-schemagen-test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		generator := NewGenerator(tempDir)
		result := &GenerateResult{
			FilePath:       filepath.Join(tempDir, "aws", "ec2", "instance.cue"),
			PackageName:    "ec2",
			DefinitionName: "Instance",
			FieldCount:     5,
			Content:        "package ec2\n\n#Instance: {\n\tid: string\n}\n",
		}

		err = generator.WriteSchema(result, result.Content, false)
		require.NoError(t, err)

		// Verify file was created
		content, err := os.ReadFile(result.FilePath)
		require.NoError(t, err)
		assert.Equal(t, result.Content, string(content))
	})

	t.Run("returns SchemaExistsError when file exists and force is false", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "pudl-schemagen-test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		generator := NewGenerator(tempDir)
		schemaPath := filepath.Join(tempDir, "aws", "ec2", "instance.cue")
		
		// Create the file first
		require.NoError(t, os.MkdirAll(filepath.Dir(schemaPath), 0755))
		require.NoError(t, os.WriteFile(schemaPath, []byte("existing content"), 0644))

		result := &GenerateResult{
			FilePath:       schemaPath,
			PackageName:    "ec2",
			DefinitionName: "Instance",
			FieldCount:     5,
			Content:        "new content",
		}

		err = generator.WriteSchema(result, result.Content, false)
		require.Error(t, err)

		// Verify it's a SchemaExistsError
		existsErr, ok := err.(*SchemaExistsError)
		require.True(t, ok, "expected SchemaExistsError, got %T", err)
		assert.Equal(t, schemaPath, existsErr.FilePath)
		assert.Equal(t, "Instance", existsErr.DefinitionName)
		assert.Equal(t, "ec2", existsErr.PackagePath)

		// Verify file was not overwritten
		content, err := os.ReadFile(schemaPath)
		require.NoError(t, err)
		assert.Equal(t, "existing content", string(content))
	})

	t.Run("overwrites existing file when force is true", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "pudl-schemagen-test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		generator := NewGenerator(tempDir)
		schemaPath := filepath.Join(tempDir, "aws", "ec2", "instance.cue")
		
		// Create the file first
		require.NoError(t, os.MkdirAll(filepath.Dir(schemaPath), 0755))
		require.NoError(t, os.WriteFile(schemaPath, []byte("existing content"), 0644))

		result := &GenerateResult{
			FilePath:       schemaPath,
			PackageName:    "ec2",
			DefinitionName: "Instance",
			FieldCount:     5,
			Content:        "new content",
		}

		err = generator.WriteSchema(result, result.Content, true)
		require.NoError(t, err)

		// Verify file was overwritten
		content, err := os.ReadFile(schemaPath)
		require.NoError(t, err)
		assert.Equal(t, "new content", string(content))
	})

	t.Run("creates parent directories", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "pudl-schemagen-test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		generator := NewGenerator(tempDir)
		result := &GenerateResult{
			FilePath:       filepath.Join(tempDir, "deeply", "nested", "path", "schema.cue"),
			PackageName:    "path",
			DefinitionName: "Schema",
			Content:        "package path\n\n#Schema: {}\n",
		}

		err = generator.WriteSchema(result, result.Content, false)
		require.NoError(t, err)

		// Verify file and directories were created
		assert.FileExists(t, result.FilePath)
	})
}

