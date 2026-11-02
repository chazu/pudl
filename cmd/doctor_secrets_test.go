package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/doctor"
)

const doctorVaultSchema = `package vault
#Login: {
 _pudl: {schema_type: "base", resource_type: "test.login", identity_fields: ["name"], sensitive_fields: ["password"], secret_scan_ignore: ["nonce"]}
 name: string
 ...
}
`

func TestDoctorReportsStoredSecretsWithoutValues(t *testing.T) {
	pudlDir := cliWorkspace(t)
	writeFile(t, filepath.Join(pudlDir, "schema", "user", "vault", "types.cue"), doctorVaultSchema)
	token := cliTestToken()
	jwt := "eyJ" + strings.Repeat("hbGciOiJ", 3) + ".eyJ" + strings.Repeat("zdWIiOiI", 4) + "." + strings.Repeat("SflKxwRJ", 5)
	catchall := filepath.Join(pudlDir, "data", "catchall.json")
	writeFile(t, catchall, `{"misc":"a","auth":{"token":"`+token+`"}}`)
	// Stored before password became sensitive, and a nonce declared not secret:
	// both are covered by the schema, so neither is reported here.
	login := filepath.Join(pudlDir, "data", "login.json")
	writeFile(t, login, `{"name":"svc","password":"`+token+`","nonce":"`+token+`"}`)

	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	addEntry(t, db, "aaaa000000000001", "pudl/core.#Item", catchall, false)
	addEntry(t, db, "aaaa000000000002", "user/vault.#Login", login, true)
	fact, err := db.AddFact(database.Fact{Relation: "session", Args: `{"host":"h1","bearer":"` + jwt + `"}`})
	require.NoError(t, err)
	// Projected facts copy scanned payload values; they are not scanned again.
	require.NoError(t, db.WithCatalogTx(func(tx *database.CatalogTx) error {
		_, _, err := tx.ReconcileProjection(database.ProjectionSource("r1"),
			[]database.Fact{{Relation: "login", Args: `{"token":"` + token + `"}`}}, database.ProjectionObservation)
		return err
	}))
	require.NoError(t, db.Close())

	r := runCLI(t, "doctor", "--json")
	require.NoError(t, r.Err, "findings warn without failing doctor: %s", r.Stdout)
	var report doctorReport
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &report), r.Stdout)
	require.True(t, report.OK)
	require.NotNil(t, report.Secrets)
	assert.Equal(t, 2, report.Secrets.Entries)
	assert.Equal(t, 1, report.Secrets.Facts)
	assert.True(t, report.Secrets.FactsScanned)
	require.Equal(t, 2, report.Secrets.Total, "%+v", report.Secrets.Findings)
	got := map[string]doctor.SecretFinding{}
	for _, f := range report.Secrets.Findings {
		got[f.Subject+" "+f.Path] = f
	}
	entry := got["entry auth.token"]
	assert.Equal(t, "aaaa000000000001", entry.ID)
	assert.NotEmpty(t, entry.Proquint)
	assert.Equal(t, "github_token", string(entry.Kind))
	assert.Contains(t, entry.Fix, "_pudl.sensitive_fields")
	assert.Contains(t, entry.Fix, "_pudl.secret_scan_ignore")
	factFinding := got["fact bearer"]
	assert.Equal(t, fact.ID, factFinding.ID)
	assert.Equal(t, "session", factFinding.Relation)
	assert.Equal(t, "jwt", string(factFinding.Kind))
	for _, secret := range []string{token[4:20], jwt[3:20]} {
		assert.NotContains(t, r.Stdout, secret, "no finding carries the value")
	}

	human := runCLI(t, "doctor")
	require.NoError(t, human.Err, human.Stdout)
	assert.Contains(t, human.Stdout, "Secrets: 2 likely secrets outside schema coverage (2 payloads, 1 facts scanned)")
	assert.Contains(t, human.Stdout, "auth.token (github_token)")
	assert.Contains(t, human.Stdout, "fact "+fact.ID+" (session): bearer (jwt)")
	assert.NotContains(t, human.Stdout, token[4:20])

	scoped := runCLI(t, "doctor", "--json", "--schema", "pudl/core.#Item")
	require.NoError(t, scoped.Err, scoped.Stdout)
	var scopedReport doctorReport
	require.NoError(t, json.Unmarshal([]byte(scoped.Stdout), &scopedReport))
	assert.Equal(t, 1, scopedReport.Secrets.Entries)
	assert.False(t, scopedReport.Secrets.FactsScanned, "scope flags select catalog entries; facts have neither")
	assert.Equal(t, 1, scopedReport.Secrets.Total)

	healthOnly := runCLI(t, "doctor", "--json", "--health-only")
	require.NoError(t, healthOnly.Err)
	assert.NotContains(t, healthOnly.Stdout, `"secrets"`)
}
