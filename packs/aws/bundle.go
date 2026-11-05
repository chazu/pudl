// Package awspack embeds the AWS content pack. Its files are laid out as they
// are installed under .pudl/.
package awspack

import "embed"

//go:embed schema populators
var Files embed.FS
