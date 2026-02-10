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

		existingContent := "package ec2\n\n#Instance: {\n\told: string\n}\n"
		newContent := "package ec2\n\n#Instance: {\n\tnew: string\n}\n"

		// Create the file first
		require.NoError(t, os.MkdirAll(filepath.Dir(schemaPath), 0755))
		require.NoError(t, os.WriteFile(schemaPath, []byte(existingContent), 0644))

		result := &GenerateResult{
			FilePath:       schemaPath,
			PackageName:    "ec2",
			DefinitionName: "Instance",
			FieldCount:     5,
			Content:        newContent,
		}

		err = generator.WriteSchema(result, result.Content, false)
		require.Error(t, err)

		// Verify it's a SchemaExistsError
		existsErr, ok := err.(*SchemaExistsError)
		require.True(t, ok, "expected SchemaExistsError, got %T: %v", err, err)
		assert.Equal(t, schemaPath, existsErr.FilePath)
		assert.Equal(t, "Instance", existsErr.DefinitionName)
		assert.Equal(t, "ec2", existsErr.PackagePath)

		// Verify file was not overwritten
		content, err := os.ReadFile(schemaPath)
		require.NoError(t, err)
		assert.Equal(t, existingContent, string(content))
	})

	t.Run("overwrites existing file when force is true", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "pudl-schemagen-test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		generator := NewGenerator(tempDir)
		schemaPath := filepath.Join(tempDir, "aws", "ec2", "instance.cue")

		existingContent := "package ec2\n\n#Instance: {\n\told: string\n}\n"
		newContent := "package ec2\n\n#Instance: {\n\tnew: string\n}\n"

		// Create the file first
		require.NoError(t, os.MkdirAll(filepath.Dir(schemaPath), 0755))
		require.NoError(t, os.WriteFile(schemaPath, []byte(existingContent), 0644))

		result := &GenerateResult{
			FilePath:       schemaPath,
			PackageName:    "ec2",
			DefinitionName: "Instance",
			FieldCount:     5,
			Content:        newContent,
		}

		err = generator.WriteSchema(result, result.Content, true)
		require.NoError(t, err)

		// Verify file was overwritten
		content, err := os.ReadFile(schemaPath)
		require.NoError(t, err)
		assert.Equal(t, newContent, string(content))
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

	t.Run("rejects invalid CUE content", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "pudl-schemagen-test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		generator := NewGenerator(tempDir)
		result := &GenerateResult{
			FilePath:       filepath.Join(tempDir, "invalid", "schema.cue"),
			PackageName:    "invalid",
			DefinitionName: "Schema",
			Content:        "package invalid\n\n#Schema: { invalid-field: string }\n", // unquoted hyphen
		}

		err = generator.WriteSchema(result, result.Content, false)
		require.Error(t, err)

		// Verify it's a SchemaValidationError
		validationErr, ok := err.(*SchemaValidationError)
		require.True(t, ok, "expected SchemaValidationError, got %T: %v", err, err)
		assert.NotEmpty(t, validationErr.Errors)

		// Verify file was NOT created
		assert.NoFileExists(t, result.FilePath)
	})
}

func TestNeedsQuoting(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid CUE identifiers - no quoting needed
		{"simple lowercase", "name", false},
		{"simple uppercase", "Name", false},
		{"with underscore", "field_name", false},
		{"with dollar", "field$name", false},
		{"starts with underscore", "_private", false},
		{"starts with dollar", "$special", false},
		{"mixed case with digits", "Field123", false},
		{"underscore and digits", "field_123", false},

		// Invalid CUE identifiers - quoting needed
		{"empty string", "", true},
		{"contains hyphen", "alert-group", true},
		{"contains dot", "prometheus.io", true},
		{"contains slash", "kubernetes.io/revision", true},
		{"starts with digit", "123field", true},
		{"contains space", "field name", true},
		{"contains colon", "field:name", true},
		{"contains at", "field@name", true},
		{"kubernetes annotation", "deployment.kubernetes.io/revision", true},
		{"prometheus annotation", "prometheus.io/port", true},
		{"kubectl annotation", "kubectl.kubernetes.io/restartedAt", true},
		{"label with hyphen", "api-telemetry-extension", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := needsQuoting(tt.input)
			assert.Equal(t, tt.expected, result, "needsQuoting(%q) = %v, want %v", tt.input, result, tt.expected)
		})
	}
}

func TestFormatFieldName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// No quoting needed
		{"simple name", "name", "name"},
		{"with underscore", "field_name", "field_name"},
		{"starts with underscore", "_private", "_private"},

		// Quoting needed
		{"contains hyphen", "alert-group", `"alert-group"`},
		{"kubernetes annotation", "deployment.kubernetes.io/revision", `"deployment.kubernetes.io/revision"`},
		{"prometheus annotation", "prometheus.io/port", `"prometheus.io/port"`},
		{"empty string", "", `""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatFieldName(tt.input)
			assert.Equal(t, tt.expected, result, "formatFieldName(%q) = %q, want %q", tt.input, result, tt.expected)
		})
	}
}

func TestValidateCUEContent(t *testing.T) {
	t.Run("valid CUE content", func(t *testing.T) {
		content := `package test

#Schema: {
	name: string
	count: int
	"special-field": string
}
`
		err := ValidateCUEContent(content)
		assert.NoError(t, err)
	})

	t.Run("invalid CUE syntax - unquoted hyphen", func(t *testing.T) {
		content := `package test

#Schema: {
	invalid-field: string
}
`
		err := ValidateCUEContent(content)
		require.Error(t, err)

		validationErr, ok := err.(*SchemaValidationError)
		require.True(t, ok, "expected SchemaValidationError, got %T", err)
		assert.NotEmpty(t, validationErr.Errors)
		assert.Contains(t, validationErr.Error(), "CUE syntax error")
	})

	t.Run("invalid CUE syntax - missing closing brace", func(t *testing.T) {
		content := `package test

#Schema: {
	name: string
`
		err := ValidateCUEContent(content)
		require.Error(t, err)

		validationErr, ok := err.(*SchemaValidationError)
		require.True(t, ok, "expected SchemaValidationError, got %T", err)
		assert.NotEmpty(t, validationErr.Errors)
	})

	t.Run("valid CUE with quoted special fields", func(t *testing.T) {
		// This is what our generator should produce for K8s data
		content := `package apps

#Deployment: {
	metadata: {
		annotations: {
			"alert-group": string
			"deployment.kubernetes.io/revision": string
			"prometheus.io/port": string
		}
	}
}
`
		err := ValidateCUEContent(content)
		assert.NoError(t, err)
	})
}

func TestSchemaValidationError(t *testing.T) {
	t.Run("error message format", func(t *testing.T) {
		err := &SchemaValidationError{
			Content: "invalid content",
			Errors:  []string{"error 1", "error 2"},
		}

		assert.Contains(t, err.Error(), "error 1")
		assert.Contains(t, err.Error(), "error 2")
		assert.Contains(t, err.Error(), "invalid CUE syntax")
	})

	t.Run("error is detectable by type assertion", func(t *testing.T) {
		var err error = &SchemaValidationError{
			Content: "test",
			Errors:  []string{"test error"},
		}

		validationErr, ok := err.(*SchemaValidationError)
		assert.True(t, ok, "should be able to type assert to *SchemaValidationError")
		assert.Equal(t, "test", validationErr.Content)
		assert.Len(t, validationErr.Errors, 1)
	})
}

