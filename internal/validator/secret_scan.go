package validator

import (
	"fmt"

	"cuelang.org/go/cue"

	"github.com/chazu/pudl/internal/fieldpath"
)

// decodeSecretScanIgnore strictly decodes `_pudl.secret_scan_ignore`: the
// field paths whose values are known not to be secrets, so the likely-secret
// scan stays quiet about them. A present but invalid list (not concrete, not a
// list of strings, or an unparsable path) suppresses nothing and records
// SecretScanIgnoreError, which CheckModuleIntegrity reports as a schema load
// error: a broken allowlist must neither vanish silently nor widen.
func decodeSecretScanIgnore(pudl cue.Value, meta *SchemaMetadata) {
	v := pudl.LookupPath(cue.ParsePath("secret_scan_ignore"))
	if !v.Exists() {
		return
	}
	paths, err := parseSecretScanIgnore(v)
	if err != nil {
		meta.SecretScanIgnoreError = fmt.Sprintf("secret_scan_ignore: %v", err)
		return
	}
	meta.SecretScanIgnore = paths
}

func parseSecretScanIgnore(v cue.Value) ([]string, error) {
	if err := v.Validate(cue.Concrete(true)); err != nil {
		return nil, err
	}
	var paths []string
	if err := v.Decode(&paths); err != nil {
		return nil, fmt.Errorf("must be a list of field paths")
	}
	for _, p := range paths {
		if _, err := fieldpath.Parse(p); err != nil {
			return nil, err
		}
	}
	return paths, nil
}
