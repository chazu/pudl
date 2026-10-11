package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

var modelNewCheck, modelNewExpect, modelNewMaxAge string
var modelNewEvidence []string

func newCheckModel(cmd *cobra.Command, name string) error {
	if modelNewPopulate != "" || len(modelNewInputs) > 0 {
		return fmt.Errorf("--check saves a checks-only model; omit --populate and --input")
	}
	if modelNewExpect != "empty" && modelNewExpect != "nonempty" {
		return fmt.Errorf("--expect must be empty or nonempty")
	}
	if modelNewMaxAge != "" {
		if _, err := parseCheckAge(modelNewMaxAge); err != nil {
			return err
		}
		if len(modelNewEvidence) == 0 {
			return fmt.Errorf("--max-age requires --evidence")
		}
	}
	check := map[string]any{"name": name, "query": modelNewCheck, "expect": modelNewExpect, "severity": "fail", "message": "Saved check for " + modelNewCheck}
	if len(modelNewEvidence) > 0 {
		check["evidence"] = modelNewEvidence
	}
	if modelNewMaxAge != "" {
		check["max_age"] = modelNewMaxAge
	}
	encoded, err := json.Marshal(check)
	if err != nil {
		return err
	}
	source := fmt.Sprintf("package models\nimport sm \"pudl.schemas/pudl/systemmodel@v0\"\n#%s: sm.#SystemModel & {name:%q,checks:[%s]}\n", modelDefinitionName(name), name, encoded)
	root, err := modelWriteRoot(false)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, "schema", "models")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, modelFileName(name))
	flags := os.O_CREATE | os.O_WRONLY | os.O_EXCL
	if modelNewForce {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0644)
	if err != nil {
		return err
	}
	_, writeErr := f.WriteString(source)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if jsonOutput {
		return printJSON(map[string]any{"path": path, "name": name, "query": modelNewCheck, "evidence": modelNewEvidence, "next_action": "pudl run " + name + " --detailed-exitcode"})
	}
	_, err = fmt.Fprintf(outw(), "Saved check: %s\nNext: pudl run %s --detailed-exitcode\n", path, name)
	return err
}

func init() {
	modelNewCmd.Flags().StringVar(&modelNewCheck, "check", "", "Save an existing query relation as a checks-only model")
	modelNewCmd.Flags().StringVar(&modelNewExpect, "expect", "empty", "Saved check expectation: empty or nonempty")
	modelNewCmd.Flags().StringArrayVar(&modelNewEvidence, "evidence", nil, "Saved check evidence selector (snapshot ID, model:name or scope:name; repeatable)")
	modelNewCmd.Flags().StringVar(&modelNewMaxAge, "max-age", "", "Maximum age for the saved check's selected observations")
}

func parseCheckAge(value string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("--max-age requires a positive duration")
	}
	return d, nil
}
