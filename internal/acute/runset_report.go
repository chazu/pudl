package acute

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type RunSetMemberReport struct {
	Model            string `json:"model"`
	RunID            string `json:"run_id"`
	Result           string `json:"result"`
	MutationRequired bool   `json:"mutation_required,omitempty"`
	Error            string `json:"error,omitempty"`
}

type RunSetReport struct {
	ReportVersion  int                  `json:"report_version"`
	RunSetID       string               `json:"run_set_id"`
	Mode           string               `json:"mode"`
	Status         string               `json:"status"`
	PlanDigest     string               `json:"plan_digest"`
	ApprovalID     string               `json:"approval_id,omitempty"`
	ApprovalStatus string               `json:"approval_status,omitempty"`
	Edges          []RunSetEdge         `json:"edges"`
	Ordered        []string             `json:"ordered_members"`
	Members        []RunSetMemberReport `json:"members"`
}

// Digest commits to the deterministic observe-only orchestration plan and its
// run policy. Resolved values and mutation/action identities join this digest
// when mutating run-set planning lands.
func (p *RunSetPlan) Digest(mode, maxObservationAge string) (string, error) {
	payload, err := json.Marshal(struct {
		Mode              string       `json:"mode"`
		MaxObservationAge string       `json:"max_observation_age,omitempty"`
		Edges             []RunSetEdge `json:"edges"`
		Ordered           []string     `json:"ordered_members"`
	}{Mode: mode, MaxObservationAge: maxObservationAge, Edges: p.Edges, Ordered: p.Ordered})
	if err != nil {
		return "", fmt.Errorf("encode run-set plan digest: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
