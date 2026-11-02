// Package gcppack embeds the GCP content pack. Its files are laid out as they
// are installed under .pudl/.
package gcppack

import "embed"

//go:embed schema populators
var Files embed.FS
