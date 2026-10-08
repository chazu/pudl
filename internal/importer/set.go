package importer

import (
	"fmt"

	"github.com/chazu/pudl/internal/fieldpath"
)

// FieldAssignment is one `pudl import --set path=value`: Value is stored at
// Path in every imported record, overwriting what is there.
type FieldAssignment struct {
	Path  fieldpath.Path
	Value any
}

// applyAssignments applies every assignment to record, which must be a JSON
// object.
func applyAssignments(record any, set []FieldAssignment) error {
	if len(set) == 0 {
		return nil
	}
	obj, ok := record.(map[string]any)
	if !ok {
		return fmt.Errorf("--set applies only to JSON object records (got %T)", record)
	}
	for _, a := range set {
		if err := a.Path.Set(obj, a.Value); err != nil {
			return err
		}
	}
	return nil
}
