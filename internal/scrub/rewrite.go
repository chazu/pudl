package scrub

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/fieldpath"
	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/ingestprep"
	"github.com/chazu/pudl/internal/redact"
)

// valueMatch marks an artifact rewritten by value search, not by path.
const valueMatch = "value-match"

// rewrite is one entry whose payload or .meta file the scrub replaces.
type rewrite struct {
	entry      database.CatalogEntry
	kind       string   // document | item | collection | legacy | metadata
	paths      []string // redacted field paths, or valueMatch
	record     any      // the new record, for re-projection; nil for collections
	payload    []byte   // new stored bytes; nil when only the .meta changes
	newID      string
	newHash    string
	resourceID *string // recomputed identity-less resource ID, or nil
	meta       []byte  // new .meta bytes, or nil
	stored     string  // stored path after the scrub
	metaPath   string  // metadata path after the scrub
	mergeInto  bool    // newID already names another entry
}

// payloadRewrite prepares the new stored bytes of entry from record: what a
// fresh import or observe would store, and the content hash it derives.
func payloadRewrite(entry database.CatalogEntry, kind string, paths []string, record any) (*rewrite, error) {
	canonical, err := importer.EncodeRecord("", 0, record)
	if err != nil {
		return nil, err
	}
	rw := &rewrite{entry: entry, kind: kind, paths: paths, record: record, payload: canonical, newHash: hashHex(canonical)}
	if isItem(entry) && (entry.EntryType == nil || *entry.EntryType != "observe") {
		// Import items are stored indented and identified by canonical JSON.
		var indented bytes.Buffer
		if err := json.Indent(&indented, canonical, "", "  "); err != nil {
			return nil, err
		}
		rw.payload = indented.Bytes()
	}
	return rw, nil
}

// collectionRewrite re-encodes an import collection's file with the import
// framing, redacting the record at each index in redactions.
func collectionRewrite(entry database.CatalogEntry, redactions map[int][]fieldpath.Path) (*rewrite, error) {
	file, err := os.Open(entry.StoredPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	limits, err := ingestprep.Limits{}.Resolve()
	if err != nil {
		return nil, err
	}
	reader := bufio.NewReader(file)
	framing := "ndjson"
	if first, err := firstNonSpace(reader); err == nil && first == '[' {
		framing = "json-array"
	}
	var out bytes.Buffer
	if framing == "json-array" {
		out.WriteByte('[')
	}
	var paths []string
	emit := func(index int, raw json.RawMessage) error {
		record, err := idgen.DecodeJSONExact(raw)
		if err != nil {
			return fmt.Errorf("decode record %d: %w", index, err)
		}
		for _, hit := range plaintextPaths(record, redactions[index]) {
			paths = appendUnique(paths, hit)
		}
		redact.Apply(record, redactions[index])
		encoded, err := importer.EncodeRecord(framing, index, record)
		out.Write(encoded)
		return err
	}
	if framing == "ndjson" {
		_, err = ingestprep.NDJSON(reader, limits.RecordBytes, emit)
	} else {
		_, err = ingestprep.Array(reader, limits.RecordBytes, emit)
		out.WriteByte(']')
	}
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	return &rewrite{entry: entry, kind: "collection", paths: paths, payload: out.Bytes(), newHash: hashHex(out.Bytes())}, nil
}

// finish derives the new ID, resource ID, paths and .meta of rw.
func (s *Scrubber) finish(rw *rewrite, values *valueSet) {
	rw.newID, rw.stored, rw.metaPath = rw.entry.ID, rw.entry.StoredPath, rw.entry.MetadataPath
	oldHash := ""
	if rw.entry.ContentHash != nil {
		oldHash = *rw.entry.ContentHash
	}
	if rw.payload != nil {
		if rw.entry.ID == oldHash {
			rw.newID = rw.newHash // content-derived: re-derive
		}
		rw.stored = renamed(rw.entry.StoredPath, rw.entry.ID, rw.newID, rw.payload)
		rw.resourceID = s.recomputedResourceID(rw.entry, oldHash, rw.newHash)
	} else {
		rw.newHash = oldHash
	}
	meta := readMeta(rw.entry.MetadataPath)
	if meta == nil {
		return
	}
	changed := false
	if rw.payload != nil {
		meta["id"] = rw.newID
		setSection(meta, "resource_tracking", "content_hash", rw.newHash)
		if rw.resourceID != nil {
			setSection(meta, "resource_tracking", "resource_id", *rw.resourceID)
		}
		setSection(meta, "import_metadata", "size_bytes", len(rw.payload))
		changed = true
	}
	if rw.kind == "document" || rw.kind == "item" {
		// CUE validation messages quote values, as import withholds them.
		if info, ok := meta["schema_info"].(map[string]any); ok {
			if errs, ok := info["validation_errors"].([]any); ok && len(errs) > 0 {
				for i := range errs {
					errs[i] = importer.WithheldValidationMessage
				}
				changed = true
			}
		}
	}
	if _, n := values.replace(meta); n > 0 {
		changed = true
	}
	if !changed {
		return
	}
	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return
	}
	rw.meta = encoded
	rw.metaPath = renamed(rw.entry.MetadataPath, rw.entry.ID, rw.newID, encoded)
}

// recomputedResourceID returns the identity-less resource ID import would
// derive from newHash, when the entry's resource ID is still the one derived
// from its old hash. Resources with identity values keep theirs.
func (s *Scrubber) recomputedResourceID(entry database.CatalogEntry, oldHash, newHash string) *string {
	if entry.ResourceID == nil || entry.IdentityJSON != nil || s.Inferrer == nil || oldHash == newHash {
		return nil
	}
	root := s.Inferrer.GetInheritanceGraph().IdentityRoot(entry.Schema)
	if *entry.ResourceID != identity.ComputeResourceID(root, nil, oldHash) {
		return nil
	}
	id := identity.ComputeResourceID(root, nil, newHash)
	return &id
}

var revisionMarker = regexp.MustCompile(`\.r[0-9a-f]{8}$`)

// renamed is the new path for a rewritten file: the old ID (or its 16-hex
// prefix) in the name becomes the new one; a name with no ID token gets
// .r<hash8> of content before its extension. Names follow content, so a rerun
// publishes to the same path.
func renamed(path, oldID, newID string, content []byte) string {
	dir, base := filepath.Split(path)
	name := base
	if oldID != newID && len(oldID) >= 16 && len(newID) >= 16 {
		name = strings.ReplaceAll(name, oldID, newID)
		name = strings.ReplaceAll(name, oldID[:16], newID[:16])
	}
	if name == base {
		ext := filepath.Ext(base)
		stem := revisionMarker.ReplaceAllString(strings.TrimSuffix(base, ext), "")
		name = fmt.Sprintf("%s.r%s%s", stem, hashHex(content)[:8], ext)
	}
	return filepath.Join(dir, name)
}

func setSection(meta map[string]any, section, key string, value any) {
	inner, ok := meta[section].(map[string]any)
	if !ok {
		return
	}
	inner[key] = value
}

func hashHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func firstNonSpace(r *bufio.Reader) (byte, error) {
	for {
		b, err := r.Peek(1)
		if err != nil {
			return 0, err
		}
		if b[0] != ' ' && b[0] != '\t' && b[0] != '\r' && b[0] != '\n' {
			return b[0], nil
		}
		if _, err := r.ReadByte(); err != nil {
			return 0, err
		}
	}
}

func appendUnique(list []string, s string) []string {
	for _, have := range list {
		if have == s {
			return list
		}
	}
	return append(list, s)
}
