package graphexport

import "github.com/chazu/pudl/internal/graphvocab"

// Owners and the administrative overview, for partial graphs. An owner is a
// resource of a type the vocabulary marks admin.owner (AWS account, GCP
// project); it owns every resource whose provider matches and whose
// account attribute names the owner's native ID. Ownership is an
// attribute, not a relationship, so following relationships from an owner
// never reaches what it owns: these functions do.

// ownerKey identifies an owner by provider and native ID, as members name it.
type ownerKey struct{ provider, account string }

// adminTypes returns the resource types with an admin role, and the subset
// that are owners.
func adminTypes(g *Graph) (admin, owner map[string]bool) {
	admin, owner = map[string]bool{}, map[string]bool{}
	if g.Vocabulary == nil {
		return admin, owner
	}
	for _, rt := range g.Vocabulary.ResourceTypes {
		if rt.Admin != nil {
			admin[rt.Type] = true
			if rt.Admin.Owner {
				owner[rt.Type] = true
			}
		}
	}
	return admin, owner
}

// OverviewIDs lists the resources with an administrative role
// (organizations, folders, accounts, projects): the map a large inventory
// opens on.
func OverviewIDs(g *Graph) []string {
	admin, _ := adminTypes(g)
	var out []string
	for _, r := range g.Resources {
		if admin[r.Type] {
			out = append(out, r.ID)
		}
	}
	return out
}

// OwnedIDs lists the resources owned by the given owner resources.
// IDs that are not owners contribute nothing.
func OwnedIDs(g *Graph, owners []string) []string {
	_, ownerType := adminTypes(g)
	want := map[string]bool{}
	for _, id := range owners {
		want[id] = true
	}
	keys := map[ownerKey]bool{}
	for _, r := range g.Resources {
		if want[r.ID] && ownerType[r.Type] && r.NativeID != "" {
			keys[ownerKey{graphvocab.ProviderOf(r.Type), r.NativeID}] = true
		}
	}
	var out []string
	for _, r := range g.Resources {
		if r.Account != "" && keys[ownerKey{r.Provider, r.Account}] && !want[r.ID] {
			out = append(out, r.ID)
		}
	}
	return out
}

// ownedCounts maps each owner resource's ID to how many resources it owns
// in g. Owners that own nothing are left out.
func ownedCounts(g *Graph) map[string]int {
	_, ownerType := adminTypes(g)
	byKey := map[ownerKey]string{}
	for _, r := range g.Resources {
		if ownerType[r.Type] && r.NativeID != "" {
			byKey[ownerKey{graphvocab.ProviderOf(r.Type), r.NativeID}] = r.ID
		}
	}
	counts := map[string]int{}
	for _, r := range g.Resources {
		if id, ok := byKey[ownerKey{r.Provider, r.Account}]; ok && r.Account != "" && id != r.ID {
			counts[id]++
		}
	}
	return counts
}
