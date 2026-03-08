package vault

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileVaultRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("PUDL_VAULT_PASSPHRASE", "test-passphrase-123")
	defer os.Unsetenv("PUDL_VAULT_PASSPHRASE")

	// Create and populate vault
	fv, err := NewFileVault(tmpDir)
	require.NoError(t, err)

	err = fv.Set("my/api/key", "secret-value-1")
	require.NoError(t, err)

	err = fv.Set("db/password", "hunter2")
	require.NoError(t, err)

	// Verify in-memory reads
	val, err := fv.Get("my/api/key")
	require.NoError(t, err)
	assert.Equal(t, "secret-value-1", val)

	val, err = fv.Get("db/password")
	require.NoError(t, err)
	assert.Equal(t, "hunter2", val)

	// List
	paths, err := fv.List()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"db/password", "my/api/key"}, paths)

	// Reload from disk (new instance)
	fv2, err := NewFileVault(tmpDir)
	require.NoError(t, err)

	val, err = fv2.Get("my/api/key")
	require.NoError(t, err)
	assert.Equal(t, "secret-value-1", val)

	val, err = fv2.Get("db/password")
	require.NoError(t, err)
	assert.Equal(t, "hunter2", val)
}

func TestFileVaultMissingSecret(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("PUDL_VAULT_PASSPHRASE", "test-passphrase")
	defer os.Unsetenv("PUDL_VAULT_PASSPHRASE")

	fv, err := NewFileVault(tmpDir)
	require.NoError(t, err)

	_, err = fv.Get("nonexistent")
	assert.Error(t, err)
}

func TestFileVaultNoPassphrase(t *testing.T) {
	tmpDir := t.TempDir()
	os.Unsetenv("PUDL_VAULT_PASSPHRASE")

	_, err := NewFileVault(tmpDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PUDL_VAULT_PASSPHRASE")
}

func TestFileVaultRotateKey(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("PUDL_VAULT_PASSPHRASE", "old-passphrase")
	defer os.Unsetenv("PUDL_VAULT_PASSPHRASE")

	// Create vault with old passphrase
	fv, err := NewFileVault(tmpDir)
	require.NoError(t, err)

	err = fv.Set("secret/path", "my-secret")
	require.NoError(t, err)

	// Rotate to new passphrase
	err = fv.RotateKey("new-passphrase")
	require.NoError(t, err)

	// Reload with new passphrase
	os.Setenv("PUDL_VAULT_PASSPHRASE", "new-passphrase")
	fv2, err := NewFileVault(tmpDir)
	require.NoError(t, err)

	val, err := fv2.Get("secret/path")
	require.NoError(t, err)
	assert.Equal(t, "my-secret", val)

	// Old passphrase should fail
	os.Setenv("PUDL_VAULT_PASSPHRASE", "old-passphrase")
	_, err = NewFileVault(tmpDir)
	assert.Error(t, err)
}

func TestFileVaultClose(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("PUDL_VAULT_PASSPHRASE", "test")
	defer os.Unsetenv("PUDL_VAULT_PASSPHRASE")

	fv, err := NewFileVault(tmpDir)
	require.NoError(t, err)
	assert.NoError(t, fv.Close())
}
