package factstore_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/pkg/factstore"
)

func secretArgs() (args, token string) {
	token = "ghp_" + strings.Repeat("aB3dE5fG7hJ9", 3)
	return `{"service":"ci","token":"` + token + `"}`, token
}

func TestAddFactAcceptsSecretsByDefault(t *testing.T) {
	s := openStore(t)
	args, _ := secretArgs()
	_, err := s.AddFact(factstore.Fact{Relation: "credential", Args: args})
	require.NoError(t, err, "rejection is opt-in")
}

func TestAddFactRejectsSecretsWhenOptedIn(t *testing.T) {
	s, err := factstore.Open(t.TempDir(), factstore.WithSecretRejection())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	args, token := secretArgs()
	_, err = s.AddFact(factstore.Fact{Relation: "credential", Args: args})
	var secretErr *factstore.SecretError
	require.True(t, errors.As(err, &secretErr), "got %v", err)
	assert.Equal(t, []factstore.SecretFinding{{Path: "token", Kind: "github_token"}}, secretErr.Findings)
	assert.NotContains(t, err.Error(), token[4:16], "the error never carries the value")

	facts, err := s.QueryFacts(factstore.FactFilter{Relation: "credential"})
	require.NoError(t, err)
	assert.Empty(t, facts, "a rejected fact is not written")

	_, err = s.AddFact(factstore.Fact{Relation: "credential", Args: `{"service":"ci","ref":"vault://ci/token"}`})
	assert.NoError(t, err)
}

func TestScanFactArgs(t *testing.T) {
	args, _ := secretArgs()
	findings, err := factstore.ScanFactArgs(args)
	require.NoError(t, err)
	assert.Len(t, findings, 1)
	_, err = factstore.ScanFactArgs("not json")
	assert.Error(t, err)
}
