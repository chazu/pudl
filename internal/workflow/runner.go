package workflow

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"pudl/internal/artifact"
	"pudl/internal/database"
	"pudl/internal/executor"
)

// StepExecutor abstracts method execution for testability.
type StepExecutor interface {
	Run(ctx context.Context, opts executor.RunOptions) (*executor.RunResult, error)
}

// Runner executes workflows with concurrent step dispatch.
type Runner struct {
	executor StepExecutor
	db       *database.CatalogDB
	dataPath string
}

// RunOptions configures a workflow execution.
type RunOptions struct {
	DryRun         bool
	Tags           map[string]string
	MaxConcurrency int // 0 means no limit
}

// StepResult holds the outcome of a single workflow step execution.
type StepResult struct {
	StepName       string
	DefinitionName string
	MethodName     string
	Status         string // success, failed, skipped
	Output         interface{}
	Error          string
	ArtifactID     string
	StartTime      time.Time
	EndTime        time.Time
	Duration       time.Duration
	Attempt        int
}

// RunResult holds the outcome of an entire workflow execution.
type RunResult struct {
	WorkflowName string
	RunID        string
	Status       string // success, failed, partial
	Steps        map[string]*StepResult
	StartTime    time.Time
	EndTime      time.Time
	Duration     time.Duration
}

// NewRunner creates a new workflow runner.
func NewRunner(exec StepExecutor, db *database.CatalogDB, dataPath string) *Runner {
	return &Runner{
		executor: exec,
		db:       db,
		dataPath: dataPath,
	}
}

