package cmd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/systemmodel"
)

func TestValidateRunFlags(t *testing.T) {
	cases := []struct {
		name    string
		f       runFlags
		wantErr string
	}{
		{"bare observe", runFlags{}, ""},
		// An unscoped replay would set-diff desired against every observation in
		// the catalog, so it must be refused before any query runs.
		{"from-catalog without scope", runFlags{fromCatalog: true}, "--from-catalog requires --catalog-scope"},
		{"from-catalog with blank scope", runFlags{fromCatalog: true, catalogScope: "   "}, "--from-catalog requires --catalog-scope"},
		{"from-catalog with scope", runFlags{fromCatalog: true, catalogScope: "snap-1"}, ""},
		{"scope without from-catalog", runFlags{catalogScope: "snap-1"}, "--catalog-scope requires --from-catalog"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRunFlags(tc.f)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestRunVerdict(t *testing.T) {
	cases := []struct {
		name   string
		report *checkreport.ModelReport
		want   string
	}{
		{"verified drift clean -> clean", &checkreport.ModelReport{Drift: &acute.ModelDriftResult{Clean: true, Verified: true}}, "clean"},
		{"verified drift dirty", &checkreport.ModelReport{Drift: &acute.ModelDriftResult{Clean: false, Verified: true}}, "drifted"},
		// A catalog replay observed nothing, so it must not overwrite the verdict of
		// the model's last real observation — in either direction.
		{"replayed clean drift writes nothing", &checkreport.ModelReport{Drift: &acute.ModelDriftResult{Clean: true}}, ""},
		{"replayed dirty drift writes nothing", &checkreport.ModelReport{Drift: &acute.ModelDriftResult{Clean: false}}, ""},
		{"uncertain inventory is unknown", &checkreport.ModelReport{Drift: &acute.ModelDriftResult{Uncertain: true}}, "unknown"},
		{"pure populate has no verdict", &checkreport.ModelReport{Populate: &checkreport.Populate{}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, runVerdict(c.report))
		})
	}
}

// End-to-end over a real catalog: a clean run marks the model clean, and the
// run row records the same verdict.
func TestCleanRun_ModelRowClean(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db, err := database.NewCatalogDB(config.GetPudlDir())
	require.NoError(t, err)
	target := systemmodel.InstanceTarget("mymodel")
	entryType := "model-instance"
	require.NoError(t, db.AddEntry(database.CatalogEntry{
		ID: "model-row", StoredPath: "m.json", MetadataPath: "m.meta",
		ImportTimestamp: time.Now(), Format: "json", Origin: "t",
		Schema: "pudl/core.#SystemModel", Target: &target, EntryType: &entryType,
	}))
	db.Close()

	runID := "run_test_clean"
	verdict := runVerdict(&checkreport.ModelReport{Drift: &acute.ModelDriftResult{Clean: true, Verified: true}})
	cat := newRunCatalog(config.GetPudlDir())
	defer cat.Close()
	startRunRecord(cat, runID, "mymodel", "observe-only", false)
	persistRunStatus(cat, "mymodel", verdict, false)
	finishRunRecord(cat, runID, runFinishState{verdict: verdict}, nil, false)

	db2, err := database.NewCatalogDB(config.GetPudlDir())
	require.NoError(t, err)
	defer db2.Close()
	statuses, err := db2.GetTargetStatuses()
	require.NoError(t, err)
	got := map[string]string{}
	for _, s := range statuses {
		got[s.Target] = s.Status
	}
	assert.Equal(t, "clean", got[target], "a verified ∅ marks the model clean")

	run, err := db2.GetRun(runID)
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, "clean", run.Verdict)
	assert.True(t, run.Finished(), "the run is recorded terminal")
}
