package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/validator"
)

var modelListJSON bool

// ModelInfo is a registered #SystemModel definition discovered in the schema repo.
type ModelInfo struct {
	Name       string                   // the instance's display identity (`name:` field)
	DefName    string                   // short definition name, e.g. "GithubChazu"
	SchemaName string                   // canonical schema name, e.g. "models.#GithubChazu"
	Dir        string                   // module dir the definition was loaded from
	Model      *systemmodel.SystemModel // decoded model
	Template   *systemmodel.ModelTemplate
	Summary    systemmodel.TemplateSummary
}

// modelSearchDirs returns the schema dirs to search, project first (shadows global).
func modelSearchDirs() []string {
	// From the run's one workspace policy. This used to call workspace.Discover(".")
	// itself — a second answer to a question already resolved at startup, which
	// agreed only while nothing changed the working directory in between.
	if wsPolicy != nil {
		return wsPolicy.ModelSearchPaths
	}
	// Fallback for callers invoked without the Cobra lifecycle (tests).
	return []string{filepath.Join(config.GetPudlDir(), "schema")}
}

// listModels discovers all registered #SystemModel definitions across the schema
// dirs (project shadows global by model name). Returns models sorted by name and
// the dirs actually searched.
func listModels() ([]ModelInfo, []string, error) {
	var out []ModelInfo
	seen := map[string]bool{}
	var searched []string
	for _, dir := range modelSearchDirs() {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		searched = append(searched, dir)
		found, err := listModelsIn(dir)
		if err != nil {
			return nil, searched, err
		}
		for _, mi := range found {
			if seen[mi.Name] {
				continue // project already provided this model name
			}
			seen[mi.Name] = true
			out = append(out, mi)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, searched, nil
}

// listModelsIn returns every #SystemModel-derived definition in one schema dir.
// The abstract base #SystemModel (no concrete name) and incomplete defs decode-fail
// and are skipped — they are not runnable models.
func listModelsIn(dir string) ([]ModelInfo, error) {
	// Shared: resolveModel calls this once per model search dir, and several
	// commands call it in turn. The compile happens once per directory.
	loader := validator.SharedLoader(dir)
	modules, err := loader.LoadAllModules()
	if err != nil {
		return nil, fmt.Errorf("load schemas in %s: %w", dir, err)
	}
	var out []ModelInfo
	for _, mod := range modules {
		for schemaName, meta := range mod.Metadata {
			if meta.ResourceType != "system_model" {
				continue
			}
			val, ok := mod.Schemas[schemaName]
			if !ok {
				continue
			}
			template, templateErr := systemmodel.NewTemplate(val, systemmodel.TemplateOrigin{
				Definition: schemaName, SchemaName: schemaName, LoadDir: mod.LoadPath,
				PUDLRoot: filepath.Dir(dir),
			})
			if templateErr != nil {
				continue
			}
			summary, summaryErr := template.Summary()
			if summaryErr != nil {
				continue
			}
			// Preserve the existing concrete projection for commands that inspect
			// models without bindings. Bound models remain discoverable through their
			// retained template and summary until run-time elaboration.
			var m *systemmodel.SystemModel
			if len(template.Bindings) == 0 {
				m, _ = systemmodel.DecodeValue(val)
			}
			out = append(out, ModelInfo{
				Name:       template.Name,
				DefName:    shortDefName(schemaName),
				SchemaName: schemaName,
				Dir:        mod.LoadPath,
				Model:      m,
				Template:   template,
				Summary:    summary,
			})
		}
	}
	return out, nil
}

// convergeName returns the converge plugin name, or "-" for observe-only models.
func (mi ModelInfo) convergeName() string {
	if mi.Summary.ConvergePlugin != "" {
		return mi.Summary.ConvergePlugin
	}
	return "-"
}

// modelRunStatuses returns the latest run verdict keyed by model target
// (modelTargetKey(name)) — the catalog target key, best-effort — an empty map if the catalog is unavailable.
func modelRunStatuses() map[string]string {
	out := map[string]string{}
	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return out
	}
	defer db.Close()
	sts, err := db.GetTargetStatuses()
	if err != nil {
		return out
	}
	for _, s := range sts {
		out[s.Target] = s.Status
	}
	return out
}

var modelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered #SystemModel definitions",
	Long: `List every #SystemModel-derived definition registered in the schema
repository (project .pudl/schema shadows global ~/.pudl/schema), with its
populate kind, converge arm, and desired/check counts.

This is the static registry of runnable models — what 'pudl run <name>' can
resolve — independent of whether a model has been run yet.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		models, searched, err := listModels()
		if err != nil {
			return err
		}
		if modelListJSON {
			return printModelsJSON(models)
		}
		if len(searched) == 0 {
			fmt.Println("No schema repository found (run `pudl init`).")
			return nil
		}
		if len(models) == 0 {
			fmt.Println("No registered models. Register one as a #SystemModel-derived definition, then `pudl schema add`.")
			return nil
		}
		statuses := modelRunStatuses()
		fmt.Printf("Registered models (%d):\n\n", len(models))
		fmt.Printf("  %-24s %-9s %-12s %-8s %-7s %-10s %s\n", "NAME", "POPULATE", "CONVERGE", "DESIRED", "CHECKS", "STATUS", "DEFINITION")
		for _, mi := range models {
			status := statuses[modelTargetKey(mi.Name)]
			if status == "" {
				status = "-"
			}
			fmt.Printf("  %-24s %-9s %-12s %-8d %-7d %-10s %s\n",
				mi.Name,
				string(mi.Summary.PopulateKind),
				mi.convergeName(),
				mi.Summary.DesiredCount,
				mi.Summary.CheckCount,
				status,
				mi.SchemaName,
			)
		}
		return nil
	},
}

// modelSummary is the JSON shape for `pudl model list --json` / `show --json`.
type modelSummary struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
	Populate   string `json:"populate"`
	Converge   string `json:"converge,omitempty"`
	Desired    int    `json:"desired"`
	Checks     int    `json:"checks"`
	Status     string `json:"status,omitempty"`
}

func (mi ModelInfo) summary() modelSummary {
	s := modelSummary{
		Name:       mi.Name,
		Definition: mi.SchemaName,
		Populate:   string(mi.Summary.PopulateKind),
		Desired:    mi.Summary.DesiredCount,
		Checks:     mi.Summary.CheckCount,
	}
	if mi.Summary.ConvergePlugin != "" {
		s.Converge = mi.Summary.ConvergePlugin
	}
	return s
}

func printModelsJSON(models []ModelInfo) error {
	statuses := modelRunStatuses()
	out := make([]modelSummary, 0, len(models))
	for _, mi := range models {
		s := mi.summary()
		s.Status = statuses[modelTargetKey(mi.Name)]
		out = append(out, s)
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func init() {
	modelCmd.AddCommand(modelListCmd)
	modelListCmd.Flags().BoolVar(&modelListJSON, "json", false, "output as JSON")
}
