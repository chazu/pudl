package cmd

import (
	"encoding/json"
	"time"
)

var runProgressJSON bool

// Progress uses stderr; stdout remains the command's single result document.
func emitRunProgress(phase, subject, state string) {
	if !runProgressJSON {
		return
	}
	_ = json.NewEncoder(errw()).Encode(map[string]any{"event_version": 1, "type": "progress", "phase": phase, "subject": subject, "state": state, "at": time.Now().UTC().Format(time.RFC3339Nano)})
}
