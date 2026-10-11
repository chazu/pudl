package cmd

import (
	"context"
	"errors"
	"io"
	"strings"

	pudlerrors "github.com/chazu/pudl/internal/errors"
)

type resultCounter struct {
	io.Writer
	written   int
	attempted bool
}

func (w *resultCounter) Write(p []byte) (int, error) {
	w.attempted = true
	n, err := w.Writer.Write(p)
	w.written += n
	return n, err
}

// executeCommand supplies one JSON error document only when the command failed
// before producing a result. Existing reports retain their command-specific
// shapes, and a broken output pipe is never retried with a second document.
func executeCommand(ctx context.Context, args []string) error {
	previous := rootCmd.OutOrStdout()
	count := &resultCounter{Writer: previous}
	rootCmd.SetOut(count)
	defer rootCmd.SetOut(previous)
	rootCmd.SetArgs(args)
	wantsJSON := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--json" || arg == "--json=true" {
			wantsJSON = true
		}
		if arg == "--json=false" {
			wantsJSON = false
		}
	}
	oldErrors, oldUsage := rootCmd.SilenceErrors, rootCmd.SilenceUsage
	if wantsJSON {
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
	}
	defer func() { rootCmd.SilenceErrors = oldErrors; rootCmd.SilenceUsage = oldUsage }()
	err := rootCmd.ExecuteContext(ctx)
	if err == nil || !wantsJSON || count.attempted {
		return err
	}
	code := "command_failed"
	var suggestions []string
	var typed *pudlerrors.PUDLError
	if errors.As(err, &typed) {
		code = string(typed.Code)
		suggestions = typed.Suggestions
	}
	if strings.Contains(err.Error(), "not found") {
		suggestions = append(suggestions, "Inspect active definitions with pudl config --paths --json")
	}
	if writeErr := printJSON(map[string]any{"report_version": 2, "ok": false, "error": map[string]any{"code": code, "message": err.Error(), "exit_code": exitCodeFor(err), "suggestions": suggestions}}); writeErr != nil {
		return writeErr
	}
	return err
}
