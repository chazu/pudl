package webui

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAssetsHoldTheBuiltUI(t *testing.T) {
	index, err := fs.ReadFile(Assets(), "index.html")
	require.NoError(t, err, "run `make web` to build the UI bundle")
	require.Contains(t, string(index), `<div id="root">`)
	entries, err := fs.ReadDir(Assets(), "assets")
	require.NoError(t, err)
	var js int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".js") {
			js++
			// index.html must reference only bundles that are embedded.
		}
	}
	require.Greater(t, js, 0)
	for _, ref := range strings.Split(string(index), `"/assets/`)[1:] {
		name := ref[:strings.IndexByte(ref, '"')]
		_, err := fs.Stat(Assets(), "assets/"+name)
		require.NoError(t, err, "index.html references a missing bundle %s", name)
	}
}
