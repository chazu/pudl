package cmd

import (
	"os"

	"github.com/chazu/pudl/internal/systemmodel"
)

// adHocModel builds the model a `--populate command:<cmdline>` check runs. Its
// commands run from the current directory.
func adHocModel(spec string) (*systemmodel.SystemModel, string, error) {
	model, err := systemmodel.AdHocModel(spec)
	if err != nil {
		return nil, "", err
	}
	return model, mustCurrentDir(), nil
}

func mustCurrentDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}
