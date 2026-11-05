package examples

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/fileset"
)

func TestInstallRecordsUnderExamples(t *testing.T) {
	root := t.TempDir()
	_, err := Install(root, "git-inventory", fileset.Options{})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(root, ".examples", "git-inventory.json"))
}

// The content packs left examples: they install with pudl pack install.
func TestPacksAreNotExamples(t *testing.T) {
	for _, name := range []string{"gcp", "aws"} {
		_, err := Install(t.TempDir(), name, fileset.Options{})
		require.ErrorContains(t, err, `unknown example "`+name+`" (available: git-inventory, gcp-network-hygiene)`)
	}
}
