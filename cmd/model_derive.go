package cmd

import (
	"github.com/chazu/pudl/internal/systemmodel"
)

// Phase 2 — derived cross-model dependencies.
//
// A model's dependency is often latent in its `desired`: model B's desired
// resource references an identity that model A manages (e.g. B's Deployment sits
// in a Namespace A declares). We derive `model_depends_on(B, A)` by matching B's
// referenced values against A's produced identities — no manual depends_on.
//
// Design note: `desired` is not SQL-queryable (it lives in the in-memory model /
// stored record file, not a catalog column), and tags.model is set only by the
// converge path. So derivation runs in Go over resolved models and emits the
// SAME `model_depends_on` relation (under a `derived:` source) — the Phase-1
// rules are unchanged. Derivation is value-based and therefore heuristic: it can
// over-match on a coincidental string equality. It is opt-in (`pudl model deps
// --derive`), separately sourced (auditable, never corrupts the declared graph),
// and skips edges a model already declares.

// structuralKeys are desired fields that name a TYPE, not a resource identity or
// a cross-resource reference. Excluded from both produced identities and
// referenced values so they can't mint spurious edges (e.g. two models both
// declaring kind "Deployment" must not become a dependency).
var structuralKeys = map[string]bool{"kind": true, "apiVersion": true, "_schema": true}

// producedIdentities returns the resource identities a model manages/produces
// from its desired: the top-level identity values (identity_fields or
// name|path|id, via modelResourceDefs) UNION the k8s `metadata.name` values.
// It deliberately does NOT collect every nested "name" (container/port/volume
// names are not resource identities) — only the name directly under a metadata
// map, which is the k8s resource identity.
func producedIdentities(desired []map[string]any, identity identityResolver) map[string]struct{} {
	out := map[string]struct{}{}
	for _, v := range modelResourceDefs(desired, identity) {
		out[v] = struct{}{}
	}
	for _, d := range desired {
		collectMetadataNames(d, out)
	}
	return out
}

// referencedValues collects the string leaf values appearing anywhere in a
// model's desired entries — the candidate cross-resource references — skipping
// structural type tags (kind/apiVersion/_schema).
func referencedValues(desired []map[string]any) map[string]struct{} {
	out := map[string]struct{}{}
	for _, d := range desired {
		collectStrings(d, out)
	}
	return out
}

// collectStrings walks any JSON-like value and adds every non-empty string leaf,
// skipping values under structural keys.
func collectStrings(v any, out map[string]struct{}) {
	switch t := v.(type) {
	case string:
		if t != "" {
			out[t] = struct{}{}
		}
	case map[string]any:
		for k, vv := range t {
			if structuralKeys[k] {
				continue
			}
			collectStrings(vv, out)
		}
	case []any:
		for _, vv := range t {
			collectStrings(vv, out)
		}
	}
}

// collectMetadataNames adds the value of `metadata.name` for every map (at any
// depth) that has a metadata sub-map with a string name — the k8s resource
// identity. It does NOT match a bare "name" elsewhere (e.g. a container's name).
func collectMetadataNames(v any, out map[string]struct{}) {
	switch t := v.(type) {
	case map[string]any:
		if meta, ok := t["metadata"].(map[string]any); ok {
			if name, ok := meta["name"].(string); ok && name != "" {
				out[name] = struct{}{}
			}
		}
		for _, vv := range t {
			collectMetadataNames(vv, out)
		}
	case []any:
		for _, vv := range t {
			collectMetadataNames(vv, out)
		}
	}
}

// deriveDependencies computes the derived dependency edges across a set of
// models: B -> A when B references an identity A produces, A != B, and B does
// not already DECLARE A (declared wins; no duplicate). Returns model name ->
// set of derived dependency names.
func deriveDependencies(models []*systemmodel.SystemModel, identity identityResolver) map[string]map[string]struct{} {
	produced := make(map[string]map[string]struct{}, len(models))
	declaredByModel := make(map[string]map[string]struct{}, len(models))
	for _, m := range models {
		produced[m.Name] = producedIdentities(m.Desired, identity)
		d, _ := declaredDepsOf(m)
		declaredByModel[m.Name] = d
	}

	out := map[string]map[string]struct{}{}
	for _, b := range models {
		refs := referencedValues(b.Desired)
		// B referencing its own produced identity is not a cross-model edge.
		for own := range produced[b.Name] {
			delete(refs, own)
		}
		if len(refs) == 0 {
			continue
		}
		for _, a := range models {
			if a.Name == b.Name {
				continue
			}
			if _, declared := declaredByModel[b.Name][a.Name]; declared {
				continue // declared edge already covers this; don't duplicate
			}
			if intersects(refs, produced[a.Name]) {
				if out[b.Name] == nil {
					out[b.Name] = map[string]struct{}{}
				}
				out[b.Name][a.Name] = struct{}{}
			}
		}
	}
	return out
}

// intersects reports whether two string sets share any element.
func intersects(a, b map[string]struct{}) bool {
	// iterate the smaller set
	if len(b) < len(a) {
		a, b = b, a
	}
	for k := range a {
		if _, ok := b[k]; ok {
			return true
		}
	}
	return false
}
