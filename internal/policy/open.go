package policy

import (
	"bytes"
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"
	cueerrors "cuelang.org/go/cue/errors"
)

// openValidate unifies member m with the record and returns the validation
// errors, ignoring closedness at every depth.
//
// A definition is closed at every depth. CUE reports a record field the
// member does not declare as "field not allowed", one struct level at a time,
// and such an error inside a nested struct breaks references into that
// struct: `if privateClusterConfig.enablePrivateEndpoint != _|_` sees bottom
// when the record has extra fields under privateClusterConfig. So each
// record field CUE rejects as not allowed, and that the member does not
// declare, is removed from (a copy of) the record and the unification is
// repeated until none is left. No constraint of the member applies to such a
// field, so removing it hides no violation. Only the final unification's
// errors are returned: errors from an earlier one may stem from a broken
// reference.
//
// dv is the full record compiled in m's context.
func openValidate(m member, dv cue.Value, raw []byte, data any) ([]cueerrors.Error, error) {
	root := len(m.value.Path().Selectors())
	cur, copied := data, false
	for {
		errs := cueerrors.Errors(m.value.Unify(dv).Validate())
		removed := false
		for _, e := range errs {
			if format, _ := e.Msg(); format != closednessError {
				continue
			}
			labels := e.Path()
			if len(labels) <= root {
				continue
			}
			segs, present := recordPath(cur, labels[root:])
			if !present || declaresAt(m.value, segs) {
				continue // a field the member adds, not record data
			}
			if !copied {
				// json.Number keeps every number's text exact when the
				// pruned record is encoded again.
				dec := json.NewDecoder(bytes.NewReader(raw))
				dec.UseNumber()
				var fresh any
				if err := dec.Decode(&fresh); err != nil {
					return nil, fmt.Errorf("decode record: %w", err)
				}
				cur, copied = fresh, true
			}
			removed = removeRecordPath(cur, labels[root:]) || removed
		}
		if !removed {
			return errs, nil
		}
		pruned, err := json.Marshal(cur)
		if err != nil {
			return nil, fmt.Errorf("encode record: %w", err)
		}
		if dv = m.value.Context().CompileBytes(pruned); dv.Err() != nil {
			return nil, fmt.Errorf("compile record: %w", dv.Err())
		}
	}
}

// declaresAt reports whether v declares a regular field at the tree path segs.
func declaresAt(v cue.Value, segs []string) bool {
	cur := v
	for _, seg := range segs {
		if seg == wildcard {
			if cur = cur.LookupPath(cue.MakePath(cue.AnyIndex)); !cur.Exists() {
				return false
			}
			continue
		}
		next, _, found := fieldOf(cur, seg)
		if !found {
			return false
		}
		cur = next
	}
	return true
}
