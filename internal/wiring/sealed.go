package wiring

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/systemmodel"
)

type SealedPolicy struct {
	WritableRefs       []string
	WritableConfigured bool
}

type SealedMember struct {
	Model   *systemmodel.SystemModel
	Aliases []string
	RunID   string
}

type ResolvedSealedMember struct {
	Model    *systemmodel.SystemModel
	RunID    string
	Evidence []SealedBindingEvidence
}

// SealedBindingEvidence contains only metadata and a provider-reference
// fingerprint. It intentionally has no field capable of carrying the provider
// path or a resolved secret value.
type SealedBindingEvidence struct {
	Direction                    string   `json:"direction"` // input | output
	ConsumerModel                string   `json:"consumer_model,omitempty"`
	ConsumerRunID                string   `json:"consumer_run_id,omitempty"`
	ConsumerPhase                string   `json:"consumer_phase,omitempty"`
	Input                        string   `json:"input,omitempty"`
	DeliveryMode                 string   `json:"delivery_mode,omitempty"`
	ClaimingActionIDs            []string `json:"claiming_action_ids,omitempty"`
	SourceKind                   string   `json:"source_kind,omitempty"` // direct-ref | producer-output
	ProducerModel                string   `json:"producer_model,omitempty"`
	ProducerRunID                string   `json:"producer_run_id,omitempty"`
	ProducerPhase                string   `json:"producer_phase,omitempty"`
	Output                       string   `json:"output,omitempty"`
	StoreMode                    string   `json:"store_mode,omitempty"`
	ProducingActionID            string   `json:"producing_action_id,omitempty"`
	ProviderScheme               string   `json:"provider_scheme"`
	ReferenceSHA256              string   `json:"reference_sha256"`
	MatchedWritablePatternSHA256 string   `json:"matched_writable_pattern_sha256,omitempty"`
	LifecycleStatus              string   `json:"lifecycle_status"`
}

type sealedOutputOwner struct {
	model, runID, phase, name string
	output                    systemmodel.SealedOutput
}

var providerSchemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*$`)

// ResolveSealedSources resolves the metadata-only reference graph for an exact
// run set. Producer output declarations exclusively own ref/store mode;
// consumers contribute only their local name and delivery mode.
func ResolveSealedSources(members []SealedMember, policy SealedPolicy) ([]ResolvedSealedMember, error) {
	if err := validateWritablePatterns(policy); err != nil {
		return nil, err
	}
	aliases := map[string]string{}
	byName := map[string]SealedMember{}
	for _, member := range members {
		if member.Model == nil || member.Model.Name == "" || member.RunID == "" {
			return nil, fmt.Errorf("sealed resolution requires concrete model and run identity")
		}
		name := member.Model.Name
		if _, exists := byName[name]; exists {
			return nil, fmt.Errorf("sealed resolution contains duplicate model %q", name)
		}
		byName[name] = member
		for _, alias := range append([]string{name}, member.Aliases...) {
			if owner, exists := aliases[alias]; exists && owner != name {
				return nil, fmt.Errorf("sealed model alias %q is ambiguous between %q and %q", alias, owner, name)
			}
			aliases[alias] = name
		}
	}

	outputs := map[string]map[string]sealedOutputOwner{}
	outputEvidence := map[string][]SealedBindingEvidence{}
	for name, member := range byName {
		outputs[name] = map[string]sealedOutputOwner{}
		for _, phase := range sealedPhases(member.Model) {
			for outputName, output := range phase.outputs {
				if _, duplicate := outputs[name][outputName]; duplicate {
					return nil, fmt.Errorf("model %q sealed output %q has multiple phase owners", name, outputName)
				}
				scheme, fingerprint, err := validateProviderRef(output.Ref)
				if err != nil {
					return nil, fmt.Errorf("model %q %s sealed output %q: %w", name, phase.name, outputName, err)
				}
				matched, err := allowWritableRef(output.Ref, policy)
				if err != nil {
					return nil, fmt.Errorf("model %q %s sealed output %q: %w", name, phase.name, outputName, err)
				}
				outputs[name][outputName] = sealedOutputOwner{
					model: name, runID: member.RunID, phase: phase.name, name: outputName, output: output,
				}
				outputEvidence[name] = append(outputEvidence[name], SealedBindingEvidence{
					Direction: "output", ProducerModel: name, ProducerRunID: member.RunID,
					ProducerPhase: phase.name, Output: outputName, StoreMode: output.StoreMode,
					ProviderScheme: scheme, ReferenceSHA256: fingerprint,
					MatchedWritablePatternSHA256: fingerprintSealedMetadata(matched), LifecycleStatus: "planned",
				})
			}
		}
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	resolved := make([]ResolvedSealedMember, 0, len(names))
	for _, name := range names {
		member := byName[name]
		model := cloneSealedModel(member.Model)
		evidence := append([]SealedBindingEvidence(nil), outputEvidence[name]...)
		for _, phase := range sealedPhases(model) {
			for inputName, input := range phase.inputs {
				var owner sealedOutputOwner
				sourceKind := "direct-ref"
				if input.Source != nil {
					sourceKind = "producer-output"
					producerName, exists := aliases[input.Source.Model]
					if !exists {
						return nil, fmt.Errorf("model %q %s sealed input %q producer %q is outside the exact run set", name, phase.name, inputName, input.Source.Model)
					}
					var found bool
					owner, found = outputs[producerName][input.Source.Output]
					if !found {
						return nil, fmt.Errorf("model %q %s sealed input %q references missing output %q on producer %q", name, phase.name, inputName, input.Source.Output, producerName)
					}
					input.Ref = owner.output.Ref
					phase.inputs[inputName] = input
				}
				scheme, fingerprint, err := validateProviderRef(input.Ref)
				if err != nil {
					return nil, fmt.Errorf("model %q %s sealed input %q: %w", name, phase.name, inputName, err)
				}
				item := SealedBindingEvidence{
					Direction: "input", ConsumerModel: name, ConsumerRunID: member.RunID,
					ConsumerPhase: phase.name, Input: inputName, DeliveryMode: input.DeliveryMode,
					SourceKind: sourceKind, ProviderScheme: scheme,
					ReferenceSHA256: fingerprint, LifecycleStatus: "planned",
				}
				if input.Source != nil {
					item.ProducerModel = owner.model
					item.ProducerRunID = owner.runID
					item.ProducerPhase = owner.phase
					item.Output = owner.name
					item.StoreMode = owner.output.StoreMode
				}
				evidence = append(evidence, item)
			}
		}
		sort.Slice(evidence, func(i, j int) bool {
			left := evidence[i].Direction + "\x00" + evidence[i].ConsumerPhase + "\x00" + evidence[i].ProducerPhase + "\x00" + evidence[i].Input + "\x00" + evidence[i].Output
			right := evidence[j].Direction + "\x00" + evidence[j].ConsumerPhase + "\x00" + evidence[j].ProducerPhase + "\x00" + evidence[j].Input + "\x00" + evidence[j].Output
			return left < right
		})
		resolved = append(resolved, ResolvedSealedMember{Model: model, RunID: member.RunID, Evidence: evidence})
	}
	return resolved, nil
}

type sealedPhase struct {
	name    string
	inputs  map[string]systemmodel.SealedInput
	outputs map[string]systemmodel.SealedOutput
}

func sealedPhases(model *systemmodel.SystemModel) []sealedPhase {
	phases := []sealedPhase{{name: "populate", inputs: model.Populate.SealedInputs}}
	if model.Converge != nil {
		phases = append(phases, sealedPhase{name: "converge", inputs: model.Converge.SealedInputs, outputs: model.Converge.SealedOutputs})
	}
	return phases
}

func cloneSealedModel(model *systemmodel.SystemModel) *systemmodel.SystemModel {
	clone := *model
	clone.Populate = model.Populate
	clone.Populate.SealedInputs = cloneSealedInputs(model.Populate.SealedInputs)
	if model.Converge != nil {
		converge := *model.Converge
		converge.SealedInputs = cloneSealedInputs(model.Converge.SealedInputs)
		converge.SealedOutputs = cloneSealedOutputs(model.Converge.SealedOutputs)
		clone.Converge = &converge
	}
	return &clone
}

func cloneSealedInputs(values map[string]systemmodel.SealedInput) map[string]systemmodel.SealedInput {
	if values == nil {
		return nil
	}
	out := make(map[string]systemmodel.SealedInput, len(values))
	for name, value := range values {
		out[name] = value
	}
	return out
}

func cloneSealedOutputs(values map[string]systemmodel.SealedOutput) map[string]systemmodel.SealedOutput {
	if values == nil {
		return nil
	}
	out := make(map[string]systemmodel.SealedOutput, len(values))
	for name, value := range values {
		out[name] = value
	}
	return out
}

func validateProviderRef(ref string) (scheme, fingerprint string, err error) {
	scheme, remainder, found := strings.Cut(ref, ":")
	if !found || remainder == "" || !providerSchemePattern.MatchString(scheme) {
		return "", "", fmt.Errorf("provider reference must be scheme:non-empty-path")
	}
	return scheme, fingerprintSealedMetadata(ref), nil
}

func fingerprintSealedMetadata(value string) string {
	if value == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func validateWritablePatterns(policy SealedPolicy) error {
	if !policy.WritableConfigured {
		return nil
	}
	for _, pattern := range policy.WritableRefs {
		if _, err := path.Match(pattern, ""); err != nil {
			return fmt.Errorf("secrets.writable_refs: invalid pattern %q: %w", pattern, err)
		}
	}
	return nil
}

func allowWritableRef(ref string, policy SealedPolicy) (string, error) {
	if !policy.WritableConfigured {
		return "", nil
	}
	for _, pattern := range policy.WritableRefs {
		if matched, _ := path.Match(pattern, ref); matched {
			return pattern, nil
		}
	}
	return "", fmt.Errorf("provider reference is not allowed by workspace secrets.writable_refs")
}

// MarshalSealedEvidence is a test/report helper that makes the non-leakage
// contract explicit at the package boundary.
func MarshalSealedEvidence(evidence []SealedBindingEvidence) ([]byte, error) {
	return json.Marshal(evidence)
}
