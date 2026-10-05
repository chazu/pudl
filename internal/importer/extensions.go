package importer

import (
	"path/filepath"
	"strings"
)

// importableExtensions are the data file extensions a directory import picks
// up. Other files in the directory are ignored rather than imported as
// unknown-format data.
var importableExtensions = map[string]bool{
	".json":   true,
	".ndjson": true,
	".jsonl":  true,
	".yaml":   true,
	".yml":    true,
	".csv":    true,
}

// IsImportableName reports whether a file name has a supported data extension,
// looking through a .gz or .zst compression suffix.
func IsImportableName(name string) bool {
	lower := strings.ToLower(name)
	switch filepath.Ext(lower) {
	case ".gz", ".zst":
		lower = strings.TrimSuffix(lower, filepath.Ext(lower))
	}
	return importableExtensions[filepath.Ext(lower)]
}
