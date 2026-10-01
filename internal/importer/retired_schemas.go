package importer

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

// Retire only the exact shipped schema. Authored replacements and symlinks are
// preserved; legacy fact bodies remain ordinary, queryable JSON assertions.
func retireMemorySchema(schemaPath string) error {
	path := filepath.Join(schemaPath, "pudl", "nous", "nous.cue")
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
	if fmt.Sprintf("%x", sha256.Sum256(data)) != "ab3bfdb46a18648332db6635027806080c24fc317a1ce36b3c6d4125c722c496" {
		return nil
	}
	return os.Remove(path)
}