// Run executes a workflow with concurrent step dispatch.
func (r *Runner) Run(ctx context.Context, wf *Workflow, opts RunOptions) (*RunResult, error) {
	startTime := time.Now()
	runID := startTime.Format("20060102T150405")

	// Build and validate DAG
	dag, err := BuildDAG(wf)
	if err != nil {
		return nil, fmt.Errorf("failed to build DAG: %w", err)
	}
	if _, err := dag.TopologicalSort(); err != nil {
		return nil, fmt.Errorf("invalid workflow: %w", err)
	}

	result := &RunResult{
		WorkflowName: wf.Name,
		RunID:        runID,
		Steps:        make(map[string]*StepResult),
		StartTime:    startTime,
	}

	// Step outputs for input threading (write-once, read-many)
	var outputs sync.Map

	completed := make(map[string]bool)
	failed := false
	var mu sync.Mutex

	totalSteps := len(wf.Steps)

	for len(completed) < totalSteps && !failed {
		ready := dag.GetReadySteps(completed)
		if len(ready) == 0 {
			break
		}

		g, gctx := errgroup.WithContext(ctx)
		if opts.MaxConcurrency > 0 {
			g.SetLimit(opts.MaxConcurrency)
		}

		for _, stepName := range ready {
			stepName := stepName // capture
			step := wf.Steps[stepName]

			g.Go(func() error {
				sr := r.executeStep(gctx, step, &outputs, opts)

				mu.Lock()
				result.Steps[stepName] = sr
				if sr.Status == "success" {
					completed[stepName] = true
				} else if sr.Status == "failed" && wf.AbortOnFailure {
					failed = true
				} else {
					completed[stepName] = true
				}
				mu.Unlock()

				return nil
			})
		}

		g.Wait()

		if failed {
			// Mark remaining steps as skipped
			for name := range wf.Steps {
				if result.Steps[name] == nil {
					result.Steps[name] = &StepResult{
						StepName:       name,
						DefinitionName: wf.Steps[name].Definition,
						MethodName:     wf.Steps[name].Method,
						Status:         "skipped",
						Error:          "skipped due to upstream failure",
					}
				}
			}
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Determine overall status
	result.Status = "success"
	for _, sr := range result.Steps {
		if sr.Status == "failed" {
			result.Status = "failed"
			break
		}
		if sr.Status == "skipped" {
			result.Status = "partial"
		}
	}

	// Save manifest
	r.saveManifest(result)

	return result, nil
}

// executeStep runs a single workflow step with retry and timeout.
func (r *Runner) executeStep(ctx context.Context, step Step, outputs *sync.Map, opts RunOptions) *StepResult {
	sr := &StepResult{
		StepName:       step.Name,
		DefinitionName: step.Definition,
		MethodName:     step.Method,
		StartTime:      time.Now(),
		Attempt:        1,
	}

	maxAttempts := step.Retries + 1

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		sr.Attempt = attempt

		// Apply step timeout
		stepCtx := ctx
		var cancel context.CancelFunc
		if step.Timeout > 0 {
			stepCtx, cancel = context.WithTimeout(ctx, step.Timeout)
		}

		// Resolve inputs from prior step outputs
		tags := r.resolveInputs(step, outputs, opts.Tags)

		// Execute via executor
		execResult, err := r.executor.Run(stepCtx, executor.RunOptions{
			DefinitionName: step.Definition,
			MethodName:     step.Method,
			DryRun:         opts.DryRun,
			Tags:           tags,
		})

		if cancel != nil {
			cancel()
		}

		if err != nil {
			sr.Error = err.Error()
			if attempt < maxAttempts {
				continue // retry
			}
			sr.Status = "failed"
			sr.EndTime = time.Now()
			sr.Duration = sr.EndTime.Sub(sr.StartTime)
			return sr
		}

		// Success
		sr.Status = "success"
		sr.Output = execResult.Output

		// Store output in sync.Map for downstream steps
		if execResult.Output != nil {
			outputs.Store(step.Name, execResult.Output)

			// Store artifact
			if !opts.DryRun && r.db != nil {
				artResult, err := artifact.Store(r.db, artifact.StoreOptions{
					Definition: step.Definition,
					Method:     step.Method,
					Output:     execResult.Output,
					Tags:       tags,
					DataPath:   r.dataPath,
				})
				if err == nil {
					sr.ArtifactID = artResult.Proquint
				}
			}
		}

		sr.EndTime = time.Now()
		sr.Duration = sr.EndTime.Sub(sr.StartTime)
		return sr
	}

	sr.Status = "failed"
	sr.EndTime = time.Now()
	sr.Duration = sr.EndTime.Sub(sr.StartTime)
	return sr
}

// resolveInputs builds a tags map by resolving step input references to prior outputs.
func (r *Runner) resolveInputs(step Step, outputs *sync.Map, baseTags map[string]string) map[string]string {
	tags := make(map[string]string)
	for k, v := range baseTags {
		tags[k] = v
	}

	for key, ref := range step.Inputs {
		if strings.HasPrefix(ref, "steps.") {
			// Extract step name and field from "steps.<name>.outputs.<field>"
			parts := strings.SplitN(ref, ".", 4)
			if len(parts) >= 2 {
				stepName := parts[1]
				if val, ok := outputs.Load(stepName); ok {
					// Try to extract the specific field from the output
					resolved := resolveOutputField(val, parts)
					tags[key] = fmt.Sprintf("%v", resolved)
				}
			}
		} else {
			tags[key] = ref
		}
	}

	return tags
}

// resolveOutputField extracts a field from a step output value.
func resolveOutputField(output interface{}, parts []string) interface{} {
	if len(parts) < 4 {
		return output
	}

	field := parts[3]

	// Try map[string]interface{}
	if m, ok := output.(map[string]interface{}); ok {
		if val, exists := m[field]; exists {
			return val
		}
	}

	// Try map[interface{}]interface{}
	if m, ok := output.(map[interface{}]interface{}); ok {
		if val, exists := m[field]; exists {
			return val
		}
	}

	return output
}

// saveManifest writes the run result as a manifest.
func (r *Runner) saveManifest(result *RunResult) {
	store := NewManifestStore(r.dataPath)

	steps := make(map[string]StepManifest)
	for name, sr := range result.Steps {
		steps[name] = StepManifest{
			StepName:       sr.StepName,
			DefinitionName: sr.DefinitionName,
			MethodName:     sr.MethodName,
			Status:         sr.Status,
			Error:          sr.Error,
			ArtifactID:     sr.ArtifactID,
			StartTime:      sr.StartTime.Format(time.RFC3339),
			EndTime:        sr.EndTime.Format(time.RFC3339),
			DurationSecs:   sr.Duration.Seconds(),
			Attempt:        sr.Attempt,
		}
	}

	manifest := &Manifest{
		WorkflowName: result.WorkflowName,
		RunID:        result.RunID,
		Status:       result.Status,
		StartTime:    result.StartTime,
		EndTime:      result.EndTime,
		DurationSecs: result.Duration.Seconds(),
		Steps:        steps,
	}

	store.Save(manifest)
}
