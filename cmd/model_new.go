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
var modelNewInputs []string
var modelNewForce bool

// parsePluginSpec accepts the intentionally explicit ad-hoc/scaffold form
// plugin:<name>. Keeping the prefix makes accidental use of a model name in
// the populate slot an actionable error.
func parsePluginSpec(spec string) (string, error) {
	name, ok := strings.CutPrefix(strings.TrimSpace(spec), "plugin:")
	name = strings.TrimSpace(name)
	if !ok || name == "" {
		return "", fmt.Errorf("populate must use plugin:<name> (got %q)", spec)
	}
	return name, nil
}

// parseKeyValueInputs turns repeated --input key=value flags into the open
// #PluginObserve input object. Values that are valid JSON retain their type;
// other values remain strings, which is the ergonomic command-line default.
func parseKeyValueInputs(args []string) (map[string]any, error) {
	out := make(map[string]any, len(args))
	for _, arg := range args {
		key, value, ok := strings.Cut(arg, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("input must use key=value (got %q)", arg)
		}
		var decoded any
		if err := json.Unmarshal([]byte(value), &decoded); err == nil {
			out[key] = decoded
		} else {
			out[key] = value
		}
	}
	return out, nil
}

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

func renderModelScaffold(name, plugin string, input map[string]any) (string, error) {
	inputJSON, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal model input: %w", err)
	}
	return fmt.Sprintf(`package models

import sm "pudl.schemas/pudl/systemmodel@v0"

#%s: sm.#SystemModel & {
	name: %q
	populate: {
		plugin: %q
		input: %s
	}
}
`, modelDefinitionName(name), name, plugin, inputJSON), nil
}

func modelWriteRoot(global bool) (string, error) {
	if !global && wsPolicy != nil && wsPolicy.InWorkspace() {
		return wsPolicy.Workspace.PudlDir, nil
	}
	return config.GetPudlDir(), nil
}

func writeModelScaffold(name, plugin string, input map[string]any, global, force bool) (string, error) {
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
	src, err := renderModelScaffold(name, plugin, input)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		return "", fmt.Errorf("write model scaffold: %w", err)
	}
	return path, nil
}
