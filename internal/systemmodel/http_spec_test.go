package systemmodel

import (
	"encoding/json"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const httpModel = `
stacks: #SystemModel & {
	name: "stacks"
	populate: {
		schema: "pudl/example.#Stack"
		base:   "https://api.example.com"
		headers: Accept: "application/vnd.example+json"
		auth: {env: "EXAMPLE_TOKEN", scheme: "token"}
		guard: {get: "/api/user", check: {in: _, ok: in.team == "readers", reason: "need a reader token"}}
		each: {
			get:  "/api/stacks?org=acme"
			list: "stacks"
			page: {next: "continuationToken", param: "continuationToken"}
			item: {in: _, out: "\(in.org)/\(in.name)"}
		}
		runs: [{
			get:     "/api/stacks/{{item}}/export"
			records: "deployment.resources"
			set: stack: "{{item}}"
			transform: {in: _, out: {urn: in.urn}}
		}]
		reject_keys: ["ciphertext"]
	}
}
`

func TestLoadModelReadsHTTPPopulate(t *testing.T) {
	m, err := loadModel([]byte(httpModel), "stacks")
	require.NoError(t, err)
	assert.Equal(t, KindHTTP, m.Populate.Kind())
	assert.Empty(t, m.Populate.Runs, "command runs are not filled from the HTTP shape")
	assert.Nil(t, m.Populate.Each)
	assert.Equal(t, "pudl/example.#Stack", m.Populate.Schema)

	h := m.Populate.HTTP
	require.NotNil(t, h)
	assert.Equal(t, "https://api.example.com", h.Base)
	assert.Equal(t, &HTTPAuth{Env: "EXAMPLE_TOKEN", Header: "Authorization", Scheme: "token"}, h.Auth)
	assert.Equal(t, []string{"ciphertext"}, h.RejectKeys)

	ok, reason := h.Guard.Check.Eval(map[string]any{"team": "writers"})
	assert.False(t, ok)
	assert.Equal(t, "need a reader token", reason)
	ok, _ = h.Guard.Check.Eval(map[string]any{"team": "readers"})
	assert.True(t, ok)

	item, err := h.Each.Item.String(map[string]any{"org": "acme", "name": "prod"})
	require.NoError(t, err)
	assert.Equal(t, "acme/prod", item)

	run := h.Runs[0].ForItem("acme/prod")
	assert.Equal(t, "/api/stacks/acme/prod/export", run.Get)
	assert.Equal(t, map[string]any{"stack": "acme/prod"}, run.Set)
	require.NotNil(t, run.Transform)

	template, err := NewTemplate(compileTemplate(t, httpModel, "stacks"), TemplateOrigin{})
	require.NoError(t, err)
	_, err = template.Model()
	require.NoError(t, err, "the guard check and the item mapping are exempt from concreteness")
	summary, err := template.Summary()
	require.NoError(t, err)
	assert.Equal(t, KindHTTP, summary.PopulateKind)
}

// The recorded model instance keeps the HTTP shape (not its CUE-only check,
// item and transforms) and still satisfies #SystemModel.
func TestHTTPPopulateRecordsItsShape(t *testing.T) {
	m, err := loadModel([]byte(httpModel), "stacks")
	require.NoError(t, err)
	b, err := json.Marshal(m)
	require.NoError(t, err)
	var rec map[string]any
	require.NoError(t, json.Unmarshal(b, &rec))
	populate := rec["populate"].(map[string]any)
	assert.Equal(t, "https://api.example.com", populate["base"])
	assert.Equal(t, map[string]any{"get": "/api/user"}, populate["guard"])
	assert.NotContains(t, populate, "argv")

	ctx := cuecontext.New()
	schema := ctx.CompileString(SchemaCUE())
	require.NoError(t, schema.Err())
	recorded := schema.LookupPath(cue.ParsePath("#SystemModel")).Unify(ctx.CompileBytes(b))
	require.NoError(t, recorded.Validate(), "the recorded instance satisfies #SystemModel")
}

func TestLoadModelRejectsBadHTTPPopulates(t *testing.T) {
	model := func(populate string) string {
		return "m: #SystemModel & {name: \"m\", populate: {base: \"https://api.example.com\"\n" + populate + "}}"
	}
	// The schema rejects most shapes outright (CUE reports an empty
	// disjunction of the populate arms); the loader rejects the rest by name.
	for name, tc := range map[string]struct{ populate, err string }{
		"plain http base": {
			`base: "http://api.example.com", runs: [{get: "/x"}]`, "base",
		},
		"absolute run path": {
			`runs: [{get: "https://other.example.com/x"}]`, "empty disjunction",
		},
		"protocol-relative path": {
			`runs: [{get: "//other.example.com/x"}]`, "empty disjunction",
		},
		"guard without check": {
			`guard: {get: "/api/user"}, runs: [{get: "/x"}]`, "guard.check is required",
		},
		"placeholder without each": {
			`runs: [{get: "/x/{{item}}"}]`, "declares no `each`",
		},
		"each without placeholder": {
			`each: {get: "/list", list: "items"}, runs: [{get: "/x"}]`, "repeat it identically",
		},
		"auth header as static header": {
			`auth: {env: "TOKEN"}, headers: authorization: "x", runs: [{get: "/x"}]`, "headers may not set Authorization",
		},
		"method field": {
			`runs: [{get: "/x", method: "POST"}]`, "empty disjunction",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadModel([]byte(model(tc.populate)), "m")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.err)
		})
	}
}

