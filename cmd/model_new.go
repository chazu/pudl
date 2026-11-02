package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/chazu/pudl/internal/config"
)

var modelNewPopulate string
var modelNewForce bool

var nonIdentifier = regexp.MustCompile(`[^[:alnum:]]+`)

func modelDefinitionName(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(part)
		b.WriteString(strings.ToUpper(string(runes[0])))
		b.WriteString(string(runes[1:]))
	}
	if b.Len() == 0 {
		return "Model"
	}
	if unicode.IsDigit([]rune(b.String())[0]) {
		return "Model" + b.String()
	}
	return b.String()
}

func modelFileName(name string) string {
	slug := strings.ToLower(strings.Trim(nonIdentifier.ReplaceAllString(name, "-"), "-"))
	if slug == "" {
		slug = "model"
	}
	return slug + ".cue"
}

func modelWriteRoot(global bool) (string, error) {
	if !global && wsPolicy != nil && wsPolicy.InWorkspace() {
		return wsPolicy.Workspace.PudlDir, nil
	}
	return config.GetPudlDir(), nil
}

// defaultScaffoldArgv is the command an unconfigured scaffold runs: it prints
// an empty record list, so the scaffold registers and runs before it is edited.
var defaultScaffoldArgv = []string{"echo", "[]"}

// writeModelScaffold writes a #CommandObserve model running argv, or the
// default scaffold command when argv is empty.
func writeModelScaffold(name string, argv []string, global, force bool) (string, error) {
	src, err := renderCommandScaffold(name, argv)
	if err != nil {
		return "", err
	}
	root, err := modelWriteRoot(global)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "schema", "models")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create model directory: %w", err)
	}
	path := filepath.Join(dir, modelFileName(name))
	if !force {
		if _, err := os.Stat(path); err == nil {
			return "", fmt.Errorf("model file already exists: %s (use --force to replace)", path)
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("check model file: %w", err)
		}
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		return "", fmt.Errorf("write model scaffold: %w", err)
	}
	return path, nil
}

// renderCommandScaffold scaffolds a #CommandObserve model running argv. An
// empty argv scaffolds the default command with a note to replace it.
func renderCommandScaffold(name string, argv []string) (string, error) {
	note := ""
	if len(argv) == 0 {
		argv = defaultScaffoldArgv
		note = "\t\t// Replace with a command that prints JSON records, e.g.\n" +
			"\t\t// [\"gcloud\", \"compute\", \"instances\", \"list\", \"--format=json\"]\n"
	}
	argvJSON, err := json.Marshal(argv)
	if err != nil {
		return "", fmt.Errorf("marshal command: %w", err)
	}
	return fmt.Sprintf(`package models

import sm "pudl.schemas/pudl/systemmodel@v0"

#%s: sm.#SystemModel & {
	name: %q
	populate: {
		// schema: "pudl/<package>.#<Definition>"  // route records like import --schema
%s		runs: [{argv: %s}]
	}
}
`, modelDefinitionName(name), name, note, argvJSON), nil
}
