package acute

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// RunSetMemberReport is one model's entry in a check report. Report is the
// member's full report (check reports from version 3); earlier run-set
// reports carried only the summary.
type RunSetMemberReport struct {
	Summary *VerdictSummary `json:"summary,omitempty"`
	Report  json.RawMessage `json:"report,omitempty"`
	Model   string          `json:"model"`
	RunID   string          `json:"run_id"`
	Result  string          `json:"result"`
	Error   string          `json:"error,omitempty"`
}

// RunSetReport is the one report of a `pudl check`, for one model or several.
// Stored `pudl run set` reports (versions 1 and 2) decode into it too.
type RunSetReport struct {
	Summary       *VerdictSummary      `json:"summary,omitempty"`
	ReportVersion int                  `json:"report_version"`
	RunSetID      string               `json:"run_set_id"`
	Mode          string               `json:"mode"`
	Status        string               `json:"status"`
	PlanDigest    string               `json:"plan_digest"`
	Edges         []RunSetEdge         `json:"edges"`
	Ordered       []string             `json:"ordered_members"`
	Members       []RunSetMemberReport `json:"members"`
}

// Digest commits to the deterministic observe-only orchestration plan.
func (p *RunSetPlan) Digest(mode string) (string, error) {
	payload, err := json.Marshal(struct {
		Mode    string       `json:"mode"`
		Edges   []RunSetEdge `json:"edges"`
		Ordered []string     `json:"ordered_members"`
	}{Mode: mode, Edges: p.Edges, Ordered: p.Ordered})
	if err != nil {
		return "", fmt.Errorf("encode run-set plan digest: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
