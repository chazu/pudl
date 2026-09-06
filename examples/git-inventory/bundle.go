// Package gitinventory supplies the example assets embedded in the PUDL CLI.
package gitinventory

import "embed"

// Files contains the maintained model and inventories used by the tutorial.
//
//go:embed model.cue observe.py baseline.json changed.json
var Files embed.FS
