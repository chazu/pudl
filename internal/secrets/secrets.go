// Package secrets detects likely secrets in decoded JSON-like data: known
// credential formats plus a conservative high-entropy heuristic. It reports
// where a secret appears and what kind it looks like — never the value or any
// part of it — so findings are safe to print, log and store.
//
// Detection only warns. Callers decide what a finding means; nothing here
// rewrites data. The package is pure: no database, schema or importer
// dependencies, so import, observe ingest, fact writes and doctor can all
// share it.
package secrets

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/fieldpath"
)

// Kind names the kind of secret a finding looks like.
type Kind string

// Detected kinds.
const (
	AWSAccessKeyID       Kind = "aws_access_key_id"
	AWSSecretAccessKey   Kind = "aws_secret_access_key"
	GCPServiceAccountKey Kind = "gcp_service_account_key"
	GCPAPIKey            Kind = "gcp_api_key"
	GitHubToken          Kind = "github_token"
	GitLabToken          Kind = "gitlab_token"
	SlackToken           Kind = "slack_token"
	PrivateKey           Kind = "private_key"
	JWT                  Kind = "jwt"
	URLCredentials       Kind = "url_credentials"
	HighEntropy          Kind = "high_entropy"
)

// RedactedValue is the placeholder schema redaction stores in place of a
// sensitive value (redact.Placeholder). Values equal to it are never reported.
const RedactedValue = "[REDACTED]"

// RootPath names the whole value when the scanned value is itself a string.
const RootPath = "(root)"

// Finding is one likely secret: where it is and what it looks like. Path uses
// field path syntax with [*] for array elements, so it can be pasted into
// `_pudl.sensitive_fields` or `_pudl.secret_scan_ignore` as is.
type Finding struct {
	Path string `json:"path"`
	Kind Kind   `json:"kind"`
}

// String renders the finding for messages: "path (kind)".
func (f Finding) String() string { return f.Path + " (" + string(f.Kind) + ")" }

// Options tunes a scan.
type Options struct {
	// Skip lists paths the caller already covers (declared sensitive, or
	// declared not secret). A finding at or below one of them is dropped.
	Skip []fieldpath.Path
}

// Scan returns the likely secrets in v, a decoded JSON value, sorted by path
// and kind with one finding per (path, kind).
func Scan(v any, opts Options) []Finding {
	s := scanner{opts: opts, seen: map[Finding]bool{}}
	s.walk(v, nil, "", nil)
	sort.Slice(s.out, func(i, j int) bool {
		if s.out[i].Path != s.out[j].Path {
			return s.out[i].Path < s.out[j].Path
		}
		return s.out[i].Kind < s.out[j].Kind
	})
	return s.out
}

// ScanJSON decodes raw JSON and scans it.
func ScanJSON(raw []byte, opts Options) ([]Finding, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return Scan(v, opts), nil
}

// Describe lists findings for messages, comma separated.
func Describe(findings []Finding) string {
	parts := make([]string, len(findings))
	for i, f := range findings {
		parts[i] = f.String()
	}
	return strings.Join(parts, ", ")
}

type scanner struct {
	opts Options
	seen map[Finding]bool
	out  []Finding
}

// walk visits v. segs are the rendered path segments so far; key is the
// nearest object key and parent the object holding v, if any.
func (s *scanner) walk(v any, segs []string, key string, parent map[string]any) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			s.walk(child, append(segs[:len(segs):len(segs)], fieldpath.FromKeys(k).String()), k, t)
		}
	case []any:
		var arraySegs []string
		if len(segs) == 0 {
			arraySegs = []string{"[*]"}
		} else {
			arraySegs = append(segs[:len(segs)-1:len(segs)-1], segs[len(segs)-1]+"[*]")
		}
		for _, child := range t {
			s.walk(child, arraySegs, key, nil)
		}
	case string:
		if kind, ok := classify(t, key, parent); ok {
			s.add(segs, kind)
		}
	}
}

func (s *scanner) add(segs []string, kind Kind) {
	path := RootPath
	if len(segs) > 0 {
		path = strings.Join(segs, ".")
	}
	f := Finding{Path: path, Kind: kind}
	if s.seen[f] || s.skipped(path) {
		return
	}
	s.seen[f] = true
	s.out = append(s.out, f)
}

// skipped reports whether path lies at or below a caller-covered path.
func (s *scanner) skipped(path string) bool {
	if len(s.opts.Skip) == 0 || path == RootPath {
		return false
	}
	p, err := fieldpath.Parse(path)
	if err != nil {
		return false
	}
	for _, skip := range s.opts.Skip {
		if p.HasPrefix(skip) {
			return true
		}
	}
	return false
}

// classify decides whether one string value looks like a secret. Known
// formats are checked first; the entropy heuristic is the fallback.
func classify(value, key string, parent map[string]any) (Kind, bool) {
	if value == RedactedValue || len(value) < 8 {
		return "", false
	}
	if kind, ok := knownFormat(value, key, parent); ok {
		return kind, true
	}
	if highEntropy(value, key) {
		return HighEntropy, true
	}
	return "", false
}
