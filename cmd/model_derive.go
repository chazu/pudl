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

// producedIdentities returns the resource identities a model manages/produces
// from its desired: the top-level identity values (identity_fields or
// name|path|id, via modelResourceDefs) UNION any nested "name" values (the k8s
// case, where identity is metadata.name rather than a top-level field).
func producedIdentities(desired []map[string]any, identity identityResolver) map[string]struct{} {
	out := map[string]struct{}{}
	for _, v := range modelResourceDefs(desired, identity) {
		out[v] = struct{}{}
	}
	for _, d := range desired {
		collectNamedValues(d, "name", out)
	}
	return out
}

// referencedValues collects the string leaf values appearing anywhere in a
// model's desired entries — the candidate cross-resource references. `_schema`
// tags are skipped.
func referencedValues(desired []map[string]any) map[string]struct{} {
	out := map[string]struct{}{}
	for _, d := range desired {
		collectStrings(d, out)
	}
	return out
}

// collectStrings walks any JSON-like value and adds every non-empty string leaf.
func collectStrings(v any, out map[string]struct{}) {
	switch t := v.(type) {
	case string:
		if t != "" {
			out[t] = struct{}{}
		}
	case map[string]any:
		for k, vv := range t {
			if k == "_schema" {
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

// collectNamedValues walks any JSON-like value and adds the string value of
// every field whose key == key (at any depth).
func collectNamedValues(v any, key string, out map[string]struct{}) {
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			if k == key {
				if s, ok := vv.(string); ok && s != "" {
					out[s] = struct{}{}
				}
			}
			collectNamedValues(vv, key, out)
		}
	case []any:
		for _, vv := range t {
			collectNamedValues(vv, key, out)
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
