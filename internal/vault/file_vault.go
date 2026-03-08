package vault

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"filippo.io/age"
)

// FileVault stores secrets in an age-encrypted JSON file.
type FileVault struct {
	path       string
	passphrase string
	secrets    map[string]string
}

// NewFileVault opens or creates an age-encrypted vault file.
// Passphrase is read from PUDL_VAULT_PASSPHRASE env var.
func NewFileVault(pudlDir string) (*FileVault, error) {
	passphrase := os.Getenv("PUDL_VAULT_PASSPHRASE")
	if passphrase == "" {
		return nil, fmt.Errorf("vault: PUDL_VAULT_PASSPHRASE environment variable must be set for file vault")
	}

	vaultDir := filepath.Join(pudlDir, "vaults")
	if err := os.MkdirAll(vaultDir, 0700); err != nil {
		return nil, fmt.Errorf("vault: failed to create vault directory: %w", err)
	}

	fv := &FileVault{
		path:       filepath.Join(vaultDir, "default.age"),
		passphrase: passphrase,
		secrets:    make(map[string]string),
	}

	if err := fv.load(); err != nil {
		return nil, err
	}
	return fv, nil
}

func (fv *FileVault) Get(path string) (string, error) {
	val, ok := fv.secrets[path]
	if !ok {
		return "", fmt.Errorf("vault: secret %q not found", path)
	}
	return val, nil
}

func (fv *FileVault) Set(path, value string) error {
	fv.secrets[path] = value
	return fv.save()
}

func (fv *FileVault) List() ([]string, error) {
	paths := make([]string, 0, len(fv.secrets))
	for k := range fv.secrets {
		paths = append(paths, k)
	}
	sort.Strings(paths)
	return paths, nil
}

func (fv *FileVault) Close() error {
	return nil
}

// RotateKey re-encrypts the vault with a new passphrase.
func (fv *FileVault) RotateKey(newPassphrase string) error {
	fv.passphrase = newPassphrase
	return fv.save()
}

func (fv *FileVault) load() error {
	data, err := os.ReadFile(fv.path)
	if os.IsNotExist(err) {
		return nil // empty vault
	}
	if err != nil {
		return fmt.Errorf("vault: failed to read vault file: %w", err)
	}

	identity, err := age.NewScryptIdentity(fv.passphrase)
	if err != nil {
		return fmt.Errorf("vault: failed to create identity: %w", err)
	}

	reader, err := age.Decrypt(bytes.NewReader(data), identity)
	if err != nil {
		return fmt.Errorf("vault: failed to decrypt vault (wrong passphrase?): %w", err)
	}

	plaintext, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("vault: failed to read decrypted data: %w", err)
	}

	if err := json.Unmarshal(plaintext, &fv.secrets); err != nil {
		return fmt.Errorf("vault: failed to parse vault data: %w", err)
	}
	return nil
}

func (fv *FileVault) save() error {
	plaintext, err := json.Marshal(fv.secrets)
	if err != nil {
		return fmt.Errorf("vault: failed to marshal secrets: %w", err)
	}

	recipient, err := age.NewScryptRecipient(fv.passphrase)
	if err != nil {
		return fmt.Errorf("vault: failed to create recipient: %w", err)
	}
	recipient.SetWorkFactor(15)

	var buf bytes.Buffer
	writer, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return fmt.Errorf("vault: failed to create encryptor: %w", err)
	}

	if _, err := writer.Write(plaintext); err != nil {
		return fmt.Errorf("vault: failed to write encrypted data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("vault: failed to finalize encryption: %w", err)
	}

	if err := os.WriteFile(fv.path, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("vault: failed to write vault file: %w", err)
	}
	return nil
}
