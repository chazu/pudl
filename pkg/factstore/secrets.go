package factstore

import (
	"fmt"

	"github.com/chazu/pudl/internal/secrets"
)

// SecretFinding names a likely secret in fact args by path and kind (for
// example "token" and "github_token"). It never carries the value.
type SecretFinding = secrets.Finding

// SecretError is returned by AddFact on a store opened with
// WithSecretRejection when the fact's args contain likely secrets. Nothing is
// written.
type SecretError struct {
	Relation string
	Findings []SecretFinding
}

func (e *SecretError) Error() string {
	return fmt.Sprintf("fact %s: args contain likely secrets: %s (facts are not redacted; store a reference instead)",
		e.Relation, secrets.Describe(e.Findings))
}

// Option configures a Store at Open.
type Option func(*Store)

// WithSecretRejection makes Store.AddFact refuse a fact whose args contain a
// likely secret, returning *SecretError. Off by default. Writes made through
// Transact's Tx are not checked; call ScanFactArgs there.
func WithSecretRejection() Option {
	return func(s *Store) { s.rejectSecrets = true }
}

// ScanFactArgs reports likely secrets in a fact's JSON args. An args string
// that is not valid JSON is an error.
func ScanFactArgs(args string) ([]SecretFinding, error) {
	return secrets.ScanJSON([]byte(args), secrets.Options{})
}

// checkSecrets enforces WithSecretRejection for one fact. Invalid JSON is
// left for AddFact itself to reject.
func (s *Store) checkSecrets(f Fact) error {
	if !s.rejectSecrets {
		return nil
	}
	findings, err := ScanFactArgs(f.Args)
	if err != nil || len(findings) == 0 {
		return nil
	}
	return &SecretError{Relation: f.Relation, Findings: findings}
}
