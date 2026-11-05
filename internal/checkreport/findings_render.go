package checkreport

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/waiver"
)

const (
	// summaryTop is how many projects or owners a "by" line names.
	summaryTop = 5
	// listLimit bounds the findings a check lists one per line.
	listLimit = 10
	// sharedLimit bounds the resources the shared section lists.
	sharedLimit = 100
	// unknownAccount heads findings whose resource has no known account.
	unknownAccount = "(unknown)"
	// sharedHeading titles the model-level section of resources several
	// checks found.
	sharedHeading = "resources failing several checks"
)

// resourceKey identifies the resource a finding names, for grouping across
// checks: its resolved resource ID, else the value that named it.
func resourceKey(f Finding) string {
	if f.ResourceID != "" {
		return f.ResourceID
	}
	return f.Resource
}

// sharedResources returns the resources findings of more than one check name.
func sharedResources(checks []CheckResult) map[string]bool {
	checksOf := map[string]map[string]bool{}
	for _, c := range checks {
		if c.Passed || c.Expect != "empty" {
			continue
		}
		for _, f := range c.Findings {
			if !current(f) {
				continue
			}
			if key := resourceKey(f); key != "" {
				if checksOf[key] == nil {
					checksOf[key] = map[string]bool{}
				}
				checksOf[key][c.Name] = true
			}
		}
	}
	shared := map[string]bool{}
	for key, names := range checksOf {
		if len(names) > 1 {
			shared[key] = true
		}
	}
	return shared
}

// current reports whether a finding still matches (not resolved or unknown).
func current(f Finding) bool {
	return f.State != FindingResolved && f.State != FindingUnknown
}

// writeFindingSummary writes how a check's current findings spread over
// accounts (projects) and owners, when any finding has one.
func writeFindingSummary(w io.Writer, findings []Finding) {
	accounts, owners := map[string]int{}, map[string]int{}
	anyAccount, anyOwner, gcpOnly, typed := false, false, true, false
	for _, f := range findings {
		if !current(f) {
			continue
		}
		account := f.Account
		if account == "" {
			account = unknownAccount
		} else {
			anyAccount = true
		}
		accounts[account]++
		owner := f.Owner
		if owner == "" {
			owner = noOwner
		} else {
			anyOwner = true
		}
		owners[owner]++
		if f.Type != "" {
			typed = true
			gcpOnly = gcpOnly && strings.HasPrefix(f.Type, "gcp.")
		}
	}
	if anyAccount {
		label := "account"
		if typed && gcpOnly {
			label = "project"
		}
		fmt.Fprintf(w, "    - by %s (%d): %s\n", label, len(accounts), topCounts(accounts))
	}
	if anyOwner {
		fmt.Fprintf(w, "    - by owner: %s\n", topCounts(owners))
	}
}

// topCounts renders the largest counts, "name n", then how many are left.
func topCounts(counts map[string]int) string {
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if pi, pj := placeholder(names[i]), placeholder(names[j]); pi != pj {
			return pj // "(none)" and "(unknown)" last
		}
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	parts := make([]string, 0, summaryTop+1)
	for i, name := range names {
		if i == summaryTop {
			parts = append(parts, fmt.Sprintf("… %d more", len(names)-summaryTop))
			break
		}
		parts = append(parts, fmt.Sprintf("%s %d", name, counts[name]))
	}
	return strings.Join(parts, ", ")
}

func placeholder(name string) bool { return name == noOwner || name == unknownAccount }

// writeFindingList lists a check's findings one per line, leaving out those
// whose resource the shared section lists, at most listLimit. withState
// prefixes each with its state and first sighting.
func writeFindingList(w io.Writer, findings []Finding, shared map[string]bool, waivers map[string]*waiver.Ref, withState bool) {
	listed, more, sharedCount, unlisted := 0, 0, 0, 0
	for _, f := range findings {
		if shared[resourceKey(f)] && current(f) {
			sharedCount++
			continue
		}
		label := findingLabel(f)
		if label == "" {
			unlisted++
			continue
		}
		if listed == listLimit {
			more++
			continue
		}
		listed++
		fmt.Fprintf(w, "    - %s\n", findingLine(f, label, waivers, withState))
	}
	if more > 0 {
		fmt.Fprintf(w, "    - … %d more; --json lists them\n", more)
	}
	if sharedCount > 0 {
		fmt.Fprintf(w, "    - %d also fail other checks; see %s\n", sharedCount, sharedHeading)
	}
	if unlisted > 0 {
		fmt.Fprintf(w, "    - %d more finding(s) without recorded witnesses; --json lists their keys\n", unlisted)
	}
}

