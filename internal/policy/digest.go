package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/format"
)

// digestOf hashes the effective CUE of a policy: every schema in its chain,
// exported without comments and without the `_pudl` metadata block, so
// editing a description or comment does not make stored results stale while
// any change to a constraint, rule attribute or the chain itself does.
func digestOf(members []member) (string, error) {
	h := sha256.New()
	for _, m := range members {
		node := m.value.Syntax(cue.Docs(false), cue.Attributes(true), cue.Optional(true), cue.Definitions(true))
		node = astutil.Apply(node, func(c astutil.Cursor) bool {
			if f, ok := c.Node().(*ast.Field); ok {
				if id, ok := f.Label.(*ast.Ident); ok && id.Name == "_pudl" {
					c.Delete()
					return false
				}
			}
			return true
		}, nil)
		src, err := format.Node(node)
		if err != nil {
			return "", fmt.Errorf("digest: format %s: %w", m.name, err)
		}
		fmt.Fprintf(h, "%s\n%s\n", m.name, src)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
