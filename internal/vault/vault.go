package vault

import (
	"fmt"
	"os"
	"strings"
)

// Vault provides secure secret storage and retrieval.
type Vault interface {
	Get(path string) (string, error)
	Set(path, value string) error
	List() ([]string, error)
	Close() error
}

// EnvVault resolves secrets from environment variables.
// Path "my/api/key" maps to env var PUDL_VAULT_MY_API_KEY.
type EnvVault struct{}

// NewEnvVault creates a new environment-variable-backed vault.
func NewEnvVault() *EnvVault {
	return &EnvVault{}
}

func (v *EnvVault) Get(path string) (string, error) {
	envVar := pathToEnvVar(path)
	val, ok := os.LookupEnv(envVar)
	if !ok {
		return "", fmt.Errorf("vault: secret %q not found (expected env var %s)", path, envVar)
	}
	return val, nil
}

func (v *EnvVault) Set(path, value string) error {
	envVar := pathToEnvVar(path)
	return os.Setenv(envVar, value)
}

func (v *EnvVault) List() ([]string, error) {
	var paths []string
	prefix := "PUDL_VAULT_"
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if strings.HasPrefix(parts[0], prefix) {
			suffix := parts[0][len(prefix):]
			path := strings.ToLower(strings.ReplaceAll(suffix, "_", "/"))
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func (v *EnvVault) Close() error {
	return nil
}

// pathToEnvVar converts a vault path to an environment variable name.
// "my/api/key" -> "PUDL_VAULT_MY_API_KEY"
func pathToEnvVar(path string) string {
	upper := strings.ToUpper(path)
	sanitized := strings.ReplaceAll(upper, "/", "_")
	return "PUDL_VAULT_" + sanitized
}
