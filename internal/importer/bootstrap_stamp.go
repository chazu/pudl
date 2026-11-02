package importer

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/chazu/pudl/internal/systemmodel"
)

// bootstrapStampFile records which built-ins a workspace holds: the digest of
// the built-in schema files this pudl ships. pudl owns those files (`pudl
// init` overwrites them), so a workspace whose stamp differs (written by
// another pudl, or never) gets every built-in rewritten on its next import.
// That is how a changed or removed built-in definition reaches an existing
// workspace; retired files are removed by retireBootstrapFiles.
const bootstrapStampFile = "pudl/.bootstrap-digest"

// BootstrapDigest returns the digest of the built-in schema files.
var BootstrapDigest = sync.OnceValue(func() string {
	contents := map[string][]byte{}
	err := fs.WalkDir(bootstrapSchemas, "bootstrap", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := bootstrapSchemas.ReadFile(path)
		contents[path] = data
		return err
	})
	if err != nil {
		// The tree is embedded at build time; failing to walk it is a build defect.
		panic(fmt.Sprintf("walk embedded bootstrap schemas: %v", err))
	}
	contents["systemmodel"] = []byte(systemmodel.SchemaCUE())
	paths := make([]string, 0, len(contents))
	for path := range contents {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, path := range paths {
		fmt.Fprintf(h, "%s\x00%d\x00", path, len(contents[path]))
		h.Write(contents[path])
	}
	return fmt.Sprintf("%x", h.Sum(nil))
})

func writeBootstrapStamp(schemaPath string) error {
	return os.WriteFile(filepath.Join(schemaPath, filepath.FromSlash(bootstrapStampFile)), []byte(BootstrapDigest()+"\n"), 0644)
}

// bootstrapCurrent reports whether the workspace's built-ins are this pudl's.
func bootstrapCurrent(schemaPath string) bool {
	data, err := os.ReadFile(filepath.Join(schemaPath, filepath.FromSlash(bootstrapStampFile)))
	return err == nil && strings.TrimSpace(string(data)) == BootstrapDigest()
}
