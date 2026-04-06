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

	// Create core package with Item schema (universal fallback)
	s.CreateSubDir("schema/pudl/core")
	coreContent := `package core

#Item: {
	_pudl: {
		schema_type:      "catchall"
		resource_type:    "unknown"
		identity_fields: []
		tracked_fields: []
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

// AddBootstrapSchemas adds specific schema packages to the test workspace.
// Unlike a real pudl init (which copies all bootstrap schemas), this only writes
// the schemas needed for the test, avoiding broken CUE dependencies from other
// bootstrap packages that reference modules not available in test environments.
//
// It also writes empty stubs for all checked bootstrap paths so that the
// importer's ensureBasicSchemas() doesn't trigger a full bootstrap copy.
func (s *TempDirSetup) AddBootstrapSchemas(workspace *PUDLWorkspace) {
	// Write empty stubs for all paths checked by ensureBasicSchemas, so it
	// won't trigger a full bootstrap copy that drags in broken dependencies.
	stubs := []struct{ dir, file, pkg string }{
		{"schema/pudl/catalog", "catalog.cue", "catalog"},
		{"schema/pudl/fs", "fs.cue", "fs"},
		{"schema/pudl/version", "version.cue", "version"},
		{"schema/pudl/infra", "infra.cue", "infra"},
		{"schema/pudl/component", "component.cue", "component"},
		{"schema/pudl/artifact", "artifact.cue", "artifact"},
		{"schema/pudl/registry", "registry.cue", "registry"},
		{"schema/pudl/aws", "aws.cue", "aws"},
		{"schema/pudl/mu", "mu.cue", "mu"},
		{"schema/pudl/brick", "brick.cue", "brick"},
	}
	for _, stub := range stubs {
		s.CreateSubDir(stub.dir)
		s.WriteFileInSubDir(stub.dir, stub.file, "package "+stub.pkg+"\n")
	}

	// Add linux schema package
	s.CreateSubDir("schema/pudl/linux")
	linuxContent := `package linux

#Host: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "linux.host"
		identity_fields: ["hostname"]
		tracked_fields: ["os", "kernel", "arch"]
	}
	hostname:        string
	os: {
		id:      string
		version: string
		name:    string
	}
	kernel:          string
	arch:            string
	uptime_seconds:  int
	...
}

#Package: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "linux.package"
		identity_fields: ["host", "name"]
		tracked_fields: ["version", "status"]
	}
	host:    string
	name:    string
	version: string
	status:  string
	...
}

#Service: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "linux.service"
		identity_fields: ["host", "unit"]
		tracked_fields: ["active", "sub"]
	}
	host:   string
	unit:   string
	active: string
	sub:    string
	...
}

#Filesystem: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "linux.filesystem"
		identity_fields: ["host", "mountpoint"]
		tracked_fields: ["device", "fstype", "size_bytes", "used_bytes", "avail_bytes"]
	}
	host:        string
	device:      string
	mountpoint:  string
	fstype:      string
	size_bytes:  int
	used_bytes:  int
	avail_bytes: int
	...
}

#User: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "linux.user"
		identity_fields: ["host", "name"]
		tracked_fields: ["uid", "gid", "home", "shell"]
	}
	host:  string
	name:  string
	uid:   int
	gid:   int
	home:  string
	shell: string
	...
}

#NetworkInterface: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "linux.network_interface"
		identity_fields: ["host", "ifname"]
		tracked_fields: ["operstate", "addr_info"]
	}
	host:        string
	ifname:      string
	operstate?:  string
	flags?:      [...string]
	mtu?:        int
	link_type?:  string
	address?:    string
	addr_info?:  [...{
		family:     string
		local:      string
		prefixlen:  int
		...
	}]
	...
}
`
	s.WriteFileInSubDir("schema/pudl/linux", "linux.cue", linuxContent)
}

// Cleanup manually cleans up the workspace (usually not needed due to t.Cleanup)
func (s *TempDirSetup) Cleanup() {
	if s.cleanup != nil {
		s.cleanup()
		s.cleanup = nil
	}
}
