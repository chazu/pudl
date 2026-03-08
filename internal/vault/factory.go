package vault

import "fmt"

// New creates a Vault instance based on the backend name.
// Supported backends: "env" (default), "file".
func New(backend string, pudlDir string) (Vault, error) {
	switch backend {
	case "", "env":
		return NewEnvVault(), nil
	case "file":
		return NewFileVault(pudlDir)
	default:
		return nil, fmt.Errorf("vault: unknown backend %q (supported: env, file)", backend)
	}
}
