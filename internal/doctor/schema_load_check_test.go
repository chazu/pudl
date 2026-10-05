package doctor

import (
	"strings"
	"testing"

	"github.com/chazu/pudl/internal/validator"
)

func TestCheckSchemaLoading(t *testing.T) {
	const good = `package fam

#Base: {
	_pudl: {schema_type: "base", resource_type: "fam.base", identity_fields: ["id"]}
	id: string
}
`

	t.Run("loadable tree is ok", func(t *testing.T) {
		validator.ResetSharedLoaders()
		home := t.TempDir()
		t.Setenv("HOME", home)
		writeSchemaModule(t, home, map[string]string{"fam/base.cue": good})

		res := CheckSchemaLoading()
		if res.Status != "ok" {
			t.Fatalf("expected ok, got %q: %s (%s)", res.Status, res.Message, res.Details)
		}
	})

	t.Run("broken package and cycle are errors", func(t *testing.T) {
		validator.ResetSharedLoaders()
		home := t.TempDir()
		t.Setenv("HOME", home)
		writeSchemaModule(t, home, map[string]string{
			"fam/base.cue":      good,
			"broken/broken.cue": "package broken\n\nthis is not valid cue {{{\n",
			"cyc/cyc.cue": `package cyc

#A: {
	_pudl: {schema_type: "base", resource_type: "a", base_schema: "cyc.#B", identity_fields: ["id"]}
	id: string
	...
}

#B: {
	_pudl: {schema_type: "base", resource_type: "b", base_schema: "cyc.#A", identity_fields: ["id"]}
	id: string
	...
}
`,
		})

		res := CheckSchemaLoading()
		if res.Status != "error" {
			t.Fatalf("expected error, got %q: %s (%s)", res.Status, res.Message, res.Details)
		}
		for _, want := range []string{"schema package broken", "base_schema cycle: cyc.#A → cyc.#B → cyc.#A"} {
			if !strings.Contains(res.Details, want) {
				t.Errorf("expected details to contain %q, got: %s", want, res.Details)
			}
		}
	})
}
