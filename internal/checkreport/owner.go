package checkreport

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/chazu/pudl/internal/acute"
)

// noOwner heads the findings no owner was resolved for: an unowned resource,
// or a witness that references no resource or catalog entry.
const noOwner = "(none)"

// OwnerOf resolves the owner of the first resource ID, then catalog entry ID,
// it knows, or "" (ownership.Index.OwnerOf).
type OwnerOf func(resourceIDs, entryIDs []string) string

// AttributeOwners sets each witness's owner from the resources it references:
// its resource_id and resource arguments, then its evidence (entry) IDs.
func AttributeOwners(results []CheckResult, ownerOf OwnerOf) {
	for i := range results {
		for j := range results[i].Witnesses {
			w := &results[i].Witnesses[j]
			var resources []string
			for _, key := range []string{"resource_id", "resource"} {
				if id, ok := w.Arguments[key].(string); ok && id != "" {
					resources = append(resources, id)
				}
			}
			if w.Resource != "" {
				resources = append(resources, w.Resource)
			}
			w.Owner = ownerOf(resources, w.EvidenceIDs)
		}
	}
}

// ForOwner returns a copy of the report limited to one owner's findings: a
// failed check keeps only the witnesses owner owns and is left out when none
// remain. Passed and uncertain checks, drift and counts are unchanged; counts
// still include every match.
func (r *ModelReport) ForOwner(owner string) *ModelReport {
	filtered := *r
	filtered.Checks = nil
	for _, c := range r.Checks {
		if c.Passed || c.Uncertain() {
			filtered.Checks = append(filtered.Checks, c)
			continue
		}
		var kept []Witness
		for _, w := range c.Witnesses {
			if w.Owner == owner {
				kept = append(kept, w)
			}
		}
		if len(kept) == 0 {
			continue
		}
		c.Witnesses = kept
		filtered.Checks = append(filtered.Checks, c)
	}
	return &filtered
}

// CheckForOwner returns a copy of a check report whose embedded member
// reports are limited to one owner's findings (ModelReport.ForOwner).
func CheckForOwner(report *acute.RunSetReport, owner string) (*acute.RunSetReport, error) {
	filtered := *report
	filtered.Members = make([]acute.RunSetMemberReport, len(report.Members))
	for i, member := range report.Members {
		if len(member.Report) > 0 {
			decoded, err := DecodeModelReport(member.Report)
			if err != nil {
				return nil, fmt.Errorf("decode member report %s: %w", member.Model, err)
			}
			if member.Report, err = json.Marshal(decoded.ForOwner(owner)); err != nil {
				return nil, err
			}
		}
		filtered.Members[i] = member
	}
	return &filtered, nil
}

// writeWitnesses lists witnesses, grouped under their owners when any has
// one: owners in name order, then those without an owner.
func writeWitnesses(w io.Writer, witnesses []Witness) {
	byOwner := map[string][]Witness{}
	var owners []string
	for _, witness := range witnesses {
		if _, seen := byOwner[witness.Owner]; !seen {
			owners = append(owners, witness.Owner)
		}
		byOwner[witness.Owner] = append(byOwner[witness.Owner], witness)
	}
	if len(owners) == 1 && owners[0] == "" {
		for _, witness := range witnesses {
			writeWitness(w, "    ", witness)
		}
		return
	}
	sort.Slice(owners, func(i, j int) bool {
		if (owners[i] == "") != (owners[j] == "") {
			return owners[j] == ""
		}
		return owners[i] < owners[j]
	})
	for _, owner := range owners {
		heading := owner
		if heading == "" {
			heading = noOwner
		}
		fmt.Fprintf(w, "    - owner %s:\n", heading)
		for _, witness := range byOwner[owner] {
			writeWitness(w, "      ", witness)
		}
	}
}

func writeWitness(w io.Writer, indent string, witness Witness) {
	args, _ := json.Marshal(witness.Arguments)
	fmt.Fprintf(w, "%s- %s%s\n", indent, args, waiverNote(witness.Waiver))
}
