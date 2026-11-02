package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/chazu/pudl/internal/systemmodel"
)

// adHocModel constructs the same model shape used by registered runs from a
// `--populate command:<cmdline>` spec. The model is intentionally not written
// to the schema repository; only its run/snapshot artifacts are durable.
func adHocModel(spec string) (*systemmodel.SystemModel, string, error) {
	parsed, err := parsePopulateSpec(spec)
	if err != nil {
		return nil, "", err
	}
	name := "adhoc-" + strings.ToLower(nonIdentifier.ReplaceAllString(filepath.Base(parsed.argv[0]), "-"))
	return &systemmodel.SystemModel{
		Name:     name,
		Populate: systemmodel.Populate{Runs: []systemmodel.CommandRun{{Argv: parsed.argv}}},
	}, mustCurrentDir(), nil
}

func mustCurrentDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}
