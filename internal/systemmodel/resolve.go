package systemmodel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"

	"github.com/chazu/pudl/internal/validator"
)

// ResolveTemplate finds a registered #SystemModel by name in the schema dirs,
// in order (the project's .pudl/schema first, then explicitly vendored
// packages; the first dir that has it wins). A model is any definition whose
// inherited _pudl.resource_type is "system_model" whose `name` (or short
// definition name) matches. The template keeps the authored cue.Value; its
// Origin names the directory it was loaded from and the pudl root that owns it.
func ResolveTemplate(dirs []string, name string) (*ModelTemplate, error) {
	var searched []string
	for _, dir := range dirs {
		if st, statErr := os.Stat(dir); statErr != nil || !st.IsDir() {
			continue
		}
		searched = append(searched, dir)
		found, err := ResolveTemplateIn(dir, name)
		if err != nil {
			return nil, err
		}
		if found != nil {
			return found, nil
		}
	}
	if len(searched) == 0 {
		return nil, fmt.Errorf("system model %q not found: no schema repository (run `pudl init`)", name)
	}
	return nil, fmt.Errorf("system model %q not found in %s — register it locally or vendor its definition package in .pudl/workspace.cue (inspect: pudl config --legacy-dependencies)", name, strings.Join(searched, ", "))
}

// ResolveTemplateIn searches one schema directory for the model name names. It
// returns nil when none matches there, and an error when the match is invalid
// or ambiguous.
func ResolveTemplateIn(dir, name string) (*ModelTemplate, error) {
	modules, err := validator.SharedLoader(dir).LoadAllModules()
	if err != nil {
		return nil, fmt.Errorf("load schemas in %s: %w", dir, err)
	}
	var match *ModelTemplate
	for _, mod := range modules {
		for schemaName, meta := range mod.Metadata {
			if meta.ResourceType != "system_model" {
				continue
			}
			value, ok := mod.Schemas[schemaName]
			if !ok {
				continue
			}
			template, templateErr := NewTemplate(value, TemplateOrigin{
				Definition: schemaName, SchemaName: schemaName, LoadDir: mod.LoadPath,
				PUDLRoot: filepath.Dir(dir),
			})
			if templateErr != nil {
				authoredName, _ := value.LookupPath(cue.ParsePath("name")).String()
				if authoredName == name || ShortDefName(schemaName) == name {
					return nil, fmt.Errorf("system model %q is invalid: %w", name, templateErr)
				}
				continue
			}
			if template.Name != name && ShortDefName(schemaName) != name {
				continue
			}
			if match != nil && match.Origin.SchemaName != template.Origin.SchemaName {
				return nil, fmt.Errorf("system model %q is ambiguous in %s (matches %s and %s)", name, dir, match.Origin.SchemaName, template.Origin.SchemaName)
			}
			match = template
		}
	}
	return match, nil
}

// ShortDefName strips the package prefix and leading '#' from a canonical
// schema name: "models.#GithubChazu" -> "GithubChazu".
func ShortDefName(canonical string) string {
	if i := strings.LastIndex(canonical, "#"); i >= 0 {
		return canonical[i+1:]
	}
	return canonical
}

// InstanceDefinition is the catalog `definition` key a model's instance row is
// recorded under.
func InstanceDefinition(name string) string { return "//models/" + name }

// InstanceTarget is the catalog `target`-column key of a model's instance row:
// InstanceDefinition without its "//" prefix, matching how the observe
// ingester stores it. Status reads and writes must use this form — keying on
// the definition (with "//") silently misses the row, so verdicts never
// persist.
func InstanceTarget(name string) string {
	return strings.TrimPrefix(InstanceDefinition(name), "//")
}
