package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/importer"
)

// importFromStdin reads data from stdin and imports it
func importFromStdin(cmd *cobra.Command) error {
	// Read stdin to a uniquely named temporary file: concurrent imports must not
	// share a path.
	tmpPath, err := importer.ReadStdinToTempFile()
	if err != nil {
		return errors.WrapError(errors.ErrCodeParsingFailed, "Failed to read from stdin", err)
	}
	defer os.Remove(tmpPath)

	// Detect format if not specified
	format := importFormat
	if format == "" {
		detectedFormat, err := importer.DetectFormatFromContent(tmpPath)
		if err != nil {
			return errors.WrapError(errors.ErrCodeParsingFailed, "Failed to detect format from stdin", err)
		}
		format = detectedFormat
	}

	// Give the staged data the declared format's extension. The temp name is
	// already unique, so the extended name is too.
	finalPath := tmpPath + importer.StdinExtension(format)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return errors.WrapError(errors.ErrCodeParsingFailed, "Failed to prepare stdin data", err)
	}
	defer os.Remove(finalPath)

	session, err := newImportSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.ctx = cmd.Context()
	session.limits = importIngestLimits()

	// Set origin to "stdin" if not specified
	origin := importOrigin
	if origin == "" {
		origin = "stdin"
	}

	result, err := importOneWithEnvelope(session.imp, session.options(finalPath, origin))
	if jsonOutput {
		if writeErr := writeImportJSON([]importOutcome{newImportOutcome("-", result, err)}); writeErr != nil {
			return writeErr
		}
	}
	if err != nil {
		return errors.WrapError(errors.ErrCodeParsingFailed, "Failed to import stdin data", err)
	}

	if !jsonOutput {
		displayImportResults(result)
	}
	return nil
}
