package cmd

// Canonicalization and validation of mu's exact mutation plans, used to build
// and revalidate a run set's approved plan digest.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/systemmodel"
)

func canonicalMutationPlan(plan []byte, stagingDir string) ([]byte, error) {
	document, _, err := validateMuMutationPlan(plan)
	if err != nil {
		return nil, err
	}
	canonical := canonicalPlanValue(document, stagingDir)
	payload, err := json.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("encode canonical mu plan: %w", err)
	}
	return payload, nil
}

func canonicalPlanValue(value any, stagingDir string) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if key == "action_key" || key == "plan_sha256" {
				continue
			}
			out[canonicalPlanString(key, stagingDir)] = canonicalPlanValue(item, stagingDir)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = canonicalPlanValue(item, stagingDir)
		}
		return out
	case string:
		return canonicalPlanString(typed, stagingDir)
	default:
		return value
	}
}

func validateMuMutationPlan(plan []byte) (any, string, error) {
	var document any
	decoder := json.NewDecoder(strings.NewReader(string(plan)))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, "", fmt.Errorf("decode mu JSON plan: %w", err)
	}
	root, ok := document.(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("mu JSON plan must be an object")
	}
	version, ok := root["version"].(json.Number)
	if !ok || version.String() != "2" {
		return nil, "", fmt.Errorf("mu JSON plan version must be exactly 2")
	}
	digest, ok := root["plan_sha256"].(string)
	if !ok || len(digest) != sha256.Size*2 {
		return nil, "", fmt.Errorf("mu JSON plan is missing a valid plan_sha256")
	}
	plugins, exists := root["plugins"].([]any)
	if !exists {
		return nil, "", fmt.Errorf("mu JSON plan v2 is missing plugin identities")
	}
	for index, item := range plugins {
		identity, ok := item.(map[string]any)
		if !ok || stringField(identity, "name") == "" || stringField(identity, "digest") == "" || stringField(identity, "version") == "" {
			return nil, "", fmt.Errorf("mu JSON plan plugin %d lacks immutable name, digest, or version identity", index)
		}
		protocol, ok := identity["protocol_version"].(json.Number)
		if !ok || protocol.String() == "0" {
			return nil, "", fmt.Errorf("mu JSON plan plugin %d lacks protocol identity", index)
		}
		if _, ok := identity["capabilities"].([]any); !ok {
			return nil, "", fmt.Errorf("mu JSON plan plugin %d lacks capability identity", index)
		}
	}
	actions, ok := root["actions"].([]any)
	if !ok {
		return nil, "", fmt.Errorf("mu JSON plan v2 is missing actions")
	}
	for index, item := range actions {
		action, ok := item.(map[string]any)
		if !ok || stringField(action, "id") == "" || stringField(action, "action_key") == "" {
			return nil, "", fmt.Errorf("mu JSON plan action %d lacks id or action_key", index)
		}
		for _, field := range []string{"command", "inputs", "outputs", "depends_on"} {
			if _, exists := action[field]; !exists {
				return nil, "", fmt.Errorf("mu JSON plan action %d lacks required %s field", index, field)
			}
		}
	}

	delete(root, "plan_sha256")
	payload, err := json.Marshal(root)
	if err != nil {
		return nil, "", fmt.Errorf("encode mu JSON plan identity: %w", err)
	}
	actual := sha256.Sum256(payload)
	if hex.EncodeToString(actual[:]) != digest {
		return nil, "", fmt.Errorf("mu JSON plan_sha256 does not match its plan content")
	}
	root["plan_sha256"] = digest
	return document, digest, nil
}

func stringField(value map[string]any, key string) string {
	text, _ := value[key].(string)
	return text
}

func canonicalPlanString(value, stagingDir string) string {
	if stagingDir == "" {
		return value
	}
	return strings.ReplaceAll(value, stagingDir, "<pudl-reconcile-workspace>")
}

type plannedMuActions struct {
	Actions []struct {
		ID            string            `json:"id"`
		SealedInputs  map[string]string `json:"sealed_inputs,omitempty"`
		SealedOutputs map[string]string `json:"sealed_outputs,omitempty"`
	} `json:"actions"`
}

func annotateSealedActionClaims(report *RunReport, plan []byte, model *systemmodel.SystemModel) error {
	var document plannedMuActions
	if err := json.Unmarshal(plan, &document); err != nil {
		return fmt.Errorf("decode mu JSON plan: %w", err)
	}
	for _, action := range document.Actions {
		for _, ref := range modelSealedReferences(model) {
			if strings.Contains(action.ID, ref) {
				return fmt.Errorf("mu plan action id contains a sealed provider reference")
			}
		}
	}
	if len(report.SealedBindings) == 0 {
		return nil
	}
	for index := range report.SealedBindings {
		evidence := &report.SealedBindings[index]
		switch evidence.Direction {
		case "input":
			if evidence.ConsumerPhase != "converge" {
				continue
			}
			for _, action := range document.Actions {
				if _, claimed := action.SealedInputs[evidence.Input]; claimed {
					evidence.ClaimingActionIDs = append(evidence.ClaimingActionIDs, action.ID)
				}
			}
			sort.Strings(evidence.ClaimingActionIDs)
			if len(evidence.ClaimingActionIDs) == 0 {
				return fmt.Errorf("converge sealed input %q has no claiming action", evidence.Input)
			}
		case "output":
			if evidence.ProducerPhase != "converge" {
				continue
			}
			for _, action := range document.Actions {
				if _, claimed := action.SealedOutputs[evidence.Output]; !claimed {
					continue
				}
				if evidence.ProducingActionID != "" {
					return fmt.Errorf("converge sealed output %q has multiple producing actions", evidence.Output)
				}
				evidence.ProducingActionID = action.ID
			}
			if evidence.ProducingActionID == "" {
				return fmt.Errorf("converge sealed output %q has no producing action", evidence.Output)
			}
		}
	}
	return nil
}
