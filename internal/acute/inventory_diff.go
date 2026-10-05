package acute

import (
	"fmt"
	"strings"
	"time"
)

// IdentityResolver returns the declared identity_fields for a schema, or nil
// when the schema is unknown or declares none.
type IdentityResolver func(schema string) []string

// ObservedRecord is one observed inventory record and when it was recorded.
type ObservedRecord struct {
	Data       map[string]any
	ObservedAt *time.Time
}

// RecordIdentity derives a stable match key for a record: its _schema plus the
// values of the schema's declared identity_fields. When the schema declares no
// identity_fields — or a declared field is absent from the record — it falls
// back to the first present of name | path | id, which covers the linux/fs/k8s
// desired shapes.
func RecordIdentity(rec map[string]any, identity IdentityResolver) (key string, label string, ok bool) {
	schema, _ := rec["_schema"].(string)

	if identity != nil {
		if fields := identity(schema); len(fields) > 0 {
			vals := make([]string, 0, len(fields))
			complete := true
			for _, f := range fields {
				v, present := rec[f]
				if !present {
					complete = false
					break
				}
				vals = append(vals, fmt.Sprintf("%v", v))
			}
			if complete {
				joined := strings.Join(vals, "/")
				return fmt.Sprintf("%s|%s", schema, joined), fmt.Sprintf("%s/%s", schemaLabel(schema), joined), true
			}
		}
	}

	for _, k := range []string{"name", "path", "id"} {
		if v, present := rec[k]; present {
			return fmt.Sprintf("%s|%v", schema, v), fmt.Sprintf("%s/%v", schemaLabel(schema), v), true
		}
	}
	return "", "", false
}

func schemaLabel(s string) string {
	if s == "" {
		return "?"
	}
	return s
}

// InventorySetDiff compares desired records against observed (inventory)
// records by identity, with ensure-present semantics. Extra observed records
// are ignored (prune is deferred, matching host-converge V1).
//
// It fails closed: a desired record that cannot be compared is reported as
// drift rather than skipped, because skipping it lets a model whose records
// carry no identity report clean having compared nothing. A desired record
// without identity is unidentifiable; one whose identity matches several
// differing observed records is ambiguous. Results follow desired order, and
// each changed resource lists every unsatisfied field in path order.
func InventorySetDiff(desired []map[string]any, observed []ObservedRecord, identity IdentityResolver) []ResourceDrift {
	byKey := make(map[string][]ObservedRecord, len(observed))
	for _, o := range observed {
		if k, _, ok := RecordIdentity(o.Data, identity); ok {
			byKey[k] = appendDistinct(byKey[k], o)
		}
	}

	var drifted []ResourceDrift
	for i, d := range desired {
		k, label, ok := RecordIdentity(d, identity)
		if !ok {
			drifted = append(drifted, ResourceDrift{
				Resource: unidentifiableLabel(d, i),
				Reason:   DriftUnidentifiable,
				Diff:     "desired record has no identity (no identity_fields, name, path or id) to match an observation",
			})
			continue
		}
		matches := byKey[k]
		switch len(matches) {
		case 0:
			drifted = append(drifted, ResourceDrift{Resource: label, Reason: DriftMissing})
		case 1:
			if fields := CompareDesired(d, matches[0].Data); len(fields) > 0 {
				drifted = append(drifted, ResourceDrift{
					Resource:   label,
					Reason:     DriftChanged,
					Fields:     fields,
					ObservedAt: matches[0].ObservedAt,
					Diff:       summarizeFields(fields),
				})
			}
		default:
			drifted = append(drifted, ResourceDrift{
				Resource:   label,
				Reason:     DriftAmbiguous,
				ObservedAt: latestObservedAt(matches),
				Diff:       fmt.Sprintf("%d differing observed records share this identity", len(matches)),
			})
		}
	}
	return drifted
}

// appendDistinct adds o unless an observed record with equal content is already
// present: the same record observed twice is one resource, not an ambiguity.
func appendDistinct(records []ObservedRecord, o ObservedRecord) []ObservedRecord {
	for _, r := range records {
		if ValuesEqual(r.Data, o.Data) {
			return records
		}
	}
	return append(records, o)
}

func latestObservedAt(records []ObservedRecord) *time.Time {
	var latest *time.Time
	for _, r := range records {
		if r.ObservedAt != nil && (latest == nil || r.ObservedAt.After(*latest)) {
			latest = r.ObservedAt
		}
	}
	return latest
}

// unidentifiableLabel names a desired record that has no identity by its schema
// and position, so the operator can find it in the model.
func unidentifiableLabel(rec map[string]any, index int) string {
	schema, _ := rec["_schema"].(string)
	return fmt.Sprintf("%s/desired[%d]", schemaLabel(schema), index)
}