// findingLabel names a finding: its resolved resource's name, else its
// recorded arguments, else the value that names its resource.
func findingLabel(f Finding) string {
	if f.Name != "" {
		return shortResource(f.Name)
	}
	if f.Arguments != nil {
		args, _ := json.Marshal(f.Arguments)
		return string(args)
	}
	return shortResource(f.Resource)
}

func findingLine(f Finding, label string, waivers map[string]*waiver.Ref, withState bool) string {
	text := label
	if withState {
		text = f.State + " " + label
		if f.FirstSeen != nil {
			text += " (first seen " + f.FirstSeen.UTC().Format(time.RFC3339) + ")"
		}
	}
	if ref := waivers[f.Key]; ref != nil && current(f) {
		text += waiverNote(ref)
	} else if f.Waived {
		text += " — waived"
	}
	return text
}

// shortResource shortens a long path-like name to its last two segments.
func shortResource(s string) string {
	if len(s) <= 60 || !strings.Contains(s, "/") {
		return s
	}
	parts := strings.Split(strings.TrimRight(s, "/"), "/")
	if len(parts) < 3 {
		return s
	}
	return "…/" + strings.Join(parts[len(parts)-2:], "/")
}

// writeSharedResources lists each resource that several checks found once,
// with those checks, grouped by account (project).
func writeSharedResources(w io.Writer, checks []CheckResult, shared map[string]bool) {
	if len(shared) == 0 {
		return
	}
	type entry struct {
		label, account string
		checks         []string
	}
	byKey := map[string]*entry{}
	for _, c := range checks {
		if c.Passed || c.Expect != "empty" {
			continue
		}
		for _, f := range c.Findings {
			key := resourceKey(f)
			if !shared[key] || !current(f) {
				continue
			}
			e := byKey[key]
			if e == nil {
				e = &entry{label: shortResource(f.Name), account: f.Account}
				if e.label == "" {
					e.label = shortResource(f.Resource)
				}
				if e.label == "" {
					e.label = key
				}
				byKey[key] = e
			}
			name := c.Name
			if f.Waived {
				name += " (waived)"
			}
			if !slices.Contains(e.checks, name) {
				e.checks = append(e.checks, name)
			}
		}
	}
	byAccount := map[string][]*entry{}
	for _, e := range byKey {
		account := e.account
		if account == "" {
			account = unknownAccount
		}
		byAccount[account] = append(byAccount[account], e)
	}
	accounts := make([]string, 0, len(byAccount))
	for a := range byAccount {
		accounts = append(accounts, a)
	}
	sort.Slice(accounts, func(i, j int) bool {
		if pi, pj := placeholder(accounts[i]), placeholder(accounts[j]); pi != pj {
			return pj
		}
		if len(byAccount[accounts[i]]) != len(byAccount[accounts[j]]) {
			return len(byAccount[accounts[i]]) > len(byAccount[accounts[j]])
		}
		return accounts[i] < accounts[j]
	})
	fmt.Fprintf(w, "\n### %s (%d)\n", sharedHeading, len(byKey))
	written := 0
	for _, a := range accounts {
		entries := byAccount[a]
		sort.Slice(entries, func(i, j int) bool { return entries[i].label < entries[j].label })
		fmt.Fprintf(w, "  - %s (%d)\n", a, len(entries))
		for _, e := range entries {
			if written == sharedLimit {
				fmt.Fprintf(w, "  - … %d more; --json lists them\n", len(byKey)-sharedLimit)
				return
			}
			written++
			fmt.Fprintf(w, "    - %s: %s\n", e.label, strings.Join(e.checks, ", "))
		}
	}
}
