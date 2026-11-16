// Package k8spack embeds the Kubernetes content pack. Its files are laid out
// as they are installed under .pudl/.
package k8spack

import "embed"

//go:embed schema populators
var Files embed.FS
