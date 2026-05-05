package mubridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Envelope is the wire format mu plugins (or any producer) can use to
// hand pudl typed data in a single self-contained artifact.
//
// JSON shape:
//
//	{
//	  "schema":      {"module": "mu/aws", "version": "v1", "definition": "#EC2Instance"},
//	  "definitions": [
//	      {"path": "ec2.cue", "content": "package aws\n#EC2Instance: {...}\n"}
//	  ],
//	  "data":        <the actual payload — any JSON value>
//	}
//
// Detection: a JSON document is treated as an envelope iff its
// top-level object contains both a "schema" key (with module+version
// populated) and a "data" key. Raw JSON without those keys is passed
// through untouched.
//
// Definitions are optional; when present, pudl writes them into its
// schema cache so subsequent imports of the same (module, version)
// classify as 'declared' instead of 'auto_registered'.
type Envelope struct {
	Schema      EnvelopeSchema      `json:"schema"`
	Definitions []EnvelopeDefFile   `json:"definitions,omitempty"`
	Data        json.RawMessage     `json:"data"`
}

// EnvelopeSchema is the schema reference portion of an envelope. Field
// shape is intentionally compatible with the SchemaRef emitted by mu
// plugins via their discover response.
type EnvelopeSchema struct {
	Module     string `json:"module"`
	Version    string `json:"version"`
	Definition string `json:"definition,omitempty"`
	Source     string `json:"source,omitempty"`
}

// EnvelopeDefFile is one inline CUE source file. Path is
// slash-separated, relative to the (module, version) directory.
type EnvelopeDefFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// CanonicalRef returns the schema_ref string used in item_schemas:
// "<module>@<version>" or "<module>@<version>#<definition>".
func (s EnvelopeSchema) CanonicalRef() string {
	if s.Module == "" || s.Version == "" {
		return ""
	}
	r := s.Module + "@" + s.Version
	if s.Definition != "" {
		r += s.Definition
	}
	return r
}

// envelopeProbe is the minimum shape needed to decide "is this an
// envelope?" without fully decoding the (potentially large) data
// payload. We unmarshal into this first; if both keys are populated
// and schema looks valid, we re-decode into the full Envelope.
type envelopeProbe struct {
	Schema *EnvelopeSchema `json:"schema"`
	Data   json.RawMessage `json:"data"`
}

// Unwrap reads JSON from r and returns either:
//   - (envelope, nil, nil)  — the input was an envelope; envelope.Data
//     holds the inner payload bytes
//   - (nil, raw, nil)       — the input was raw JSON; raw is the full
//     original bytes (so callers can rewrite to disk or pass through)
//
// Detection only considers JSON objects: a top-level array, string, or
// other non-object document is always raw. An object missing either
// "schema" (with module+version) or "data" is also raw.
//
// Unwrap reads all of r before returning, since both possible outputs
// (envelope decode, raw passthrough) need the full document. Callers
// should bound input size upstream when streaming untrusted producers.
func Unwrap(r io.Reader) (*Envelope, []byte, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("mubridge.Unwrap: read: %w", err)
	}
	env, err := DecodeEnvelope(raw)
	if err != nil {
		return nil, nil, err
	}
	if env != nil {
		return env, nil, nil
	}
	return nil, raw, nil
}

// DecodeEnvelope inspects raw JSON bytes and returns a parsed Envelope
// if the document fits the envelope shape, or (nil, nil) otherwise.
// An error is returned only when the document looks like an envelope
// but is malformed (e.g. schema present but missing module/version).
func DecodeEnvelope(raw []byte) (*Envelope, error) {
	if !looksLikeJSONObject(raw) {
		return nil, nil
	}
	body := stripUTF8BOM(raw)
	var probe envelopeProbe
	if err := json.Unmarshal(body, &probe); err != nil {
		// Not parseable as an object with these keys — treat as raw.
		return nil, nil
	}
	// An envelope requires both a schema (with module+version) AND a
	// data field. Anything less is raw JSON that happens to share a
	// key name.
	if probe.Schema == nil || probe.Data == nil {
		return nil, nil
	}
	if probe.Schema.Module == "" || probe.Schema.Version == "" {
		return nil, errors.New("mubridge: envelope schema missing module or version")
	}
	var env Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("mubridge: decode envelope: %w", err)
	}
	return &env, nil
}

func stripUTF8BOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

// looksLikeJSONObject is a cheap pre-check that skips obvious
// non-object inputs (arrays, primitives, NDJSON, CSV, etc.) without
// allocating a full json.Decoder. It tolerates leading whitespace
// and a UTF-8 BOM but is otherwise strict: the first non-whitespace
// byte must be '{'.
func looksLikeJSONObject(raw []byte) bool {
	i := 0
	if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
		i = 3
	}
	for i < len(raw) {
		switch raw[i] {
		case ' ', '\t', '\n', '\r':
			i++
		case '{':
			return true
		default:
			return false
		}
	}
	return false
}
