package graphexport

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/graphspec"
	"github.com/chazu/pudl/internal/validator"
)

// MetadataFunc looks up a schema's metadata.
type MetadataFunc func(schema string) (validator.SchemaMetadata, bool)

// LoadFunc decodes an entry's stored (already redacted) payload.
type LoadFunc func(entry database.CatalogEntry) (any, error)

// ObservationsFunc lists the snapshots that observed a resource, newest first.
type ObservationsFunc func(ctx context.Context, resourceID string) ([]database.ResourceObservation, error)

const (
	maxProperties   = 40
	maxListItems    = 20
	maxObservations = 5
)

// buildResource turns a current catalog entry into a graph resource. A
// payload that cannot be read still yields a resource, labelled from its
// identity, with a warning.
func buildResource(ctx context.Context, entry database.CatalogEntry, meta MetadataFunc, load LoadFunc, obs ObservationsFunc, warn func(string)) (Resource, []string) {
	m, _ := meta(entry.Schema)
	spec := graphSpecFor(entry.Schema, meta)
	identity := identityValues(entry)
	r := Resource{
		ID:         *entry.ResourceID,
		Type:       m.ResourceType,
		Schema:     entry.Schema,
		EntryID:    entry.ID,
		NativeID:   joinIdentity(identity),
		Properties: map[string]any{},
	}
	if r.Type == "" {
		r.Type = "unknown"
	}
	r.Provider = strings.SplitN(r.Type, ".", 2)[0]

	var record any
	if data, err := load(entry); err != nil {
		warn(fmt.Sprintf("resource %s: payload unreadable (%v); labelled from identity only", r.ID, err))
	} else {
		record = data
		r.Properties = scalarProperties(data)
	}
	r.Name = firstNonEmpty(graphspec.Eval(specValue(spec, "name"), record), topLevelString(record, "name", "Name"), r.NativeID, r.ID)
	r.Account = graphspec.Eval(specValue(spec, "account"), record)
	r.Region = graphspec.Eval(specValue(spec, "region"), record)
	r.Zone = graphspec.Eval(specValue(spec, "zone"), record)
	if r.Region == "" {
		r.Region = regionOfZone(r.Zone)
	}
	r.Evidence = resourceEvidence(ctx, entry, obs, warn)

	// Reference values other records may use for this resource.
	refs := spec.RefValues(record)
	if len(identity) == 1 {
		for _, v := range identity {
			refs = append(refs, v)
		}
	}
	return r, refs
}

// graphSpecFor returns the nearest `_pudl.graph` along the base_schema chain.
func graphSpecFor(schema string, meta MetadataFunc) *graphspec.Spec {
	seen := map[string]bool{}
	for name := schema; name != "" && !seen[name]; {
		seen[name] = true
		m, ok := meta(name)
		if !ok {
			return nil
		}
		if m.Graph != nil {
			return m.Graph
		}
		name = m.BaseSchema
	}
	return nil
}

func specValue(spec *graphspec.Spec, field string) *graphspec.Value {
	if spec == nil {
		return nil
	}
	switch field {
	case "name":
		return spec.Name
	case "account":
		return spec.Account
	case "region":
		return spec.Region
	default:
		return spec.Zone
	}
}

func identityValues(entry database.CatalogEntry) map[string]string {
	out := map[string]string{}
	if entry.IdentityJSON == nil {
		return out
	}
	var raw map[string]any
	if json.Unmarshal([]byte(*entry.IdentityJSON), &raw) != nil {
		return out
	}
	for k, v := range raw {
		if s, ok := graphspec.Scalar(v); ok {
			out[k] = s
		}
	}
	return out
}

func joinIdentity(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = values[k]
	}
	return strings.Join(parts, "/")
}

// scalarProperties keeps top-level scalars and short scalar lists; nested
// objects are left to `pudl show`.
func scalarProperties(data any) map[string]any {
	out := map[string]any{}
	obj, ok := data.(map[string]any)
	if !ok {
		return out
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if len(out) >= maxProperties {
			break
		}
		switch v := obj[k].(type) {
		case []any:
			if len(v) > 0 && len(v) <= maxListItems && allScalar(v) {
				out[k] = v
			}
		case map[string]any, nil:
		default:
			out[k] = v
		}
	}
	return out
}

func allScalar(items []any) bool {
	for _, item := range items {
		if _, ok := graphspec.Scalar(item); !ok {
			return false
		}
	}
	return true
}

func topLevelString(record any, keys ...string) string {
	obj, ok := record.(map[string]any)
	if !ok {
		return ""
	}
	for _, k := range keys {
		if s, ok := obj[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// resourceEvidence lists the snapshots that observed the resource. A
// resource only ever imported says so: pudl did not observe it.
func resourceEvidence(ctx context.Context, entry database.CatalogEntry, obs ObservationsFunc, warn func(string)) []Evidence {
	list, err := obs(ctx, *entry.ResourceID)
	if err != nil {
		warn(fmt.Sprintf("resource %s: observations unreadable: %v", *entry.ResourceID, err))
	}
	var out []Evidence
	for i, o := range list {
		if i == maxObservations {
			break
		}
		detail := "scope " + o.Scope
		if !o.Complete {
			detail += "; incomplete snapshot"
		}
		out = append(out, Evidence{
			Basis: "observed", Source: o.Source, ObservedAt: o.ObservedAt.UTC().Format(time.RFC3339),
			SnapshotID: o.SnapshotID, Detail: detail,
		})
	}
	if len(out) == 0 {
		out = append(out, Evidence{
			Basis:      "unknown",
			Source:     "pudl import (" + entry.Origin + ")",
			ObservedAt: entry.ImportTimestamp.UTC().Format(time.RFC3339),
			Detail:     "Imported from a file; no pudl collector observed it.",
		})
	}
	return out
}

// zoneSuffix matches a zone's letter after its region: GCP's "us-central1-a"
// and AWS's "us-east-1a".
var zoneSuffix = regexp.MustCompile(`^(.*\d)-?[a-z]$`)

// regionOfZone derives a region from a zone name, or "" when it is not one.
func regionOfZone(zone string) string {
	if m := zoneSuffix.FindStringSubmatch(zone); m != nil {
		return m[1]
	}
	return ""
}
