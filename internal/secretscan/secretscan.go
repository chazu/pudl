// Package secretscan runs the likely-secret detector (internal/secrets) over
// records routed to schemas, outside what those schemas already cover: paths
// declared `_pudl.sensitive_fields` (redacted) and paths declared
// `_pudl.secret_scan_ignore` (known not to be secrets). Both are inherited
// along base_schema, like redaction.
//
// It only reports. Nothing here changes a record, its hash or its storage.
package secretscan

import (
	"sync"

	"github.com/chazu/pudl/internal/fieldpath"
	"github.com/chazu/pudl/internal/redact"
	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/secrets"
)

// Scanner resolves each schema's covered paths once and scans records.
type Scanner struct {
	src       redact.Source
	sensitive *redact.Registry
	mu        sync.Mutex
	ignored   map[string][]fieldpath.Path
}

// New builds a scanner over src. sensitive may be the caller's existing
// redaction registry; nil builds one over src. A nil src scans with no schema
// coverage.
func New(src redact.Source, sensitive *redact.Registry) *Scanner {
	if sensitive == nil {
		sensitive = redact.NewRegistry(src)
	}
	return &Scanner{src: src, sensitive: sensitive, ignored: map[string][]fieldpath.Path{}}
}

// Scan reports likely secrets in record outside the paths the named schemas
// cover. Pass the intended schema (if any) and the assigned schema; empty
// names are ignored.
func (s *Scanner) Scan(record any, schemas ...string) []secrets.Finding {
	return secrets.Scan(record, secrets.Options{Skip: s.Covered(schemas...)})
}

// Covered returns the paths the named schemas cover: their sensitive fields
// and their secret_scan_ignore entries. A broken sensitive declaration covers
// nothing here (redaction already refuses such records); a broken ignore list
// suppresses nothing and is reported as a schema load error.
func (s *Scanner) Covered(schemas ...string) []fieldpath.Path {
	var out []fieldpath.Path
	if paths, err := s.sensitive.For(schemas...); err == nil {
		out = append(out, paths...)
	}
	for _, schema := range schemas {
		if schema != "" {
			out = append(out, s.ignore(schemaname.Normalize(schema))...)
		}
	}
	return out
}

// ignore returns a schema's effective secret_scan_ignore paths: the union
// along its base_schema chain.
func (s *Scanner) ignore(schema string) []fieldpath.Path {
	if s.src == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if paths, ok := s.ignored[schema]; ok {
		return paths
	}
	var paths []fieldpath.Path
	for _, name := range s.src.GetInheritanceGraph().GetCascadeChain(schema) {
		meta, ok := s.src.GetSchemaMetadata(name)
		if !ok || meta.SecretScanIgnoreError != "" {
			continue
		}
		for _, raw := range meta.SecretScanIgnore {
			if p, err := fieldpath.Parse(raw); err == nil {
				paths = append(paths, p)
			}
		}
	}
	s.ignored[schema] = paths
	return paths
}
