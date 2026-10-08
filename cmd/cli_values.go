package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/fieldpath"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/importer"
)

// parseCLIValue is the one typing rule for key=value command-line values
// (import --set, model new --input): a valid JSON value keeps its JSON type,
// with numbers decoded exactly as json.Number so a project number or other
// large integer is never rounded through float64. Anything that is not valid
// JSON stays a plain string; a JSON-quoted value ('"123"') forces a string.
func parseCLIValue(raw string) any {
	if !json.Valid([]byte(raw)) {
		return raw
	}
	value, err := idgen.DecodeJSONExact([]byte(raw))
	if err != nil {
		return raw
	}
	return value
}

// parseCLIAssignments parses repeated path=value flags whose keys are field
// paths. Wildcards are rejected: an assignment names exactly one location.
func parseCLIAssignments(flag string, args []string) ([]importer.FieldAssignment, error) {
	out := make([]importer.FieldAssignment, 0, len(args))
	for _, arg := range args {
		key, value, ok := strings.Cut(arg, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("--%s must use path=value (got %q)", flag, arg)
		}
		path, err := fieldpath.Parse(key)
		if err != nil {
			return nil, fmt.Errorf("--%s %q: %w", flag, arg, err)
		}
		if path.HasWildcard() {
			return nil, fmt.Errorf("--%s %q: wildcards are not allowed", flag, arg)
		}
		out = append(out, importer.FieldAssignment{Path: path, Value: parseCLIValue(value)})
	}
	return out, nil
}
