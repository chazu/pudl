package vault

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathToEnvVar(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"my/api/key", "PUDL_VAULT_MY_API_KEY"},
		{"token", "PUDL_VAULT_TOKEN"},
		{"aws/secret/access/key", "PUDL_VAULT_AWS_SECRET_ACCESS_KEY"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, pathToEnvVar(tt.path))
	}
}

func TestEnvVaultGet(t *testing.T) {
	v := NewEnvVault()

	os.Setenv("PUDL_VAULT_MY_KEY", "secret123")
	defer os.Unsetenv("PUDL_VAULT_MY_KEY")

	val, err := v.Get("my/key")
	require.NoError(t, err)
	assert.Equal(t, "secret123", val)

	_, err = v.Get("nonexistent/key")
	assert.Error(t, err)
}

func TestEnvVaultSet(t *testing.T) {
	v := NewEnvVault()

	err := v.Set("test/path", "value456")
	require.NoError(t, err)
	defer os.Unsetenv("PUDL_VAULT_TEST_PATH")

	assert.Equal(t, "value456", os.Getenv("PUDL_VAULT_TEST_PATH"))
}

func TestEnvVaultList(t *testing.T) {
	v := NewEnvVault()

	os.Setenv("PUDL_VAULT_A_B", "1")
	os.Setenv("PUDL_VAULT_C_D", "2")
	defer os.Unsetenv("PUDL_VAULT_A_B")
	defer os.Unsetenv("PUDL_VAULT_C_D")

	paths, err := v.List()
	require.NoError(t, err)
	assert.Contains(t, paths, "a/b")
	assert.Contains(t, paths, "c/d")
}

func TestEnvVaultClose(t *testing.T) {
	v := NewEnvVault()
	assert.NoError(t, v.Close())
}
