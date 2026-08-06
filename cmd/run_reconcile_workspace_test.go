package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/stretchr/testify/require"
)

type phaseConfigMu struct {
	muRoot        string
	observeConfig string
	buildConfig   string
}

func (m *phaseConfigMu) workspaceConfig() (string, error) {
	dirs, err := filepath.Glob(filepath.Join(m.muRoot, workspacePrefix+"*"))
	if err != nil {
		return "", err
	}
	if len(dirs) != 1 {
		return "", fmt.Errorf("found %d reconcile workspaces, want 1", len(dirs))
	}
	b, err := os.ReadFile(filepath.Join(dirs[0], "mu.cue"))
	return string(b), err
}

func (m *phaseConfigMu) Observe(_ string, _ string) ([]byte, error) {
	config, err := m.workspaceConfig()
	if err != nil {
		return nil, err
	}
	m.observeConfig = config
	return []byte(cleanObserve), nil
}

func (m *phaseConfigMu) Build(_ string, _ string, _ ...string) ([]byte, error) {
	config, err := m.workspaceConfig()
	if err != nil {
		return nil, err
	}
	m.buildConfig = config
	return []byte(`{"version":2,"targets":[],"actions":[]}`), nil
}

func TestSetupReconcileWorkspaceUsesAbsoluteDesiredSources(t *testing.T) {
	cat, muRoot := acceptanceFixture(t)
	workspace, err := setupReconcileWorkspace(cat, &scriptedMu{}, convergentModel(), muRoot, t.TempDir(), "run_sources", false)
	require.NoError(t, err)
	t.Cleanup(workspace.Cleanup)

	dirs, err := filepath.Glob(filepath.Join(muRoot, workspacePrefix+"*"))
	require.NoError(t, err)
	require.Len(t, dirs, 1)

	config, err := os.ReadFile(filepath.Join(dirs[0], "mu.cue"))
	require.NoError(t, err)
	require.Contains(t, string(config), filepath.Join(dirs[0], "desired_0.json"))
}

func TestReconcileWorkspaceKeepsConvergeSealedInputsOutOfObserve(t *testing.T) {
	cat, muRoot := acceptanceFixture(t)
	model := convergentModel()
	model.Converge.SealedInputs = map[string]systemmodel.SealedInput{
		"TOKEN": {Ref: "pass:apps/token", DeliveryMode: "env"},
	}
	mu := &phaseConfigMu{muRoot: muRoot}
	workspace, err := setupReconcileWorkspace(cat, mu, model, muRoot, t.TempDir(), "run_phases", false)
	require.NoError(t, err)
	t.Cleanup(workspace.Cleanup)

	_, err = workspace.observeDrift()
	require.NoError(t, err)
	_, err = workspace.planConverge()
	require.NoError(t, err)

	require.NotContains(t, mu.observeConfig, "sealed_inputs", "read-only observation must not resolve converge-only inputs")
	require.Contains(t, mu.buildConfig, `sealed_inputs: {"TOKEN":"pass:apps/token"}`)
	require.Contains(t, mu.buildConfig, `sealed_routing: "strict"`)
}

func TestPersistConvergeErrorIncludesErrorDetails(t *testing.T) {
	cat := newRunCatalog(t.TempDir())
	defer cat.Close()

	report := &RunReport{
		RunID: "run_converge_error",
		Model: "m",
		Mode:  "converge",
		OK:    true,
	}
	applyRunError(report, errors.New("desired manifest could not be observed"))
	require.True(t, persistRunReport(cat, report, false))

	db, err := cat.required()
	require.NoError(t, err)
	stored, err := db.GetRunReport("run_converge_error")
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Contains(t, string(stored.Report), "desired manifest could not be observed")
}
