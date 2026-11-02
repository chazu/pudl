package populate

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/fieldpath"
	"github.com/chazu/pudl/internal/systemmodel"
)

// enumerate runs the `each` command and returns the inputs it lists, in
// order, without duplicates. Any failure fails the populate: without its
// inputs there is no fan-out whose failed slice could be named.
func (x *executor) enumerate(each systemmodel.Enumeration, out string) ([]string, error) {
	fail := func(err error) ([]string, error) {
		return nil, fmt.Errorf("populate each (%s): %w", strings.Join(each.Argv, " "), err)
	}
	run := systemmodel.CommandRun{Argv: each.Argv, Dir: each.Dir}
	failure, err := x.attempt(run, out)
	if err != nil {
		return fail(err)
	}
	if failure != nil {
		if failure.Stderr != "" {
			return fail(fmt.Errorf("%s\nstderr (last %d bytes):\n%s", failure.Error, stderrTail, failure.Stderr))
		}
		return fail(fmt.Errorf("%s", failure.Error))
	}
	var selectPath *fieldpath.Path
	if each.Select != "" {
		path, err := fieldpath.Parse(each.Select)
		if err != nil {
			return fail(fmt.Errorf("select: %w", err))
		}
		selectPath = &path
	}
	items := make([]string, 0)
	seen := map[string]bool{}
	err = decodeJSONStream(out, func(v any) error {
		item, err := enumeratedItem(v, selectPath, len(items))
		if err != nil {
			return err
		}
		if !seen[item] {
			seen[item] = true
			items = append(items, item)
		}
		return nil
	})
	if err != nil {
		return fail(err)
	}
	return items, nil
}

// enumeratedItem is one listed input: a string, or the string (or number) at
// select in an object.
func enumeratedItem(v any, selectPath *fieldpath.Path, index int) (string, error) {
	if selectPath != nil {
		obj, ok := v.(map[string]any)
		if !ok {
			return "", fmt.Errorf("item %d is %T, not a JSON object to select from", index, v)
		}
		found, ok := selectPath.Lookup(obj)
		if !ok {
			return "", fmt.Errorf("item %d has no %s", index, selectPath)
		}
		v = found
	}
	var item string
	switch value := v.(type) {
	case string:
		item = value
	case json.Number:
		item = value.String()
	default:
		if selectPath == nil {
			return "", fmt.Errorf("item %d is %T, not a string (set select to pick a field)", index, v)
		}
		return "", fmt.Errorf("item %d %s is %T, not a string", index, selectPath, v)
	}
	if item == "" {
		return "", fmt.Errorf("item %d is empty", index)
	}
	return item, nil
}

// expandRuns repeats every run for each item, item by item, with the item
// placeholder substituted.
func expandRuns(runs []systemmodel.CommandRun, items []string) []systemmodel.CommandRun {
	out := make([]systemmodel.CommandRun, 0, len(runs)*len(items))
	for _, item := range items {
		for _, run := range runs {
			out = append(out, run.ForItem(item))
		}
	}
	return out
}
