// Package pulumipack embeds the Pulumi content pack. Its files are laid out
// as they are installed under .pudl/.
package pulumipack

import "embed"

//go:embed schema populators
var Files embed.FS
