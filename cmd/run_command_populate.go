package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/fieldpath"
	"github.com/chazu/pudl/internal/mubridge"
	"github.com/chazu/pudl/internal/proc"
	"github.com/chazu/pudl/internal/systemmodel"
)

// defaultCommandTimeout bounds each #CommandRun without a timeout.
const defaultCommandTimeout = 10 * time.Minute

// stderrTail is how much of a failed run's stderr its error quotes.
const stderrTail = 4 << 10

// runCommandPopulate executes a #CommandObserve populate: each run's argv
// directly (no shell, no mu), its stdout spooled to disk, decoded as JSON
// records, `set` applied, and every record ingested as one observation. Any
// failing run fails the phase before anything is ingested.
func runCommandPopulate(cat *runCatalog, ctx context.Context, m *systemmodel.SystemModel, modelDir, runID, snapshotID string) (*PopulateReport, error) {
	p := m.Populate
	timeout := defaultCommandTimeout
	if p.Timeout != "" {
		d, err := time.ParseDuration(p.Timeout)
		if err != nil {
			return nil, fmt.Errorf("populate timeout: %w", err)
		}
		timeout = d
	}
	limits, err := importIngestLimits().Resolve()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "pudl_command_populate_")
	if err != nil {
		return nil, fmt.Errorf("create populate workspace: %w", err)
	}
	defer workspaces.track(dir)()

	records := make([]any, 0)
	budget := &byteBudget{remaining: limits.StagingBytes}
	for i, run := range p.Runs {
		out := filepath.Join(dir, fmt.Sprintf("run-%d.json", i))
		if err := executeCommandRun(ctx, run, modelDir, out, timeout, budget); err != nil {
			return nil, fmt.Errorf("populate run %d (%s): %w", i, strings.Join(run.Argv, " "), err)
		}
		got, err := decodeCommandRecords(out, run.Set)
		if err != nil {
			return nil, fmt.Errorf("populate run %d (%s): %w", i, strings.Join(run.Argv, " "), err)
		}
		records = append(records, got...)
	}

	target := populateTargetName(m.Name)
	wrapped, err := json.Marshal([]mubridge.ObserveResult{{Target: target, Current: map[string]any{"records": records}}})
	if err != nil {
		return nil, fmt.Errorf("marshal observe results: %w", err)
	}
	count, snapshotID, err := ingestPopulateOutput(cat, wrapped, populateIngest{observation: m.Observation,
		ctx:          ctx,
		snapshotID:   snapshotID,
		runID:        runID,
		model:        m.Name,
		source:       database.SnapshotSourceCommand,
		manualSchema: p.Schema,
	})
	if err != nil {
		return nil, err
	}
	return &PopulateReport{Target: target, Records: count, SnapshotID: snapshotID}, nil
}

// executeCommandRun runs one command with stdout spooled to out and stderr
// passed through, keeping its tail for the error.
func executeCommandRun(ctx context.Context, run systemmodel.CommandRun, modelDir, out string, timeout time.Duration, budget *byteBudget) error {
	workDir := modelDir
	if run.Dir != "" {
		workDir = run.Dir
		if !filepath.IsAbs(workDir) {
			workDir = filepath.Join(modelDir, workDir)
		}
	}
	program := run.Argv[0]
	if strings.ContainsRune(program, filepath.Separator) && !filepath.IsAbs(program) {
		program = filepath.Join(workDir, program)
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	file, err := os.Create(out)
	if err != nil {
		return err
	}
	defer file.Close()
	tail := &tailBuffer{max: stderrTail}
	command := proc.Command(runCtx, proc.DefaultGrace, program, run.Argv[1:]...)
	command.Dir = workDir
	command.Stdout = &budgetWriter{w: file, budget: budget}
	command.Stderr = io.MultiWriter(errw(), tail)
	if err := command.Run(); err != nil {
		if stopped := proc.Stopped(ctx, runCtx); stopped != nil {
			err = fmt.Errorf("%w (%v)", err, stopped)
		}
		if detail := strings.TrimSpace(tail.String()); detail != "" {
			return fmt.Errorf("%w\nstderr (last %d bytes):\n%s", err, stderrTail, detail)
		}
		return err
	}
	return nil
}

// decodeCommandRecords reads a run's stdout: a stream of JSON values, where an
// array contributes its elements and an object is one record. Numbers stay
// exact. set is applied to every record.
func decodeCommandRecords(path string, set map[string]any) ([]any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	assignments := flattenSet(nil, set)
	dec := json.NewDecoder(file)
	dec.UseNumber()
	var records []any
	add := func(v any) error {
		obj, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("stdout record %d is %T, not a JSON object", len(records), v)
		}
		for _, a := range assignments {
			if err := a.path.Set(obj, a.value); err != nil {
				return fmt.Errorf("set: %w", err)
			}
		}
		records = append(records, obj)
		return nil
	}
	for {
		var v any
		if err := dec.Decode(&v); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("stdout is not JSON: %w", err)
		}
		if arr, ok := v.([]any); ok {
			for _, item := range arr {
				if err := add(item); err != nil {
					return nil, err
				}
			}
			continue
		}
		if err := add(v); err != nil {
			return nil, err
		}
	}
	return records, nil
}

type setAssignment struct {
	path  fieldpath.Path
	value any
}

// flattenSet turns a nested `set` struct into leaf assignments, so
// `set: metadata: labels: env: "x"` sets one label instead of replacing the
// whole metadata object. Order is deterministic.
func flattenSet(prefix []string, set map[string]any) []setAssignment {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []setAssignment
	for _, k := range keys {
		path := append(append([]string{}, prefix...), k)
		if nested, ok := set[k].(map[string]any); ok && len(nested) > 0 {
			out = append(out, flattenSet(path, nested)...)
			continue
		}
		out = append(out, setAssignment{path: fieldpath.FromKeys(path...), value: set[k]})
	}
	return out
}

// byteBudget bounds the total stdout all runs may spool.
type byteBudget struct{ remaining int64 }

type budgetWriter struct {
	w      io.Writer
	budget *byteBudget
}

func (b *budgetWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > b.budget.remaining {
		return 0, fmt.Errorf("command output exceeds the staging byte limit")
	}
	b.budget.remaining -= int64(len(p))
	return b.w.Write(p)
}

// tailBuffer keeps the last max bytes written.
type tailBuffer struct {
	buf bytes.Buffer
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.buf.Write(p)
	if extra := t.buf.Len() - t.max; extra > 0 {
		t.buf.Next(extra)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string { return t.buf.String() }
