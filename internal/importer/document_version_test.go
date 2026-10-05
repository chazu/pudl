package importer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chazu/pudl/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestConcurrentDocumentVersionsAndMetadataAgree(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	// Mu target identity is deliberately stable as its payload changes.
	sourceA := setup.WriteFile("a.json", `{"target":"//host/test","toolchain":"a"}`)
	sourceB := setup.WriteFile("b.json", `{"target":"//host/test","toolchain":"b"}`)
	impA, err := NewEnhancedImporter(workspace.DataDir, gitSchemaDir(t), workspace.Root)
	require.NoError(t, err)
	defer impA.Close()
	impB, err := NewEnhancedImporter(workspace.DataDir, gitSchemaDir(t), workspace.Root)
	require.NoError(t, err)
	defer impB.Close()
	type outcome struct {
		result *ImportResult
		err    error
	}
	out := make(chan outcome, 2)
	start := make(chan struct{})
	for i, imp := range []*EnhancedImporter{impA, impB} {
		source := []string{sourceA, sourceB}[i]
		go func() {
			<-start
			result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, ManualSchema: "pudl/mu.#Target"})
			out <- outcome{result, err}
		}()
	}
	close(start)
	versions := map[int]bool{}
	resource := ""
	for range 2 {
		got := <-out
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		versions[got.result.Version] = true
		if resource == "" {
			resource = got.result.ResourceID
		} else {
			require.Equal(t, resource, got.result.ResourceID)
		}
		bytes, err := os.ReadFile(filepath.Clean(got.result.MetadataPath))
		require.NoError(t, err)
		var meta ImportMetadata
		require.NoError(t, json.Unmarshal(bytes, &meta))
		require.Equal(t, got.result.Version, meta.ResourceTracking.Version)
		entry, err := impA.catalogDB.GetEntry(got.result.ID)
		require.NoError(t, err)
		require.Equal(t, got.result.Version, *entry.Version)
	}
	require.Equal(t, map[int]bool{1: true, 2: true}, versions)
}
