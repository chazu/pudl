package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/repair"
	"github.com/chazu/pudl/internal/validator"
)

// Items of an imported array were inferred one by one. When a pack later
// gains a type that claims them, doctor --fix reclassifies them, as it does
// any inferred entry. (Re-importing identical content dedupes to the old
// entry, so this is the only way an upgraded pack reaches existing records.)
func TestDoctorFixReinfersImportedArrayItems(t *testing.T) {
	pudlDir := cliWorkspace(t)
	r := runCLI(t, "example", "install", "gcp", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	// An older pack without the instance type.
	typeFile := filepath.Join(pudlDir, "schema", "gcp", "compute.cue")
	saved, err := os.ReadFile(typeFile)
	require.NoError(t, err)
	require.NoError(t, os.Remove(typeFile))
	resetSchemaCaches()

	vms := filepath.Join(pudlDir, "populators", "gcp", "instances.json")
	r = runCLI(t, "import", "--path", vms, "--set", "project=example-project", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Empty(t, instancesInGraph(t), "without the type, the VMs have no identity")

	// The newer pack arrives.
	require.NoError(t, os.WriteFile(typeFile, saved, 0o644))
	resetSchemaCaches()
	_, repairs := doctorRepairs(t, "--fix")
	require.Equal(t, repair.StatusApplied, repairs["inference"].Status, "%+v", repairs["inference"])
	require.Equal(t, 3, repairs["inference"].Changed)
	require.Len(t, instancesInGraph(t), 3)
}

func resetSchemaCaches() {
	inference.ResetShared()
	validator.ResetSharedLoaders()
}

func instancesInGraph(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, r := range exportGraph(t).Resources {
		if r.Type == "gcp.compute.instance" {
			out = append(out, r.Name)
		}
	}
	return out
}
