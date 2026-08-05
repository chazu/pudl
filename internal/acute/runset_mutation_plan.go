package acute

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/wiring"
)

// RunSetMutationOptions is the run policy committed by an exact mutating plan.
// Durations use their canonical string form so the JSON contract is stable and
// readable across process boundaries.
type RunSetMutationOptions struct {
	MaxObservationAge string `json:"max_observation_age,omitempty"`
	MaxIterations     int    `json:"max_iterations"`
	MaxApplies        int    `json:"max_applies"`
}

// BindingCommitment is the stable subset of binding evidence that determines
// the executable model. Evaluation timestamps and observed age remain report
// evidence but are excluded: recomputing the same plan later must not become
// stale merely because time advanced while the same age policy still passes.
type BindingCommitment struct {
	Input          string          `json:"input"`
	ProducerModel  string          `json:"producer_model"`
	ProducerRunID  string          `json:"producer_run_id"`
	SnapshotID     string          `json:"snapshot_id"`
	Workspace      string          `json:"workspace"`
	Schema         string          `json:"schema"`
	Identity       json.RawMessage `json:"identity"`
	Path           string          `json:"path"`
	Selection      string          `json:"selection"`
	Value          json.RawMessage `json:"value"`
	ValueType      string          `json:"value_type"`
	ScalarSHA256   string          `json:"scalar_sha256"`
	ResolutionCode string          `json:"resolution_status"`
	MaxAge         string          `json:"max_age,omitempty"`
}

// SealedReferenceCommitment is safe durable metadata for one provider
// reference. ReferenceSHA256 fingerprints the full operational reference; the
// provider path itself is deliberately absent.
type SealedReferenceCommitment struct {
	Phase           string `json:"phase"`
	Direction       string `json:"direction"` // input | output
	Name            string `json:"name"`
	ProviderScheme  string `json:"provider_scheme"`
	ReferenceSHA256 string `json:"reference_sha256"`
	DeliveryMode    string `json:"delivery_mode,omitempty"`
	StoreMode       string `json:"store_mode,omitempty"`
}

// RunSetMutationMemberPlan commits one member's effective runtime projection
// and the exact mu plan produced from it. Raw provider refs and raw mu plan text
// are never persisted; collision-resistant digests bind both to the approval.
type RunSetMutationMemberPlan struct {
	Model            string                      `json:"model"`
	RunID            string                      `json:"run_id"`
	SnapshotID       string                      `json:"snapshot_id"`
	ModelSHA256      string                      `json:"model_sha256"`
	MuPlanSHA256     string                      `json:"mu_plan_sha256,omitempty"`
	MutationRequired bool                        `json:"mutation_required"`
	Bindings         []BindingCommitment         `json:"bindings,omitempty"`
	Sealed           []SealedReferenceCommitment `json:"sealed,omitempty"`
}

// RunSetMutationPlan is the immutable, redacted approval identity for one
// complete mutating run set. CanonicalDigest hashes this value exactly.
type RunSetMutationPlan struct {
	PlanVersion             int                        `json:"plan_version"`
	RunSetID                string                     `json:"run_set_id"`
	Mode                    string                     `json:"mode"`
	Options                 RunSetMutationOptions      `json:"options"`
	Edges                   []RunSetEdge               `json:"edges"`
	Ordered                 []string                   `json:"ordered_members"`
	Members                 []RunSetMutationMemberPlan `json:"members"`
	WritableRefsConfigured  bool                       `json:"writable_refs_configured"`
	WritableRefFingerprints []string                   `json:"writable_ref_fingerprints,omitempty"`
}

