package systemmodel

import (
	"fmt"

	"cuelang.org/go/cue"
)

// TemplateSummary is discovery metadata read from the authored value without
// decoding the whole model.
type TemplateSummary struct {
	PopulateKind PopulateKind
	DesiredCount int
	CheckCount   int
}

func (t *ModelTemplate) Summary() (TemplateSummary, error) {
	var summary TemplateSummary
	summary.PopulateKind = KindCommand
	if !t.value.LookupPath(cue.ParsePath("populate")).Exists() {
		summary.PopulateKind = KindNone
	}
	var err error
	if summary.DesiredCount, err = listLength(t.value, "desired"); err != nil {
		return summary, err
	}
	if summary.CheckCount, err = listLength(t.value, "checks"); err != nil {
		return summary, err
	}
	return summary, nil
}

func listLength(value cue.Value, path string) (int, error) {
	list := value.LookupPath(cue.ParsePath(path))
	if !list.Exists() {
		return 0, nil
	}
	iter, err := list.List()
	if err != nil {
		return 0, fmt.Errorf("%s must be a list: %w", path, err)
	}
	count := 0
	for iter.Next() {
		count++
	}
	return count, nil
}
