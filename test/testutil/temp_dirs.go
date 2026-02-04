package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TempDirSetup manages temporary directories for tests
type TempDirSetup struct {
	t       *testing.T
	tempDir string
	cleanup func()
}

// NewTempDirSetup creates a new temporary directory setup for testing
func NewTempDirSetup(t *testing.T) *TempDirSetup {
	tempDir, err := os.MkdirTemp("", "pudl-test-*")
	require.NoError(t, err, "Failed to create temp directory")

	setup := &TempDirSetup{
		t:       t,
		tempDir: tempDir,
		cleanup: func() {
			if err := os.RemoveAll(tempDir); err != nil {
				t.Logf("Warning: failed to cleanup temp directory %s: %v", tempDir, err)
			}
		},
	}

	// Register cleanup to run automatically
	t.Cleanup(setup.cleanup)

	return setup
}

// TempDir returns the path to the temporary directory
func (s *TempDirSetup) TempDir() string {
	return s.tempDir
}

// CreateSubDir creates a subdirectory within the temp directory
func (s *TempDirSetup) CreateSubDir(name string) string {
	subDir := filepath.Join(s.tempDir, name)
	err := os.MkdirAll(subDir, 0755)
	require.NoError(s.t, err, "Failed to create subdirectory %s", name)
	return subDir
}

// WriteFile writes content to a file within the temp directory
func (s *TempDirSetup) WriteFile(filename, content string) string {
	filePath := filepath.Join(s.tempDir, filename)
	
	// Create parent directories if they don't exist
	parentDir := filepath.Dir(filePath)
	if parentDir != s.tempDir {
		err := os.MkdirAll(parentDir, 0755)
		require.NoError(s.t, err, "Failed to create parent directory for %s", filename)
	}
	
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(s.t, err, "Failed to write file %s", filename)
	return filePath
}

// WriteFileInSubDir writes content to a file within a subdirectory
func (s *TempDirSetup) WriteFileInSubDir(subDir, filename, content string) string {
	fullSubDir := filepath.Join(s.tempDir, subDir)
	err := os.MkdirAll(fullSubDir, 0755)
	require.NoError(s.t, err, "Failed to create subdirectory %s", subDir)
	
	filePath := filepath.Join(fullSubDir, filename)
	err = os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(s.t, err, "Failed to write file %s in %s", filename, subDir)
	return filePath
}

// CopyFixture copies a fixture file to the temp directory
func (s *TempDirSetup) CopyFixture(fixturePath, destName string) string {
	// Read fixture file
	content, err := os.ReadFile(fixturePath)
	require.NoError(s.t, err, "Failed to read fixture file %s", fixturePath)
	
	// Write to temp directory
	return s.WriteFile(destName, string(content))
}

// CreatePUDLWorkspace creates a mock PUDL workspace structure
func (s *TempDirSetup) CreatePUDLWorkspace() *PUDLWorkspace {
	workspace := &PUDLWorkspace{
		Root:       s.tempDir,
		SchemaDir:  s.CreateSubDir("schema"),
		DataDir:    s.CreateSubDir("data"),
		ConfigFile: filepath.Join(s.tempDir, "config.yaml"),
	}

	// Create basic config file
	configContent := `schema_path: ` + workspace.SchemaDir + `
data_path: ` + workspace.DataDir + `
`
	s.WriteFile("config.yaml", configContent)

	// Create basic schema structure
	s.CreateSubDir("schema/pudl")
	s.CreateSubDir("schema/examples")
	s.CreateSubDir("schema/cue.mod")

	// Create basic module.cue
	moduleContent := `language: version: "v0.14.0"
module: "pudl.schemas@v0"
source: kind: "self"
`
	s.WriteFileInSubDir("schema/cue.mod", "module.cue", moduleContent)

	// Create core package with catchall schema
	s.CreateSubDir("schema/pudl/core")
	coreContent := `package core

#CatchAll: {
	_pudl: {
		schema_type:      "catchall"
		resource_type:    "unknown"
		cascade_priority: 0
		identity_fields: []
		tracked_fields: []
		compliance_level: "permissive"
	}
	...
}
`
	s.WriteFileInSubDir("schema/pudl/core", "core.cue", coreContent)

	return workspace
}

// PUDLWorkspace represents a mock PUDL workspace for testing
type PUDLWorkspace struct {
	Root       string
	SchemaDir  string
	DataDir    string
	ConfigFile string
}

// SchemaPath returns a path within the schema directory
func (w *PUDLWorkspace) SchemaPath(parts ...string) string {
	allParts := append([]string{w.SchemaDir}, parts...)
	return filepath.Join(allParts...)
}

// DataPath returns a path within the data directory
func (w *PUDLWorkspace) DataPath(parts ...string) string {
	allParts := append([]string{w.DataDir}, parts...)
	return filepath.Join(allParts...)
}

// Cleanup manually cleans up the workspace (usually not needed due to t.Cleanup)
func (s *TempDirSetup) Cleanup() {
	if s.cleanup != nil {
		s.cleanup()
		s.cleanup = nil
	}
}
