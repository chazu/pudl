package mubridge

import (
	"fmt"

	"pudl/internal/drift"
)

// ActionSpec matches mu's plugin protocol format.
type ActionSpec struct {
	ID        string            `json:"id"`
	Command   []string          `json:"command"`
	Inputs    map[string]string `json:"inputs"`
	Outputs   []string          `json:"outputs"`
	DependsOn []string          `json:"depends_on,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Network   bool              `json:"network,omitempty"`
}

// PlanResponse matches mu's plan response format.
type PlanResponse struct {
	Actions []ActionSpec      `json:"actions"`
	Outputs map[string]string `json:"outputs,omitempty"`
	Error   string            `json:"error,omitempty"`
}

// ExportFromDriftReport converts a drift result into a mu-compatible plan response.
// Each field difference becomes an ActionSpec describing the detected drift.
func ExportFromDriftReport(report *drift.DriftResult) *PlanResponse {
	if report == nil {
		return &PlanResponse{
			Error: "nil drift report",
		}
	}

	actions := make([]ActionSpec, 0, len(report.Differences))

	for i, diff := range report.Differences {
		actionID := fmt.Sprintf("%s-drift-%d", report.Definition, i)
		action := ActionSpec{
			ID:      actionID,
			Inputs:  map[string]string{"definition": report.Definition, "field": diff.Path},
			Outputs: []string{fmt.Sprintf("%s.%s", report.Definition, diff.Path)},
		}

		switch diff.Type {
		case "changed":
			action.Command = []string{
				"echo",
				fmt.Sprintf("drift detected: field %s changed from %v to %v", diff.Path, diff.Declared, diff.Live),
			}
			action.Inputs["declared"] = fmt.Sprintf("%v", diff.Declared)
			action.Inputs["live"] = fmt.Sprintf("%v", diff.Live)
			action.Inputs["type"] = "changed"
		case "added":
			action.Command = []string{
				"echo",
				fmt.Sprintf("drift detected: field %s added with value %v", diff.Path, diff.Live),
			}
			action.Inputs["live"] = fmt.Sprintf("%v", diff.Live)
			action.Inputs["type"] = "added"
		case "removed":
			action.Command = []string{
				"echo",
				fmt.Sprintf("drift detected: field %s removed (was %v)", diff.Path, diff.Declared),
			}
			action.Inputs["declared"] = fmt.Sprintf("%v", diff.Declared)
			action.Inputs["type"] = "removed"
		}

		actions = append(actions, action)
	}

	resp := &PlanResponse{
		Actions: actions,
		Outputs: map[string]string{
			"definition": report.Definition,
			"status":     report.Status,
		},
	}

	return resp
}
