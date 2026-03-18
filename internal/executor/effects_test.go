package executor

import (
	"testing"
)

func TestParseEffects_ValidEffects(t *testing.T) {
	output := map[string]interface{}{
		"result": "ok",
		"pudl/effects": []interface{}{
			map[string]interface{}{
				"kind":        "create",
				"description": "Create EC2 instance",
				"params": map[string]interface{}{
					"instance_type": "t3.micro",
					"ami":           "ami-12345",
				},
			},
			map[string]interface{}{
				"kind":        "http",
				"description": "Notify webhook",
				"params": map[string]interface{}{
					"url":    "https://example.com/hook",
					"method": "POST",
				},
			},
		},
	}

	effects, found := ParseEffects(output)
	if !found {
		t.Fatal("expected to find effects")
	}
	if len(effects) != 2 {
		t.Fatalf("expected 2 effects, got %d", len(effects))
	}

	if effects[0].Kind != "create" {
		t.Errorf("expected kind 'create', got %q", effects[0].Kind)
	}
	if effects[0].Description != "Create EC2 instance" {
		t.Errorf("expected description 'Create EC2 instance', got %q", effects[0].Description)
	}
	if effects[0].Params["instance_type"] != "t3.micro" {
		t.Errorf("expected param instance_type='t3.micro', got %v", effects[0].Params["instance_type"])
	}

	if effects[1].Kind != "http" {
		t.Errorf("expected kind 'http', got %q", effects[1].Kind)
	}
}

func TestParseEffects_NoEffectsKey(t *testing.T) {
	output := map[string]interface{}{
		"result": "ok",
	}

	effects, found := ParseEffects(output)
	if found {
		t.Fatal("expected no effects")
	}
	if effects != nil {
		t.Fatal("expected nil effects")
	}
}

func TestParseEffects_NonMapOutput(t *testing.T) {
	effects, found := ParseEffects("just a string")
	if found {
		t.Fatal("expected no effects from string output")
	}
	if effects != nil {
		t.Fatal("expected nil effects")
	}
}

func TestParseEffects_NilOutput(t *testing.T) {
	effects, found := ParseEffects(nil)
	if found {
		t.Fatal("expected no effects from nil output")
	}
	if effects != nil {
		t.Fatal("expected nil effects")
	}
}

func TestParseEffects_MalformedEffects(t *testing.T) {
	// Effect without kind should be skipped
	output := map[string]interface{}{
		"pudl/effects": []interface{}{
			map[string]interface{}{
				"description": "Missing kind",
			},
			map[string]interface{}{
				"kind":        "delete",
				"description": "Valid effect",
			},
		},
	}

	effects, found := ParseEffects(output)
	if !found {
		t.Fatal("expected to find effects (one valid)")
	}
	if len(effects) != 1 {
		t.Fatalf("expected 1 valid effect, got %d", len(effects))
	}
	if effects[0].Kind != "delete" {
		t.Errorf("expected kind 'delete', got %q", effects[0].Kind)
	}
}

func TestParseEffects_EmptyEffectList(t *testing.T) {
	output := map[string]interface{}{
		"pudl/effects": []interface{}{},
	}

	effects, found := ParseEffects(output)
	if found {
		t.Fatal("expected no effects from empty list")
	}
	if effects != nil {
		t.Fatal("expected nil effects")
	}
}

func TestParseEffects_EffectWithNoParams(t *testing.T) {
	output := map[string]interface{}{
		"pudl/effects": []interface{}{
			map[string]interface{}{
				"kind":        "exec",
				"description": "Run cleanup script",
			},
		},
	}

	effects, found := ParseEffects(output)
	if !found {
		t.Fatal("expected to find effects")
	}
	if len(effects) != 1 {
		t.Fatalf("expected 1 effect, got %d", len(effects))
	}
	if effects[0].Kind != "exec" {
		t.Errorf("expected kind 'exec', got %q", effects[0].Kind)
	}
	if len(effects[0].Params) != 0 {
		t.Errorf("expected empty params, got %v", effects[0].Params)
	}
}

func TestFormatEffect(t *testing.T) {
	e := Effect{
		Kind:        "create",
		Description: "Create resource",
		Params:      map[string]interface{}{"name": "test"},
	}
	s := FormatEffect(e)
	if s == "" {
		t.Fatal("expected non-empty format string")
	}
}

func TestFormatEffectOutcome(t *testing.T) {
	eo := EffectOutcome{
		Effect: Effect{Kind: "delete", Description: "Remove instance"},
		Status: "executed",
	}
	s := FormatEffectOutcome(eo)
	if s == "" {
		t.Fatal("expected non-empty format string")
	}

	eo.Error = "permission denied"
	s = FormatEffectOutcome(eo)
	if s == "" {
		t.Fatal("expected non-empty format string with error")
	}
}
