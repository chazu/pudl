package database

import (
	"strings"

	"github.com/chazu/pudl/internal/schemaname"
)

// SchemaFilterCondition returns the WHERE condition for a schema filter.
//
// A value naming a definition (it contains '#') is anchored: it matches the
// stored name exactly, as the tail after a '/' (so "aws.#EC2Instance" finds
// "pudl/aws.#EC2Instance"), or — for a bare "#Route" — as the tail after a
// '.'. Anchoring is what keeps "#Route" from also matching "#Router". Any
// other value (a package or a fragment) is an escaped substring match.
func SchemaFilterCondition(value string) (string, []any) {
	if !strings.Contains(value, "#") {
		return `schema LIKE ? ESCAPE '\'`, []any{"%" + EscapeLike(value) + "%"}
	}
	v := schemaname.Normalize(value)
	esc := EscapeLike(v)
	if strings.HasPrefix(v, "#") {
		return `(schema = ? OR schema LIKE ? ESCAPE '\')`, []any{v, "%." + esc}
	}
	return `(schema = ? OR schema LIKE ? ESCAPE '\')`, []any{v, "%/" + esc}
}

// EscapeLike escapes LIKE wildcards so the value matches literally under
// ESCAPE '\'.
func EscapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
