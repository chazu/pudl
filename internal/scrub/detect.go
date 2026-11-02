package scrub

import (
	"context"
	"fmt"
	"os"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/fieldpath"
	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/redact"
)

// finding is an entry holding plaintext at its sensitive paths.
type finding struct {
	entry    database.CatalogEntry
	redacted any      // the record with every sensitive path redacted
	paths    []string // the paths that held plaintext
}

// detect streams the entries in scope and returns those holding plaintext at
// a sensitive path, collecting the values into plan. Collection parents are
// left to planParents: each of their records is an item entry of its own.
func (s *Scrubber) detect(ctx context.Context, reg *redact.Registry, plan *Plan) ([]*finding, error) {
	var found []*finding
	check := func(entry database.CatalogEntry) error {
		if isCollection(entry) {
			return nil
		}
		f, err := s.detectEntry(entry, reg, plan)
		if err != nil {
			plan.Errors = append(plan.Errors, fmt.Sprintf("%s: %v", idgen.HashToProquint(entry.ID), err))
			return nil
		}
		if f != nil {
			found = append(found, f)
		}
		return nil
	}
	if s.Scope.EntryID != "" {
		entry, err := s.DB.GetEntryByProquint(s.Scope.EntryID)
		if err != nil {
			if entry, err = s.DB.GetEntry(s.Scope.EntryID); err != nil {
				return nil, fmt.Errorf("entry not found: %s", s.Scope.EntryID)
			}
		}
		return found, check(*entry)
	}
	filter := database.FilterOptions{Schema: s.Scope.Schema, Origin: s.Scope.Origin}
	return found, s.DB.EachEntryContext(ctx, filter, check)
}

// detectEntry checks one entry under the union of its intended and assigned
// schemas' sensitive paths, as import redacts it.
func (s *Scrubber) detectEntry(entry database.CatalogEntry, reg *redact.Registry, plan *Plan) (*finding, error) {
	meta := readMeta(entry.MetadataPath)
	paths, err := reg.For(metaString(meta, "schema_info", "intended_schema"), entry.Schema)
	if err != nil {
		return nil, fmt.Errorf("cannot evaluate sensitive fields: %w", err)
	}
	if len(paths) == 0 {
		return nil, nil
	}
	record, structured, err := loadRecord(entry)
	if err != nil {
		return nil, fmt.Errorf("load payload: %w", err)
	}
	hits := plaintextPaths(record, paths)
	if len(hits) == 0 {
		return nil, nil
	}
	if !structured {
		plan.Unsupported = append(plan.Unsupported, Subject{Kind: "entry", ID: entry.ID, Schema: entry.Schema, Paths: hits})
		return nil, nil
	}
	before, err := s.identityOf(entry.Schema, record)
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		for _, v := range p.LookupAll(record) {
			plan.values.collect(v)
		}
	}
	redact.Apply(record, paths)
	after, err := s.identityOf(entry.Schema, record)
	if err != nil {
		return nil, err
	}
	if before != after {
		return nil, fmt.Errorf("refused: redacting %v would change the entry's identity", hits)
	}
	return &finding{entry: entry, redacted: record, paths: hits}, nil
}

// plaintextPaths lists the paths with a value that is not the placeholder.
func plaintextPaths(record any, paths []fieldpath.Path) []string {
	var hits []string
	for _, p := range paths {
		for _, v := range p.LookupAll(record) {
			if v != redact.Placeholder {
				hits = append(hits, p.String())
				break
			}
		}
	}
	return hits
}

// identityOf is the canonical identity a record has under schema, or "" when
// the schema has no identity fields or they cannot be extracted.
func (s *Scrubber) identityOf(schema string, record any) (string, error) {
	if s.Inferrer == nil {
		return "", nil
	}
	meta, ok := s.Inferrer.GetSchemaMetadata(schema)
	if !ok || len(meta.IdentityFields) == 0 {
		return "", nil
	}
	values, err := identity.ExtractFieldValues(record, meta.IdentityFields)
	if err != nil || len(values) == 0 {
		return "", nil
	}
	return identity.CanonicalIdentityJSON(values)
}

// loadRecord decodes an entry's payload. structured is false for payloads the
// scrub cannot rewrite (non-JSON or compressed); they are decoded only so
// detection can report them.
func loadRecord(entry database.CatalogEntry) (any, bool, error) {
	if entry.Format != "json" || importer.DetectCompression(entry.StoredPath) != "none" {
		data, err := importer.DecodeStoredFile(entry.StoredPath, entry.Format)
		return data, false, err
	}
	raw, err := os.ReadFile(entry.StoredPath)
	if err != nil {
		return nil, false, err
	}
	record, err := idgen.DecodeJSONExact(raw)
	return record, true, err
}

// readMeta decodes an entry's .meta file, or returns nil.
func readMeta(path string) map[string]any {
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	decoded, err := idgen.DecodeJSONExact(raw)
	if err != nil {
		return nil
	}
	meta, _ := decoded.(map[string]any)
	return meta
}

func metaString(meta map[string]any, section, key string) string {
	inner, _ := meta[section].(map[string]any)
	value, _ := inner[key].(string)
	return value
}

func isCollection(entry database.CatalogEntry) bool {
	return entry.CollectionType != nil && *entry.CollectionType == "collection"
}

func isItem(entry database.CatalogEntry) bool {
	return entry.CollectionType != nil && *entry.CollectionType == "item"
}
