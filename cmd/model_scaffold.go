package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
)

var (
	scaffoldCategory string
	scaffoldMethods  string
	scaffoldSockets  string
	scaffoldAuth     string
)

var modelScaffoldCmd = &cobra.Command{
	Use:   "scaffold <name>",
	Short: "Generate model boilerplate",
	Long: `Generate a model CUE file, method stubs, and definition template.

Creates:
  models/<name>/<name>.cue       — Model CUE file
  methods/<name>/<method>.clj    — Method stub files
  definitions/<name>_def.cue     — Definition template

Flags:
  --category    Model category (default: custom)
  --methods     Comma-separated method names (default: list,create)
  --sockets     Comma-separated socket specs as name:direction (e.g., api_url:input,resource_id:output)
  --auth        Auth method: bearer, sigv4, basic, custom

Examples:
    pudl model scaffold myservice
    pudl model scaffold myservice --category compute --methods list,create,delete
    pudl model scaffold myservice --sockets api_url:input,resource_id:output --auth bearer`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runModelScaffoldCommand(args[0])
	},
}

func init() {
	modelCmd.AddCommand(modelScaffoldCmd)
	modelScaffoldCmd.Flags().StringVar(&scaffoldCategory, "category", "custom", "Model category")
	modelScaffoldCmd.Flags().StringVar(&scaffoldMethods, "methods", "list,create", "Comma-separated method names")
	modelScaffoldCmd.Flags().StringVar(&scaffoldSockets, "sockets", "", "Comma-separated socket specs (name:direction)")
	modelScaffoldCmd.Flags().StringVar(&scaffoldAuth, "auth", "", "Auth method (bearer, sigv4, basic, custom)")
}

func runModelScaffoldCommand(name string) error {
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	methods := strings.Split(scaffoldMethods, ",")
	for i := range methods {
		methods[i] = strings.TrimSpace(methods[i])
	}

	type socketSpec struct {
		Name      string
		Direction string
	}
	var sockets []socketSpec
	if scaffoldSockets != "" {
		for _, s := range strings.Split(scaffoldSockets, ",") {
			parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid socket spec %q, expected name:direction", s)
			}
			dir := parts[1]
			if dir != "input" && dir != "output" {
				return fmt.Errorf("invalid socket direction %q, must be 'input' or 'output'", dir)
			}
			sockets = append(sockets, socketSpec{Name: parts[0], Direction: dir})
		}
	}

	// Generate model CUE file
	modelDir := filepath.Join(cfg.SchemaPath, "models", name)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return fmt.Errorf("creating model directory: %w", err)
	}

	modelCue := generateModelCUE(name, scaffoldCategory, methods, sockets, scaffoldAuth)
	modelPath := filepath.Join(modelDir, name+".cue")
	if err := os.WriteFile(modelPath, []byte(modelCue), 0644); err != nil {
		return fmt.Errorf("writing model file: %w", err)
	}
	fmt.Printf("  Created model:      %s\n", modelPath)

	// Generate method stubs
	methodsDir := filepath.Join(cfg.SchemaPath, "methods", name)
	if err := os.MkdirAll(methodsDir, 0755); err != nil {
		return fmt.Errorf("creating methods directory: %w", err)
	}

	for _, m := range methods {
		stub := generateMethodStub(name, m)
		stubPath := filepath.Join(methodsDir, m+".clj")
		if err := os.WriteFile(stubPath, []byte(stub), 0644); err != nil {
			return fmt.Errorf("writing method stub: %w", err)
		}
		fmt.Printf("  Created method:     %s\n", stubPath)
	}

	// Generate definition template
	defDir := filepath.Join(cfg.SchemaPath, "definitions")
	if err := os.MkdirAll(defDir, 0755); err != nil {
		return fmt.Errorf("creating definitions directory: %w", err)
	}

	defCue := generateDefinitionTemplate(name, sockets)
	defPath := filepath.Join(defDir, name+"_def.cue")
	if err := os.WriteFile(defPath, []byte(defCue), 0644); err != nil {
		return fmt.Errorf("writing definition template: %w", err)
	}
	fmt.Printf("  Created definition: %s\n", defPath)

	fmt.Printf("\nScaffolded model %q with %d methods.\n", name, len(methods))
	return nil
}

