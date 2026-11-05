package doctor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/resourcegroup"
)

// SameResourceInput is what the shared identity check reads.
type SameResourceInput struct {
	// DeclaredKinds are the kinds same_resource_kind facts declare.
	DeclaredKinds map[string]bool
	// UsedKinds maps each kind a schema or rule names to where it is named
	// ("schema gcp.#GKECluster", "rule gcp_same_k8s_object").
	UsedKinds map[string][]string
	// Groups is the catalog's grouping; nil when there is no catalog.
	Groups *resourcegroup.Index
	// Err is a failure to read any of the above.
	Err error
}

// CheckSameResource reports kinds that no same_resource_kind fact declares
// and components that were not grouped because two members share an identity
// namespace. Both leave links missing rather than wrong, so they are warnings.
func CheckSameResource(in SameResourceInput) *CheckResult {
	if in.Err != nil {
		return &CheckResult{
			Status:  "warning",
			Message: "Shared identity could not be checked",
			Details: in.Err.Error(),
			Fix:     "Run 'pudl query same_resource' for details",
		}
	}
	var problems []string
	for _, kind := range sortedKinds(in.UsedKinds) {
		if !in.DeclaredKinds[kind] {
			problems = append(problems, fmt.Sprintf("kind %s is not declared by a same_resource_kind fact (named by %s)", kind, strings.Join(in.UsedKinds[kind], ", ")))
		}
	}
	groups := 0
	if in.Groups != nil {
		groups = len(in.Groups.Groups)
		for _, c := range in.Groups.Conflicts {
			namespaces := make([]string, 0, len(c.Namespaces))
			for ns, ids := range c.Namespaces {
				namespaces = append(namespaces, fmt.Sprintf("%s (%s)", ns, strings.Join(ids, ", ")))
			}
			sort.Strings(namespaces)
			problems = append(problems, fmt.Sprintf("%s records %s share a key but were not grouped: more than one record of %s",
				strings.Join(c.Kinds, "/"), strings.Join(c.Members, ", "), strings.Join(namespaces, "; ")))
		}
	}
	if len(problems) == 0 {
		return &CheckResult{
			Status:  "ok",
			Message: fmt.Sprintf("Shared identity: %d group(s), %d kind(s) declared", groups, len(in.DeclaredKinds)),
		}
	}
	return &CheckResult{
		Status:  "warning",
		Message: fmt.Sprintf("%d shared identity problem(s); affected records are not linked", len(problems)),
		Details: strings.Join(problems, "\n"),
		Fix:     "Declare missing kinds with same_resource_kind facts in a rule file; for a conflict, remove the stale record (pudl delete) or narrow the schema's _pudl.same_resource key (docs/schema-authoring.md, \"same_resource\")",
	}
}

func sortedKinds(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
