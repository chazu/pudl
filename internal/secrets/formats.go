package secrets

import (
	"encoding/base64"
	"regexp"
	"strings"
)

// formats are credential shapes recognizable from the value alone. Each is
// anchored on a distinctive prefix so it does not fire on identifiers.
var formats = []struct {
	kind Kind
	re   *regexp.Regexp
}{
	// Long-term (AKIA) and temporary (ASIA) access key IDs: exactly 20
	// characters. IAM unique IDs (AIDA, AROA, ...) are not credentials.
	{AWSAccessKeyID, regexp.MustCompile(`(?:^|[^A-Z0-9])(?:AKIA|ASIA)[A-Z0-9]{16}(?:$|[^A-Z0-9])`)},
	{GCPAPIKey, regexp.MustCompile(`AIza[0-9A-Za-z_\-]{35}`)},
	{GitHubToken, regexp.MustCompile(`(?:^|[^A-Za-z0-9_])(?:gh[pousr]_[A-Za-z0-9]{36,255}|github_pat_[A-Za-z0-9_]{22,255})`)},
	{GitLabToken, regexp.MustCompile(`(?:^|[^A-Za-z0-9_])gl(?:pat|dt|rt|ptt|ft|cbt|soat|oas)-[0-9A-Za-z_\-]{20,}`)},
	{SlackToken, regexp.MustCompile(`(?:^|[^A-Za-z0-9])(?:xox[abposre]|xapp)-[0-9A-Za-z\-]{10,}`)},
	{PrivateKey, privateKeyRE},
	{JWT, regexp.MustCompile(`(?:^|[^A-Za-z0-9_\-])eyJ[A-Za-z0-9_\-]{8,}\.eyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}`)},
}

// privateKeyRE matches a PEM private-key header (RSA, EC, OpenSSH, PKCS#8,
// encrypted, PGP).
var privateKeyRE = regexp.MustCompile(`-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----`)

// urlCredentialsRE matches `scheme://user:password@`. The password group is
// checked against placeholders separately.
var urlCredentialsRE = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.\-]*://[^/\s:@]+:([^/\s@]+)@`)

// awsSecretRE is the shape of an AWS secret access key; it only counts beside
// a key name that says so (see knownFormat).
var awsSecretRE = regexp.MustCompile(`^[A-Za-z0-9/+]{40}$`)

// knownFormat checks value against the recognizable credential formats. key
// is the value's field name and parent the object holding it, for formats
// that need context.
func knownFormat(value, key string, parent map[string]any) (Kind, bool) {
	if privateKeyRE.MatchString(value) && isServiceAccount(parent) {
		return GCPServiceAccountKey, true
	}
	for _, f := range formats {
		if f.re.MatchString(value) {
			return f.kind, true
		}
	}
	if m := urlCredentialsRE.FindStringSubmatch(value); m != nil && !placeholderPassword(m[1]) {
		return URLCredentials, true
	}
	if awsSecretRE.MatchString(value) && strings.Contains(normalizeKey(key), "secret") && classCount(value) >= 3 {
		return AWSSecretAccessKey, true
	}
	if encodedPrivateKey(value) {
		return PrivateKey, true
	}
	return "", false
}

// isServiceAccount reports whether parent is a GCP service-account key file.
func isServiceAccount(parent map[string]any) bool {
	t, _ := parent["type"].(string)
	return t == "service_account"
}

// placeholderPassword reports passwords that are obviously not real: masks,
// template variables and angle-bracket stand-ins.
func placeholderPassword(p string) bool {
	if strings.Trim(p, "*xX•") == "" {
		return true
	}
	return strings.HasPrefix(p, "$") || strings.HasPrefix(p, "{{") || strings.HasPrefix(p, "%") ||
		(strings.HasPrefix(p, "<") && strings.HasSuffix(p, ">")) || p == RedactedValue
}

// encodedPrivateKey reports a base64 value that decodes to a PEM private key,
// as kubeconfig `client-key-data` and Kubernetes TLS secrets carry them.
func encodedPrivateKey(value string) bool {
	if len(value) < 64 || !strings.HasPrefix(value, "LS0t") { // base64("---")
		return false
	}
	decoded, ok := decodeBase64(value)
	return ok && privateKeyRE.Match(decoded)
}

// decodeBase64 decodes standard or URL-safe base64, padded or not.
func decodeBase64(value string) ([]byte, bool) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(value); err == nil {
			return b, true
		}
	}
	return nil, false
}