func TestValidateItemKeepsInputsInsideThePath(t *testing.T) {
	for _, ok := range []string{"acme/app/prod", "a.b_c-d~e", "42"} {
		assert.NoError(t, ValidateItem(ok), ok)
	}
	for _, bad := range []string{"../admin", "a?b=c", "a#b", "a b", "a%2Fb", "x/../y", ""} {
		assert.Error(t, ValidateItem(bad), bad)
	}
}

func TestErrorPathsNamesPathsNotValues(t *testing.T) {
	tr, err := NewTransform(cuecontext.New().CompileString(`{in: _, out: {n: in.secret & int}}`))
	require.NoError(t, err)
	_, _, err = tr.Apply(map[string]any{"secret": "hunter2"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hunter2", "CUE quotes the value")
	described := ErrorPaths(err)
	assert.NotContains(t, described, "hunter2")
	assert.True(t, strings.HasPrefix(described, " at "), described)
}

const namedGuardModel = `
m: #SystemModel & {
	name: "m"
	populate: {
		base: "https://api.example.com"
		guard: {
			get: {user: "/api/user", roles: "/api/roles?mine=true"}
			check: {in: _, ok: in.user.org == "acme" && len(in.roles.items) == 1}
		}
		runs: [{get: "/x"}]
	}
}
`

func TestLoadModelReadsANamedGuard(t *testing.T) {
	m, err := loadModel([]byte(namedGuardModel), "m")
	require.NoError(t, err)
	g := m.Populate.HTTP.Guard
	require.NotNil(t, g)
	_, single := g.Single()
	assert.False(t, single)
	assert.Equal(t, []string{"roles", "user"}, g.Names(), "sent in name order")
	ok, _ := g.Check.Eval(map[string]any{
		"user":  map[string]any{"org": "acme"},
		"roles": map[string]any{"items": []any{"reader"}},
	})
	assert.True(t, ok)
	ok, _ = g.Check.Eval(map[string]any{"user": map[string]any{"org": "acme"}})
	assert.False(t, ok, "a check over a missing response is not passed")

	b, err := json.Marshal(m)
	require.NoError(t, err)
	var rec map[string]any
	require.NoError(t, json.Unmarshal(b, &rec))
	assert.Equal(t, map[string]any{"get": map[string]any{"user": "/api/user", "roles": "/api/roles?mine=true"}},
		rec["populate"].(map[string]any)["guard"])
	ctx := cuecontext.New()
	recorded := ctx.CompileString(SchemaCUE()).LookupPath(cue.ParsePath("#SystemModel")).Unify(ctx.CompileBytes(b))
	require.NoError(t, recorded.Validate())
}

func TestLoadModelRejectsBadNamedGuards(t *testing.T) {
	model := func(get string) string {
		return `m: #SystemModel & {name: "m", populate: {base: "https://api.example.com", runs: [{get: "/x"}],
			guard: {get: ` + get + `, check: {in: _, ok: true}}}}`
	}
	for name, tc := range map[string]struct{ get, err string }{
		"no requests":   {`{}`, "names no request"},
		"empty name":    {`{"": "/a", b: "/b"}`, "may not be empty"},
		"absolute path": {`{a: "https://other.example.com/a"}`, "empty disjunction"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadModel([]byte(model(tc.get)), "m")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.err)
		})
	}
}
