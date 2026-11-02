package importer

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

// retiredBootstrapFiles lists files the bootstrap tree once shipped, with the
// SHA-256 of every shipped version. A workspace copy that still matches one is
// pudl's own and is removed (with its directory, once empty); an edited copy
// or a symlink is the user's and is kept. Existing workspaces never re-copy a
// removed file, so without this a retired schema would keep loading there and
// doctor would report it as a user schema in the reserved pudl/ namespace.
var retiredBootstrapFiles = map[string][]string{
	// The agent-memory schemas (removed 2026-09-30).
	"pudl/nous/nous.cue": {"ab3bfdb46a18648332db6635027806080c24fc317a1ce36b3c6d4125c722c496"},
	// mu build targets, unused since the mu integration was removed (2026-11-01).
	"pudl/brick/brick.cue": {
		"9d794d878a83ea18bb0899108ded217afa71f6cf0a967b3329e904541a4900e9",
		"2eeae892dbaf62a623c52f78cf873f22716975af9ef1e3477a9ccd791845b56e",
	},
}

// retireBootstrapFiles removes the unmodified workspace copies of retired
// bootstrap files under schemaPath.
func retireBootstrapFiles(schemaPath string) error {
	for rel, shipped := range retiredBootstrapFiles {
		if err := retireFile(filepath.Join(schemaPath, filepath.FromSlash(rel)), shipped); err != nil {
			return fmt.Errorf("retire %s: %w", rel, err)
		}
	}
	return nil
}

func retireFile(path string, shipped []string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(data))
	for _, hash := range shipped {
		if sum == hash {
			if err := os.Remove(path); err != nil {
				return err
			}
			_ = os.Remove(filepath.Dir(path)) // only succeeds when empty
			return nil
		}
	}
	return nil
}
