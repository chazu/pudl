// Package gcpnetwork supplies the fixture and live GCP firewall workflow.
package gcpnetwork

import "embed"

//go:embed schema.cue rules.cue model.cue baseline.json fixed.json
var Files embed.FS
