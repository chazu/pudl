package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/lister"
	"github.com/chazu/pudl/internal/schemaname"
)

var listCountBy string

// listCountKeys are the fields `pudl list --count-by` groups by. Origins are
// grouped as list displays them, so collection items count under their
// collection rather than one group per item.
var listCountKeys = map[string]func(lister.ListEntry) string{
	"schema": func(e lister.ListEntry) string { return e.Schema },
	"origin": func(e lister.ListEntry) string { return formatOriginForDisplay(e.Origin) },
	"format": func(e lister.ListEntry) string { return e.Format },
}

// listCountKeyNames are the accepted --count-by values, in help order.
var listCountKeyNames = []string{"schema", "origin", "format"}

// bookkeepingSchemas are the types pudl writes for its own records (observe
// snapshots, system models and legacy run artifacts) rather than observed
// resources. Counts flag them so they do not crowd out resource types.
var bookkeepingSchemas = map[string]bool{}

func init() {
	for _, name := range []string{
		"pudl/core.#ObserveSnapshot",
		"pudl/systemmodel.#SystemModel",
		"pudl/mu.#DriftObservation",
		"pudl/mu.#Manifest",
		"pudl/mu.#ManifestAction",
		"pudl/mu.#PlanOutput",
	} {
		bookkeepingSchemas[schemaname.Normalize(name)] = true
	}
}

// isBookkeepingSchema reports whether schema names one of pudl's own record types.
func isBookkeepingSchema(schema string) bool {
	return bookkeepingSchemas[schemaname.Normalize(schema)]
}

// listCountGroup is one value of the counted field and its entry count.
type listCountGroup struct {
	Value       string
	Count       int
	Bookkeeping bool
}

// listCountOutput is `pudl list --count-by FIELD --json`. Each group is an
// object keyed by the counted field, e.g. {"schema": "...", "count": 3}.
type listCountOutput struct {
	CountBy      string           `json:"count_by"`
	Groups       []map[string]any `json:"groups"`
	TotalEntries int              `json:"total_entries"`
	TotalMatched int              `json:"total_matched"`
}

// countListEntries groups entries by key, most common first, ties by value.
func countListEntries(entries []lister.ListEntry, field string) []listCountGroup {
	key := listCountKeys[field]
	counts := map[string]int{}
	for _, e := range entries {
		counts[key(e)]++
	}
	groups := make([]listCountGroup, 0, len(counts))
	for value, n := range counts {
		groups = append(groups, listCountGroup{
			Value:       value,
			Count:       n,
			Bookkeeping: field == "schema" && isBookkeepingSchema(value),
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Count != groups[j].Count {
			return groups[i].Count > groups[j].Count
		}
		return groups[i].Value < groups[j].Value
	})
	return groups
}

// runListCount counts every entry matching filters (capped by --limit) by
// field and prints the groups.
func runListCount(l *lister.Lister, filters lister.FilterOptions, displayOpts lister.DisplayOptions, field string) error {
	if listCountKeys[field] == nil {
		return errors.NewInputError(
			fmt.Sprintf("Cannot count by %q", field),
			"Use --count-by "+strings.Join(listCountKeyNames, ", --count-by "))
	}
	displayOpts.All = true
	displayOpts.Page = 1
	results, err := l.ListData(filters, displayOpts)
	if err != nil {
		return err
	}
	groups := countListEntries(results.Entries, field)
	if jsonOutput {
		return printJSON(listCountJSON(field, groups, results))
	}
	if len(groups) == 0 {
		fmt.Fprintln(outw(), "No data found matching the specified criteria.")
		return nil
	}
	printListCount(field, groups, len(results.Entries))
	return nil
}

func listCountJSON(field string, groups []listCountGroup, results *lister.ListResults) listCountOutput {
	out := listCountOutput{
		CountBy:      field,
		Groups:       make([]map[string]any, len(groups)),
		TotalEntries: len(results.Entries),
		TotalMatched: results.TotalMatched,
	}
	for i, g := range groups {
		group := map[string]any{field: g.Value, "count": g.Count}
		if g.Bookkeeping {
			group["bookkeeping"] = true
		}
		out.Groups[i] = group
	}
	return out
}

// printListCount prints a COUNT/FIELD table; pudl's bookkeeping schemas
// follow the resource types in their own section.
func printListCount(field string, groups []listCountGroup, total int) {
	var resources, bookkeeping []listCountGroup
	for _, g := range groups {
		if g.Bookkeeping {
			bookkeeping = append(bookkeeping, g)
		} else {
			resources = append(resources, g)
		}
	}
	width := len("COUNT")
	if w := len(fmt.Sprint(groups[0].Count)); w > width {
		width = w
	}
	fmt.Fprintf(outw(), "%d entries in %d %ss\n\n", total, len(groups), field)
	fmt.Fprintf(outw(), "%*s  %s\n", width, "COUNT", strings.ToUpper(field))
	for _, g := range resources {
		fmt.Fprintf(outw(), "%*d  %s\n", width, g.Count, g.Value)
	}
	if len(bookkeeping) > 0 {
		if len(resources) > 0 {
			fmt.Fprintln(outw())
		}
		fmt.Fprintln(outw(), "pudl bookkeeping:")
		for _, g := range bookkeeping {
			fmt.Fprintf(outw(), "%*d  %s\n", width, g.Count, g.Value)
		}
	}
}
