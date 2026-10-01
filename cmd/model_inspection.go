package cmd

import "github.com/chazu/pudl/internal/systemmodel"

var modelDiscover bool

type modelInspection struct {
	*systemmodel.SystemModel
	PluginDiscovery []modelPluginDiscovery `json:"plugin_discovery,omitempty"`
}

type modelPluginDiscovery struct {
	Name  string         `json:"name"`
	Info  map[string]any `json:"info,omitempty"`
	Error string         `json:"error,omitempty"`
}

type muPluginDiscovery func(string) (map[string]any, error)

func discoverModelPlugins(m *systemmodel.SystemModel, discover muPluginDiscovery) []modelPluginDiscovery {
	out := make([]modelPluginDiscovery, 0, len(m.Plugins))
	for _, plugin := range m.Plugins {
		info, err := discover(plugin.Name)
		entry := modelPluginDiscovery{Name: plugin.Name, Info: info}
		if err != nil {
			entry.Error = err.Error()
		}
		out = append(out, entry)
	}
	return out
}
