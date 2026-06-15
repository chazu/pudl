package cmd

import "testing"

func stopCommands(settings map[string]interface{}) []string {
	hooks, _ := settings["hooks"].(map[string]interface{})
	groups, _ := hooks["Stop"].([]interface{})
	var cmds []string
	for _, g := range groups {
		gm, _ := g.(map[string]interface{})
		inner, _ := gm["hooks"].([]interface{})
		for _, hh := range inner {
			hm, _ := hh.(map[string]interface{})
			if c, _ := hm["command"].(string); c != "" {
				cmds = append(cmds, c)
			}
		}
	}
	return cmds
}

func TestMergeHooksIntoEmpty(t *testing.T) {
	settings := map[string]interface{}{}
	added := mergeHooks(settings)
	if len(added) != 2 {
		t.Fatalf("added = %v, want 2 hooks", added)
	}
	// idempotent second merge
	if again := mergeHooks(settings); len(again) != 0 {
		t.Errorf("second merge added %v, want none", again)
	}
}

func TestMergeHooksPreservesExisting(t *testing.T) {
	settings := map[string]interface{}{
		"model": "opus",
		"hooks": map[string]interface{}{
			"Stop": []interface{}{
				map[string]interface{}{
					"hooks": []interface{}{
						map[string]interface{}{"type": "command", "command": "echo existing"},
					},
				},
			},
		},
	}
	mergeHooks(settings)

	if settings["model"] != "opus" {
		t.Errorf("model setting clobbered: %v", settings["model"])
	}
	cmds := stopCommands(settings)
	hasExisting, hasOurs := false, false
	for _, c := range cmds {
		if c == "echo existing" {
			hasExisting = true
		}
		if c == "pudl facts curate" {
			hasOurs = true
		}
	}
	if !hasExisting {
		t.Error("existing Stop hook was lost")
	}
	if !hasOurs {
		t.Error("our Stop hook was not added")
	}
}
