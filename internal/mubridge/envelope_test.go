package mubridge

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestUnwrap_RawJSON(t *testing.T) {
	cases := []string{
		`{"instance_id":"i-abc"}`,        // plain object, no schema/data keys
		`[1,2,3]`,                        // array
		`"just a string"`,                // primitive
		`{"data":"only data, no schema"}`, // half an envelope
		`{"schema":{"module":"mu/aws","version":"v1"}}`, // half an envelope
	}
	for _, in := range cases {
		env, raw, err := Unwrap(strings.NewReader(in))
		if err != nil {
			t.Errorf("%s: unexpected error %v", in, err)
			continue
		}
		if env != nil {
			t.Errorf("%s: classified as envelope, want raw", in)
		}
		if string(raw) != in {
			t.Errorf("%s: raw bytes mismatch", in)
		}
	}
}

func TestUnwrap_Envelope(t *testing.T) {
	in := `{
		"schema": {"module": "mu/aws", "version": "v1", "definition": "#EC2Instance"},
		"definitions": [
			{"path": "ec2.cue", "content": "package aws\n#EC2Instance: {}\n"}
		],
		"data": {"instance_id": "i-abc", "state": "running"}
	}`
	env, raw, err := Unwrap(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Unwrap: %v", err)
	}
	if raw != nil {
		t.Fatal("expected raw=nil for envelope input")
	}
	if env == nil {
		t.Fatal("expected envelope")
	}
	if env.Schema.Module != "mu/aws" || env.Schema.Version != "v1" {
		t.Errorf("schema = %+v", env.Schema)
	}
	if ref := env.Schema.CanonicalRef(); ref != "mu/aws@v1#EC2Instance" {
		t.Errorf("canonical ref = %q", ref)
	}
	if len(env.Definitions) != 1 || env.Definitions[0].Path != "ec2.cue" {
		t.Errorf("definitions = %+v", env.Definitions)
	}
	// Data is a json.RawMessage; round-trip it.
	var got map[string]string
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if got["instance_id"] != "i-abc" || got["state"] != "running" {
		t.Errorf("data = %+v", got)
	}
}

func TestUnwrap_EnvelopeSchemaMissingFields(t *testing.T) {
	in := `{"schema":{"module":"mu/aws"},"data":{"x":1}}`
	if _, _, err := Unwrap(strings.NewReader(in)); err == nil {
		t.Error("expected error for envelope with missing version")
	}
}

func TestUnwrap_LeadingWhitespaceAndBOM(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	in := append(bom, []byte("  \n\t"+`{"schema":{"module":"mu/aws","version":"v1"},"data":1}`)...)
	env, raw, err := Unwrap(bytes.NewReader(in))
	if err != nil {
		t.Fatalf("Unwrap: %v", err)
	}
	if env == nil {
		t.Errorf("expected envelope, got raw=%q", raw)
	}
}

func TestUnwrap_DataPreservesNumbers(t *testing.T) {
	// json.RawMessage preserves the original number literal — important
	// for downstream importers that care about float vs int distinction.
	in := `{"schema":{"module":"mu/aws","version":"v1"},"data":{"count":42,"ratio":3.14}}`
	env, _, err := Unwrap(strings.NewReader(in))
	if err != nil || env == nil {
		t.Fatalf("Unwrap: env=%v err=%v", env, err)
	}
	if !bytes.Contains(env.Data, []byte("42")) {
		t.Errorf("Data lost integer literal: %s", env.Data)
	}
}

func TestCanonicalRef_NoDefinition(t *testing.T) {
	s := EnvelopeSchema{Module: "mu/aws", Version: "v1"}
	if got := s.CanonicalRef(); got != "mu/aws@v1" {
		t.Errorf("got %q, want mu/aws@v1", got)
	}
}

func TestDecodeEnvelope_TopLevelArrayIsRaw(t *testing.T) {
	env, err := DecodeEnvelope([]byte(`[{"schema":{"module":"x","version":"v1"},"data":1}]`))
	if err != nil {
		t.Fatal(err)
	}
	if env != nil {
		t.Error("array containing envelope-shaped object is not itself an envelope")
	}
}