// NewRunSetMutationMemberPlan builds the stable approval projection for one
// elaborated member. It rejects unresolved source-bound sealed inputs: a plan
// cannot be approved until every operational reference is concrete.
func NewRunSetMutationMemberPlan(model *systemmodel.SystemModel, runID, snapshotID string, evidence []wiring.BindingEvidence, muPlan []byte, mutationRequired bool) (RunSetMutationMemberPlan, error) {
	if model == nil || model.Name == "" || runID == "" || snapshotID == "" {
		return RunSetMutationMemberPlan{}, fmt.Errorf("mutation member plan requires model, run id, and snapshot id")
	}
	modelJSON, err := json.Marshal(model)
	if err != nil {
		return RunSetMutationMemberPlan{}, fmt.Errorf("encode model %q for mutation plan: %w", model.Name, err)
	}
	bindings, err := bindingCommitments(evidence)
	if err != nil {
		return RunSetMutationMemberPlan{}, err
	}
	sealed, err := sealedCommitments(model)
	if err != nil {
		return RunSetMutationMemberPlan{}, err
	}
	member := RunSetMutationMemberPlan{
		Model: model.Name, RunID: runID, SnapshotID: snapshotID,
		ModelSHA256: digestBytes(modelJSON), MutationRequired: mutationRequired,
		Bindings: bindings, Sealed: sealed,
	}
	if mutationRequired {
		if len(muPlan) == 0 {
			return RunSetMutationMemberPlan{}, fmt.Errorf("mutation member %q requires a non-empty mu plan", model.Name)
		}
		member.MuPlanSHA256 = digestBytes(muPlan)
	}
	return member, nil
}

func bindingCommitments(evidence []wiring.BindingEvidence) ([]BindingCommitment, error) {
	out := make([]BindingCommitment, 0, len(evidence))
	for _, item := range evidence {
		identity, err := json.Marshal(item.Identity)
		if err != nil {
			return nil, fmt.Errorf("encode binding identity for %q: %w", item.Input, err)
		}
		commitment := BindingCommitment{
			Input: item.Input, ProducerModel: item.ProducerModel,
			ProducerRunID: item.ProducerRunID, SnapshotID: item.SnapshotID,
			Workspace: item.Workspace, Schema: item.Schema, Identity: identity,
			Path: item.Path, Selection: item.Selection, Value: item.Value,
			ValueType: item.ValueType, ScalarSHA256: item.ScalarSHA256,
			ResolutionCode: item.ResolutionCode,
		}
		if item.MaxAge != nil {
			commitment.MaxAge = item.MaxAge.String()
		}
		out = append(out, commitment)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Input < out[j].Input })
	return out, nil
}

func sealedCommitments(model *systemmodel.SystemModel) ([]SealedReferenceCommitment, error) {
	var out []SealedReferenceCommitment
	appendInputs := func(phase string, inputs map[string]systemmodel.SealedInput) error {
		for name, input := range inputs {
			if input.Ref == "" {
				return fmt.Errorf("model %q %s sealed input %q has an unresolved producer source", model.Name, phase, name)
			}
			out = append(out, sealedCommitment(phase, "input", name, input.Ref, input.DeliveryMode, ""))
		}
		return nil
	}
	appendOutputs := func(phase string, outputs map[string]systemmodel.SealedOutput) {
		for name, output := range outputs {
			out = append(out, sealedCommitment(phase, "output", name, output.Ref, "", output.StoreMode))
		}
	}
	if err := appendInputs("populate", model.Populate.SealedInputs); err != nil {
		return nil, err
	}
	if model.Converge != nil {
		if err := appendInputs("converge", model.Converge.SealedInputs); err != nil {
			return nil, err
		}
		appendOutputs("converge", model.Converge.SealedOutputs)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Phase != out[j].Phase {
			return out[i].Phase < out[j].Phase
		}
		if out[i].Direction != out[j].Direction {
			return out[i].Direction < out[j].Direction
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func sealedCommitment(phase, direction, name, ref, deliveryMode, storeMode string) SealedReferenceCommitment {
	scheme := ref
	if index := strings.IndexByte(ref, ':'); index >= 0 {
		scheme = ref[:index]
	}
	return SealedReferenceCommitment{
		Phase: phase, Direction: direction, Name: name,
		ProviderScheme: scheme, ReferenceSHA256: digestBytes([]byte(ref)),
		DeliveryMode: deliveryMode, StoreMode: storeMode,
	}
}

func FingerprintWritableRefs(refs []string) []string {
	fingerprints := make([]string, 0, len(refs))
	for _, ref := range refs {
		fingerprints = append(fingerprints, digestBytes([]byte(ref)))
	}
	sort.Strings(fingerprints)
	return fingerprints
}

func (p RunSetMutationPlan) CanonicalDigest() (string, error) {
	if p.PlanVersion == 0 || p.RunSetID == "" || p.Mode != "converge" {
		return "", fmt.Errorf("invalid mutating run-set plan identity")
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("encode mutating run-set plan: %w", err)
	}
	return digestBytes(payload), nil
}

func digestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