func generateModelCUE(name, category string, methods []string, sockets interface{}, auth string) string {
	var b strings.Builder

	pkgName := strings.ReplaceAll(name, "-", "_")

	b.WriteString(fmt.Sprintf("package %s\n\n", pkgName))
	b.WriteString("import \"pudl.schemas/pudl/model\"\n\n")

	// Resource schema
	capName := strings.Title(name) //nolint:staticcheck
	b.WriteString(fmt.Sprintf("#%sResource: {\n", capName))
	b.WriteString("\tid:   string\n")
	b.WriteString("\tname: string\n")
	b.WriteString("\t...\n")
	b.WriteString("}\n\n")

	// Model definition
	b.WriteString(fmt.Sprintf("#%sModel: model.#Model & {\n", capName))
	b.WriteString(fmt.Sprintf("\tschema: #%sResource\n\n", capName))

	// Metadata
	b.WriteString("\tmetadata: model.#ModelMetadata & {\n")
	b.WriteString(fmt.Sprintf("\t\tname:        %q\n", name))
	b.WriteString(fmt.Sprintf("\t\tdescription: \"Manages %s resources\"\n", name))
	b.WriteString(fmt.Sprintf("\t\tcategory:    %q\n", category))
	b.WriteString("\t}\n\n")

	// Methods
	b.WriteString("\tmethods: {\n")
	for _, m := range methods {
		b.WriteString(fmt.Sprintf("\t\t%s: model.#Method & {\n", m))
		b.WriteString("\t\t\tkind:        \"action\"\n")
		b.WriteString(fmt.Sprintf("\t\t\tdescription: \"%s %s resources\"\n", strings.Title(m), name)) //nolint:staticcheck
		b.WriteString("\t\t\ttimeout:     \"1m\"\n")
		b.WriteString("\t\t}\n")
	}
	b.WriteString("\t}\n")

	// Sockets
	type socketSpec struct {
		Name      string
		Direction string
	}
	if ss, ok := sockets.([]socketSpec); ok && len(ss) > 0 {
		b.WriteString("\n\tsockets: {\n")
		for _, s := range ss {
			b.WriteString(fmt.Sprintf("\t\t%s: model.#Socket & {\n", s.Name))
			b.WriteString(fmt.Sprintf("\t\t\tdirection:   %q\n", s.Direction))
			b.WriteString("\t\t\ttype:        string\n")
			b.WriteString(fmt.Sprintf("\t\t\tdescription: \"%s socket\"\n", s.Name))
			b.WriteString("\t\t}\n")
		}
		b.WriteString("\t}\n")
	}

	// Auth
	if auth != "" {
		b.WriteString(fmt.Sprintf("\n\tauth: model.#AuthConfig & {\n\t\tmethod: %q\n\t}\n", auth))
	}

	b.WriteString("}\n")

	return b.String()
}

func generateMethodStub(modelName, methodName string) string {
	return fmt.Sprintf(`;; %s/%s — method implementation
;; Args map contains definition socket values, method inputs, and tags.

(defn run [args]
  ;; TODO: Implement %s logic
  {"status" "not_implemented"
   "method" "%s"
   "model"  "%s"})
`, modelName, methodName, methodName, methodName, modelName)
}

func generateDefinitionTemplate(name string, sockets interface{}) string {
	var b strings.Builder

	b.WriteString("package definitions\n\n")

	pkgName := strings.ReplaceAll(name, "-", "_")
	capName := strings.Title(name) //nolint:staticcheck

	b.WriteString(fmt.Sprintf("import \"pudl.schemas/models/%s\"\n\n", pkgName))
	b.WriteString(fmt.Sprintf("// Example definition for %s\n", name))
	b.WriteString(fmt.Sprintf("example_%s: %s.#%sModel & {\n", name, pkgName, capName))

	type socketSpec struct {
		Name      string
		Direction string
	}
	if ss, ok := sockets.([]socketSpec); ok && len(ss) > 0 {
		b.WriteString("\tsockets: {\n")
		for _, s := range ss {
			if s.Direction == "input" {
				b.WriteString(fmt.Sprintf("\t\t%s: {\n", s.Name))
				b.WriteString(fmt.Sprintf("\t\t\tvalue: \"TODO: set %s value\"\n", s.Name))
				b.WriteString("\t\t}\n")
			}
		}
		b.WriteString("\t}\n")
	}

	b.WriteString("}\n")

	return b.String()
}
