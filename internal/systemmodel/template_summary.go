package systemmodel

import (
	"fmt"

	"cuelang.org/go/cue"
)

// TemplateSummary is concrete discovery metadata that remains inspectable even
// while desired/config fields depend on unresolved plain inputs.
type TemplateSummary struct {
	PopulateKind   PopulateKind
	PopulatePlugin string
	ConvergePlugin string
	DesiredCount   int
	CheckCount     int
	PluginNames    []string
}

func (t *ModelTemplate) Summary() (TemplateSummary, error) {
	var summary TemplateSummary
	populate := t.value.LookupPath(cue.ParsePath("populate"))
	if !populate.Exists() {
		return summary, fmt.Errorf("template %q has no populate arm", t.Name)
	}
	if ewe := populate.LookupPath(cue.ParsePath("eweSource")); ewe.Exists() {
		summary.PopulateKind = KindEweTarget
	} else {
		summary.PopulateKind = KindPluginObserve
		if plugin := populate.LookupPath(cue.ParsePath("plugin")); plugin.Exists() {
			summary.PopulatePlugin, _ = plugin.String()
		}
	}
	if converge := t.value.LookupPath(cue.ParsePath("converge.plugin")); converge.Exists() {
		summary.ConvergePlugin, _ = converge.String()
	}
	var err error
	if summary.DesiredCount, err = listLength(t.value, "desired"); err != nil {
		return summary, err
	}
	if summary.CheckCount, err = listLength(t.value, "checks"); err != nil {
		return summary, err
	}
	plugins := t.value.LookupPath(cue.ParsePath("plugins"))
	if plugins.Exists() {
		iter, iterErr := plugins.List()
		if iterErr != nil {
			return summary, fmt.Errorf("template %q plugins: %w", t.Name, iterErr)
		}
		for iter.Next() {
			name, nameErr := iter.Value().LookupPath(cue.ParsePath("name")).String()
			if nameErr == nil {
				summary.PluginNames = append(summary.PluginNames, name)
			}
		}
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
