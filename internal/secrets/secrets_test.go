package secrets

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/fieldpath"
)

// fake builds a token-shaped test value at run time, so no credential-looking
// literal sits in the source: n characters after prefix, cycling a mixed
// alphabet.
func fake(prefix string, n int) string {
	const alphabet = "aB3dE5fG7hJ9kL1mN2pQ4rS6tU8vW0xYz"
	var b strings.Builder
	b.WriteString(prefix)
	for i := 0; i < n; i++ {
		b.WriteByte(alphabet[(i*7+i/len(alphabet))%len(alphabet)])
	}
	return b.String()
}

func pem(label string) string {
	return "-----BEGIN " + label + "-----\n" + fake("MII", 60) + "\n-----END " + label + "-----\n"
}

func TestScanDetectsKnownFormats(t *testing.T) {
	jwt := "eyJ" + fake("", 20) + ".eyJ" + fake("", 30) + "." + fake("", 43)
	tests := []struct {
		name  string
		value any
		want  Kind
	}{
		{"aws access key id", "AKIA" + strings.ToUpper(fake("", 16)), AWSAccessKeyID},
		{"aws temporary key id", "ASIA" + strings.ToUpper(fake("", 16)), AWSAccessKeyID},
		{"gcp api key", "AIza" + fake("", 35), GCPAPIKey},
		{"github classic token", fake("ghp_", 36), GitHubToken},
		{"github fine-grained token", fake("github_pat_", 82), GitHubToken},
		{"gitlab token", fake("glpat-", 20), GitLabToken},
		{"slack bot token", "xoxb-" + fake("", 24), SlackToken},
		{"pem private key", pem("RSA PRIVATE KEY"), PrivateKey},
		{"openssh private key", pem("OPENSSH PRIVATE KEY"), PrivateKey},
		{"base64 pem private key", base64.StdEncoding.EncodeToString([]byte(pem("EC PRIVATE KEY"))), PrivateKey},
		{"jwt", jwt, JWT},
		{"bearer header", "Bearer " + jwt, JWT},
		{"connection string", "postgres://app:" + fake("", 12) + "@db.internal:5432/app", URLCredentials},
		{"high entropy", fake("", 40), HighEntropy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Scan(map[string]any{"config": map[string]any{"value": tt.value}}, Options{})
			require.Equal(t, []Finding{{Path: "config.value", Kind: tt.want}}, got)
		})
	}
}

func TestScanContextualFormats(t *testing.T) {
	saKey := map[string]any{"type": "service_account", "private_key_id": strings.Repeat("ab12", 10), "private_key": pem("PRIVATE KEY")}
	assert.Equal(t, []Finding{{Path: "private_key", Kind: GCPServiceAccountKey}}, Scan(saKey, Options{}))

	secret := fake("", 30) + "/" + fake("", 9)
	assert.Equal(t, []Finding{{Path: "SecretAccessKey", Kind: AWSSecretAccessKey}},
		Scan(map[string]any{"SecretAccessKey": secret}, Options{}))
	assert.NotContains(t, Scan(map[string]any{"Description": secret}, Options{}),
		Finding{Path: "Description", Kind: AWSSecretAccessKey},
		"a 40-character value is only an AWS secret beside a secret-named key")
}

func TestScanNeverReturnsTheValue(t *testing.T) {
	token := fake("ghp_", 36)
	record := map[string]any{"a": []any{map[string]any{"b": token}}, "c": "postgres://u:" + fake("", 10) + "@h/db"}
	out, err := json.Marshal(Scan(record, Options{}))
	require.NoError(t, err)
	assert.NotContains(t, string(out), token[4:12])
	assert.NotContains(t, string(out), "postgres")
	assert.JSONEq(t, `[{"path":"a[*].b","kind":"github_token"},{"path":"c","kind":"url_credentials"}]`, string(out))
}

func TestScanPathsAreFieldPaths(t *testing.T) {
	token := fake("ghp_", 36)
	record := map[string]any{
		"labels": map[string]any{"cloud.example.com/token": token},
		"env":    []any{map[string]any{"value": token}, map[string]any{"value": token}},
		"nested": []any{[]any{token}},
	}
	got := Scan(record, Options{})
	require.Len(t, got, 3, "one finding per path, however many elements match")
	for _, f := range got {
		_, err := fieldpath.Parse(f.Path)
		assert.NoError(t, err, f.Path)
	}
	assert.Equal(t, []string{"env[*].value", `labels."cloud.example.com/token"`, "nested[*][*]"},
		[]string{got[0].Path, got[1].Path, got[2].Path})
	assert.Equal(t, []Finding{{Path: "[*]", Kind: GitHubToken}}, Scan([]any{token}, Options{}))
	assert.Equal(t, []Finding{{Path: RootPath, Kind: GitHubToken}}, Scan(token, Options{}))
}

func TestScanSkipsCoveredAndRedactedPaths(t *testing.T) {
	token := fake("ghp_", 36)
	record := map[string]any{
		"env":      []any{map[string]any{"name": "TOKEN", "value": token}},
		"spec":     map[string]any{"auth": map[string]any{"token": token}},
		"redacted": RedactedValue,
		"other":    token,
	}
	skip := []fieldpath.Path{fieldpath.MustParse("env[*].value"), fieldpath.MustParse("spec.auth")}
	assert.Equal(t, []Finding{{Path: "other", Kind: GitHubToken}}, Scan(record, Options{Skip: skip}),
		"a skip path covers itself and everything below it")
}

func TestHighEntropyIgnoresIdentifiersAndDigests(t *testing.T) {
	notSecrets := map[string]string{
		"sha256":           "9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b",
		"uid":              "f6e5d4c3-b2a1-4098-8f7e-6d5c4b3a2918",
		"md5Hash":          "Xm2vGFQ3mXvR8l1k3vS0BQ==",
		"labelFingerprint": fake("", 30),
		"name":             "projects/acme-prod-381522/zones/us-central1-a/instances/web-1",
		"full_name":        "chazu/RepositoryName2024x",
		"node_id":          "MDEwOlJlcG9zaXRvcnkxMjk2MjY5",
		"description":      "A sentence with Mixed Case words and digits 12345 in it",
		"selfLink":         "https://www.googleapis.com/compute/v1/projects/p/global/firewalls/allow-ssh",
		"image":            "nginx@sha256:9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b",
		"instanceId":       "i-0a1b2c3d4e5f67890",
		"short":            "Ab3dE5fG7h",
		"camel":            "ServiceAccountTokenCreator",
		"masked":           "postgres://app:****@db:5432/app",
		"templated":        "postgres://app:${DB_PASSWORD}@db:5432/app",
	}
	for key, value := range notSecrets {
		assert.Empty(t, Scan(map[string]any{key: value}, Options{}), "%s=%s", key, value)
	}
}

func TestDescribe(t *testing.T) {
	assert.Equal(t, "a (jwt), b[*].c (high_entropy)", Describe([]Finding{{"a", JWT}, {"b[*].c", HighEntropy}}))
}

func TestScanJSON(t *testing.T) {
	got, err := ScanJSON([]byte(`{"token":"`+fake("ghp_", 36)+`","n":1}`), Options{})
	require.NoError(t, err)
	assert.Equal(t, []Finding{{Path: "token", Kind: GitHubToken}}, got)
	_, err = ScanJSON([]byte(`{`), Options{})
	assert.Error(t, err)
}
