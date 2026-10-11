package acute

import (
	"fmt"
	"strings"
	"time"

	resourceidentity "github.com/chazu/pudl/internal/identity"
)

// IdentityResolver returns the declared identity_fields for a schema, or nil
// when the schema is unknown or declares none.
type IdentityResolver func(schema string) []string

// ObservedRecord is one observed inventory record and when it was recorded.
type ObservedRecord struct {
	// IdentityFields are the field names recorded on the catalog entry at ingestion.
	IdentityFields []string
	Data           map[string]any
	ObservedAt     *time.Time
}

// RecordIdentity derives a stable match key for a record: its _schema plus the
// values of the schema's declared identity_fields. When the schema declares no
// identity_fields it falls
// back to the first present of name | path | id, which covers the linux/fs/k8s
// desired shapes.
func RecordIdentity(rec map[string]any, identity IdentityResolver) (key string, label string, ok bool) {
	schema, _ := rec["_schema"].(string)

	if identity != nil {
		if fields := identity(schema); len(fields) > 0 {
			values, err := resourceidentity.ExtractFieldValues(rec, fields)
			if err != nil {
				return "", "", false
			}
			encoded, err := resourceidentity.CanonicalIdentityJSON(values)
			if err != nil {
				return "", "", false
			}
			vals := make([]string, 0, len(fields))
			for _, f := range fields {
				vals = append(vals, fmt.Sprint(values[f]))
			}
			return schema + "|" + encoded, schemaLabel(schema) + "/" + strings.Join(vals, "/"), true
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
	return inventorySetDiff(desired, observed, identity, nil)
}

func InventorySetDiffWithPrevious(desired []map[string]any, observed []ObservedRecord, identity IdentityResolver, history PreviousInventory) []ResourceDrift {
	return inventorySetDiff(desired, observed, identity, &history)
}

func inventorySetDiff(desired []map[string]any, observed []ObservedRecord, identity IdentityResolver, history *PreviousInventory) []ResourceDrift {
	byKey := make(map[string][]ObservedRecord, len(observed))
	for _, o := range observed {
		if k, _, ok := RecordIdentity(o.Data, identity); ok {
			byKey[k] = appendDistinct(byKey[k], o)
		}
	}

	var drifted []ResourceDrift
	for i, d := range desired {
		before := len(drifted)
		k, label, ok := RecordIdentity(d, identity)
		if !ok {
			drifted = append(drifted, ResourceDrift{
				Resource: unidentifiableLabel(d, i),
				Reason:   DriftUnidentifiable,
				Diff:     "desired record has no identity (no identity_fields, name, path or id) to match an observation",
			})
			if history != nil {
				attachPrevious(&drifted[len(drifted)-1], d, identity, *history)
			}
			continue
		}
		matches := byKey[k]
		switch len(matches) {
		case 0:
			drifted = append(drifted, ResourceDrift{Resource: label, Reason: DriftMissing})
		case 1:
			if fields := compareDesired(d, matches[0].Data, history != nil); len(fields) > 0 {
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
		if history != nil && len(drifted) > before {
			attachPrevious(&drifted[len(drifted)-1], d, identity, *history)
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
