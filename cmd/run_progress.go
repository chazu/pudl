package cmd

import (
	"encoding/json"
	"time"

	"github.com/chazu/pudl/internal/checks"
)

var runProgressJSON bool

func emitCheckProgress(event checks.Progress) {
	emitRunProgress(event.Phase, event.Check, event.State)
}

// Progress uses stderr; stdout remains the command's single result document.
func emitRunProgress(phase, subject, state string) {
	if !runProgressJSON {
		return
	}
	_ = json.NewEncoder(errw()).Encode(map[string]any{"event_version": 1, "type": "progress", "phase": phase, "subject": subject, "state": state, "at": time.Now().UTC().Format(time.RFC3339Nano)})
}

func init() {
	runCmd.PersistentFlags().BoolVar(&runProgressJSON, "progress-json", false, "Emit phase progress events as JSON lines on stderr (alongside diagnostics)")
}
