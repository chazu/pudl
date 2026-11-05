package checkreport

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/chazu/pudl/internal/acute"
)

// noOwner heads the findings no owner was resolved for: an unowned resource,
// or a finding that references no resource or catalog entry.
const noOwner = "(none)"

// OwnerOf resolves the owner of the first resource ID, then catalog entry ID,
// it knows, or "" (ownership.Index.OwnerOf).
type OwnerOf func(resourceIDs, entryIDs []string) string

// Arguments a finding references its resource by, tried in order: resource
// IDs (or names that may be one), then catalog entry IDs.
var (
	resourceArgs = []string{"resource_id", "resource", "target", "name"}
	entryArgs    = []string{"entry_id", "evidence_id", "observation_id"}
)

// AttributeOwners sets the owner of each witness and each finding from the
// resource its arguments reference. A finding MatchFindings made in this
// evaluation is attributed from all of its arguments, recorded or not (past
// the witness limit, or waived and so listed last); any other finding from
// its recorded arguments.
func AttributeOwners(results []CheckResult, ownerOf OwnerOf) {
	for i := range results {
		for j := range results[i].Witnesses {
			results[i].Witnesses[j].Owner = ownerOfArgs(results[i].Witnesses[j].Arguments, ownerOf)
		}
		for j := range results[i].Findings {
			f := &results[i].Findings[j]
			if f.ResourceID != "" {
				if owner := ownerOf([]string{f.ResourceID}, nil); owner != "" {
					f.Owner = owner
					continue
				}
			}
			args := f.Arguments
			if f.refs != nil {
				args = f.refs
			}
			f.Owner = ownerOfArgs(args, ownerOf)
		}
	}
}

// stringArgs keeps a match's non-empty string arguments: the values a
// resource or owner can be resolved from.
func stringArgs(args map[string]any) map[string]any {
	refs := map[string]any{}
	for key, value := range args {
		if s, ok := value.(string); ok && s != "" {
			refs[key] = s
		}
	}
	return refs
}

func ownerOfArgs(args map[string]any, ownerOf OwnerOf) string {
	if len(args) == 0 {
		return ""
	}
	pick := func(keys []string) []string {
		var ids []string
		for _, key := range keys {
			if id, ok := args[key].(string); ok && id != "" {
				ids = append(ids, id)
			}
		}
		return ids
	}
	return ownerOf(pick(resourceArgs), pick(entryArgs))
}

// ForOwner returns a copy of the report limited to one owner's findings: each
// check keeps only the witnesses and findings owner owns, and a check that
// did not pass is left out when none remain. Passed and uncertain checks stay;
// drift, counts and change totals are unchanged and still cover every match.
func (r *ModelReport) ForOwner(owner string) *ModelReport {
	filtered := *r
	filtered.Checks = nil
	for _, c := range r.Checks {
		var witnesses []Witness
		for _, w := range c.Witnesses {
			if w.Owner == owner {
				witnesses = append(witnesses, w)
			}
		}
		var findings []Finding
		for _, f := range c.Findings {
			if f.Owner == owner {
				findings = append(findings, f)
			}
		}
		if !c.Passed && !c.Uncertain() && len(witnesses) == 0 && len(findings) == 0 {
			continue
		}
		c.Witnesses, c.Findings = witnesses, findings
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

// ownedLine is one rendered finding and its owner.
type ownedLine struct {
	owner, text string
}

// writeByOwner lists findings, grouped under their owners when any has one:
// owners in name order, then those without an owner. Without owners it is
// the flat list.
func writeByOwner(w io.Writer, lines []ownedLine) {
	byOwner := map[string][]string{}
	var owners []string
	for _, line := range lines {
		if _, seen := byOwner[line.owner]; !seen {
			owners = append(owners, line.owner)
		}
		byOwner[line.owner] = append(byOwner[line.owner], line.text)
	}
	if len(owners) == 1 && owners[0] == "" {
		for _, line := range lines {
			fmt.Fprintf(w, "    - %s\n", line.text)
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
		for _, text := range byOwner[owner] {
			fmt.Fprintf(w, "      - %s\n", text)
		}
	}
}

// writeWitnesses lists witnesses with their waiver notes, grouped by owner.
func writeWitnesses(w io.Writer, witnesses []Witness) {
	lines := make([]ownedLine, 0, len(witnesses))
	for _, witness := range witnesses {
		args, _ := json.Marshal(witness.Arguments)
		lines = append(lines, ownedLine{owner: witness.Owner, text: string(args) + waiverNote(witness.Waiver)})
	}
	writeByOwner(w, lines)
}
