// Package checks evaluates model assertions independently of the CLI.
package checks

import (
	"context"
	"fmt"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/evidence"
	"github.com/chazu/pudl/internal/projection"
	"github.com/chazu/pudl/internal/systemmodel"
)

// Scopes recorded on a check result. ScopeRun means the check was evaluated
// with the run's ID bound as a constraint; ScopeLatestKnown means it saw the
// whole catalog; ScopeSnapshots means it saw only the evidence it selected.
const (
	ScopeRun         = "run"
	ScopeLatestKnown = "latest-known"
	ScopeSnapshots   = "snapshots"
)

type Progress struct {
	Phase string `json:"phase"`
	Check string `json:"check,omitempty"`
	State string `json:"state"`
}
type Request struct {
	Catalog                *database.CatalogDB
	Registry               *projection.Registry
	RulePaths              []string
	Load                   projection.LoadFunc
	Checks                 []systemmodel.Check
	RunID, CurrentSnapshot string
	FromCatalog            bool
	OnProgress             func(Progress)
}
type Result struct {
	Name, Query, Expect, Severity, Message, Scope, Outcome string
	Passed                                                 bool
	Count                                                  int
	Evidence                                               []evidence.Reference
	Diagnostics                                            []projection.Diagnostic
	Tuples                                                 []datalog.Tuple
}

// Evaluate synchronizes projections, verifies required evidence, selects the
// query view and evaluates every assertion. It has no command globals or writers.
func Evaluate(evalCtx context.Context, req Request) ([]Result, error) {
	if len(req.Checks) == 0 {
		return nil, nil
	}
	if req.Catalog == nil || req.Registry == nil || req.Load == nil {
		return nil, fmt.Errorf("check evaluation requires catalog, schemas, and payload reader")
	}
	db, reg := req.Catalog, req.Registry
	rules, err := datalog.LoadRulesFromPaths(req.RulePaths...)
	if err != nil {
		return nil, fmt.Errorf("load rules: %w", err)
	}
	syncReport, syncErr := projection.Sync(evalCtx, db, reg, req.Load, false)
	if err := evalCtx.Err(); err != nil {
		return nil, err
	}
	var results []Result
	for _, c := range req.Checks {
		if err := evalCtx.Err(); err != nil {
			return results, err
		}
		if req.OnProgress != nil {
			req.OnProgress(Progress{Phase: "checks", Check: c.Name, State: "started"})
		}
		scope := ScopeLatestKnown
		var constraints map[string]interface{}
		if !req.FromCatalog && req.RunID != "" && HeadExposesRunID(rules, c.Query) {
			scope = ScopeRun
			constraints = map[string]interface{}{"run_id": req.RunID}
		}

		result := Result{Name: c.Name, Query: c.Query, Expect: c.Expect, Severity: c.Severity, Scope: scope, Message: c.Message}
		queryDB := db
		var view *evidence.View
		if len(c.Evidence) > 0 {
			result.Scope = ScopeSnapshots
			constraints = nil // snapshot membership scopes catalog joins as well
			maxAge, err := parseEvidenceAge(c.MaxAge)
			if err == nil {
				view, err = evidence.Open(evalCtx, db, reg, evidence.Request{Selectors: c.Evidence, Current: req.CurrentSnapshot, MaxAge: maxAge})
			}
			if err != nil {
				result.Diagnostics = []projection.Diagnostic{{Code: "evidence_unavailable", Message: err.Error()}}
			} else {
				defer view.Close()
				result.Evidence = view.References
				result.Diagnostics = view.Diagnostics
				if view.DB != nil {
					queryDB = view.DB
					result.Diagnostics = append(result.Diagnostics, projection.CheckDiagnostics(queryDB, reg.Subset(view.Schemas), rules, c.Query, projection.SyncReport{}, nil)...)
				}
			}
		} else if c.MaxAge != "" {
			result.Diagnostics = []projection.Diagnostic{{Code: "evidence_scope_required", Message: "max_age requires explicit evidence selectors"}}
		} else {
			result.Diagnostics = projection.CheckDiagnostics(db, reg, rules, c.Query, syncReport, syncErr)
		}
		if len(result.Diagnostics) > 0 {
			result.Outcome = "unknown"
			results = append(results, result)
			continue
		}
		tuples, err := datalog.EvaluateContext(evalCtx, queryDB, rules, c.Query, constraints, datalog.TemporalScope{}, datalog.EvalOptions{})
		if view != nil {
			view.Close()
			view = nil
		}
		if err != nil {
			result.Outcome = "error"
			result.Diagnostics = []projection.Diagnostic{{Code: "evaluation_error", Message: err.Error()}}
			results = append(results, result)
			continue
		}

		result.Count = len(tuples)
		result.Passed = Passes(c.Expect, result.Count)
		result.Outcome = checkOutcome(result.Passed)
		result.Tuples = tuples
		results = append(results, result)
	}
	return results, nil
}

func parseEvidenceAge(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	age, err := time.ParseDuration(value)
	if err != nil || age <= 0 {
		return 0, fmt.Errorf("max_age must be a positive duration")
	}
	return age, nil
}

func checkOutcome(passed bool) string {
	if passed {
		return "pass"
	}
	return "fail"
}

func Passes(expect string, count int) bool {
	switch expect {
	case "empty":
		return count == 0
	case "nonempty":
		return count > 0
	default:
		return false
	}
}

func HeadExposesRunID(rules []datalog.Rule, relation string) bool {
	for _, rule := range rules {
		if rule.Head.Rel != relation {
			continue
		}
		if term, ok := rule.Head.Args["run_id"]; ok && term.IsVariable() {
			return true
		}
	}
	return false
}
