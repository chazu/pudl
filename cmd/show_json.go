package cmd

import (
	"encoding/json"
	"os"

	"github.com/chazu/pudl/internal/lister"
)

// showOutput is `pudl show --json`: the catalog entry and its stored record
// (omitted with --meta-only), plus its metadata when --metadata asks for it.
type showOutput struct {
	Entry    lister.ListEntry `json:"entry"`
	Metadata json.RawMessage  `json:"metadata,omitempty"`
	Raw      json.RawMessage  `json:"raw,omitempty"`
	// RawText carries stored data that is not JSON (YAML, CSV) verbatim.
	RawText string `json:"raw_text,omitempty"`
	// Compliance is each applicable policy's stored result.
	Compliance []complianceResult `json:"compliance"`
}

// writeEntryJSON writes entry as one JSON document. Stored JSON is embedded
// byte-for-byte, so numbers keep their exact value.
func writeEntryJSON(entry lister.ListEntry, includeMetadata, includeRaw bool) error {
	out := showOutput{Entry: entry, Compliance: entryCompliance(entry)}
	if includeMetadata {
		content, err := os.ReadFile(entry.MetadataPath)
		if err != nil {
			return err
		}
		out.Metadata = embedJSON(content)
	}
	if includeRaw {
		content, err := os.ReadFile(entry.StoredPath)
		if err != nil {
			return err
		}
		if raw := embedJSON(content); raw != nil {
			out.Raw = raw
		} else {
			out.RawText = string(content)
		}
	}
	return printJSON(out)
}

// embedJSON returns content as an embeddable JSON value, or nil when it is not
// a single valid JSON document.
func embedJSON(content []byte) json.RawMessage {
	if !json.Valid(content) {
		return nil
	}
	return json.RawMessage(content)
}
