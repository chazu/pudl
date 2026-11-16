// Package argocdpack embeds the Argo CD content pack. Its files are laid out
// as they are installed under .pudl/.
package argocdpack

import "embed"

//go:embed schema populators
var Files embed.FS
