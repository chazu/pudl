package validator

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCUEErrorParserConflictingValues(t *testing.T) {
	parser := NewCUEErrorParser()

	err := errors.New("spec.replicas: conflicting values 3 and 5")
	results := parser.Parse(err)

	assert.Len(t, results, 1)
	assert.Equal(t, "spec.replicas", results[0].Path)
	assert.Equal(t, "3", results[0].Expected)
	assert.Equal(t, "5", results[0].Got)
	assert.Equal(t, "conflicting values", results[0].Constraint)
	assert.NotEmpty(t, results[0].Suggestion)
}

func TestCUEErrorParserIncompleteValue(t *testing.T) {
	parser := NewCUEErrorParser()

	err := errors.New("metadata.labels: incomplete value string")
	results := parser.Parse(err)

	assert.Len(t, results, 1)
	assert.Equal(t, "metadata.labels", results[0].Path)
	assert.Equal(t, "incomplete value", results[0].Constraint)
	assert.Equal(t, "string", results[0].Got)
}

func TestCUEErrorParserValueNotAllowed(t *testing.T) {
	parser := NewCUEErrorParser()

	err := errors.New(`status.phase: value "Unknown" not allowed (enum: "Pending"|"Running"|"Succeeded"|"Failed")`)
	results := parser.Parse(err)

	assert.Len(t, results, 1)
	assert.Equal(t, "status.phase", results[0].Path)
	assert.Equal(t, `"Unknown"`, results[0].Got)
	assert.Equal(t, "value not allowed", results[0].Constraint)
}

func TestCUEErrorParserTypeMismatch(t *testing.T) {
	parser := NewCUEErrorParser()

	err := errors.New(`spec.port: 8080 (type int) does not match "8080" (type string)`)
	results := parser.Parse(err)

	assert.Len(t, results, 1)
	assert.Equal(t, "spec.port", results[0].Path)
	assert.Equal(t, "type mismatch", results[0].Constraint)
}

func TestCUEErrorParserMissingField(t *testing.T) {
	parser := NewCUEErrorParser()

	err := errors.New("metadata: missing required field name")
	results := parser.Parse(err)

	assert.Len(t, results, 1)
	assert.Equal(t, "metadata", results[0].Path)
	assert.Equal(t, "name", results[0].Expected)
	assert.Equal(t, "missing required field", results[0].Constraint)
}

func TestCUEErrorParserMultipleErrors(t *testing.T) {
	parser := NewCUEErrorParser()

	err := errors.New(`spec.replicas: conflicting values 3 and 5
metadata.labels: incomplete value string`)
	results := parser.Parse(err)

	assert.Len(t, results, 2)
	assert.Equal(t, "spec.replicas", results[0].Path)
	assert.Equal(t, "metadata.labels", results[1].Path)
}

func TestCUEErrorParserNilError(t *testing.T) {
	parser := NewCUEErrorParser()

	results := parser.Parse(nil)

	assert.Nil(t, results)
}

func TestCUEErrorParserFormatError(t *testing.T) {
	parser := NewCUEErrorParser()

	pe := ParsedError{
		Path:       "spec.replicas",
		Expected:   "3",
		Got:        "5",
		Constraint: "conflicting values",
		Suggestion: "Ensure field has a single consistent value",
	}

	formatted := parser.FormatError(pe)

	assert.Contains(t, formatted, "spec.replicas")
	assert.Contains(t, formatted, "conflicting values")
	assert.Contains(t, formatted, "Ensure field has a single consistent value")
}

func TestCUEErrorParserGenericError(t *testing.T) {
	parser := NewCUEErrorParser()

	err := errors.New("some.field: some error message")
	results := parser.Parse(err)

	assert.Len(t, results, 1)
	assert.Equal(t, "some.field", results[0].Path)
	assert.NotEmpty(t, results[0].Suggestion)
}

