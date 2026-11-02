// Package populate runs a model's #CommandObserve populate: plain commands
// whose stdout is a stream of JSON records. It spools and decodes their
// output; ingesting the records into the catalog is the caller's.
package populate

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

	"github.com/chazu/pudl/internal/fieldpath"
	"github.com/chazu/pudl/internal/ingestprep"
	"github.com/chazu/pudl/internal/proc"
	"github.com/chazu/pudl/internal/systemmodel"
)

// DefaultTimeout bounds each #CommandRun of a populate without a timeout.
const DefaultTimeout = 10 * time.Minute

// stderrTail is how much of a failed run's stderr is kept.
const stderrTail = 4 << 10

// Request is how a model's commands run: where relative paths resolve, how
// much stdout they may spool, and where their stderr goes.
type Request struct {
	// ModelDir is the base for a relative run dir and a relative program path.
	ModelDir string
	// Limits bounds the spooled stdout of every run together (StagingBytes).
	Limits ingestprep.Limits
	// Stderr receives each command's stderr as it runs.
	Stderr io.Writer
	// TrackTempDir registers the spool directory so an interrupt can remove
	// it, and returns its release. Nil removes it on return only.
	TrackTempDir func(dir string) (release func())
}

// Result is what a populate observed.
type Result struct {
	// Records are the decoded records of every run that succeeded.
	Records []any
	// Runs counts the runs attempted, after enumeration.
	Runs int
	// Failed lists the runs that failed, in run order.
	Failed []FailedInput
	// Inputs are the enumerated inputs, in order; nil without `each`.
	Inputs []string
}

// Run executes a #CommandObserve populate: the `each` enumeration if any,
// then each run's argv directly (no shell), its stdout spooled to disk,
// decoded as JSON records, and `set` applied.
//
// A run that fails (exits non-zero, times out, or prints something other than
// JSON objects) is a failed input in the Result, and the other runs go on.
// With FailFast it fails the populate instead. Run returns an error, and no
// Result, when the populate cannot proceed: a bad timeout, a failed
// enumeration, the staging byte limit, or the context ending.
func Run(ctx context.Context, p systemmodel.Populate, req Request) (Result, error) {
	timeout := DefaultTimeout
	if p.Timeout != "" {
		d, err := time.ParseDuration(p.Timeout)
		if err != nil {
			return Result{}, fmt.Errorf("populate timeout: %w", err)
		}
		timeout = d
	}
	limits, err := req.Limits.Resolve()
	if err != nil {
		return Result{}, err
	}
	dir, err := os.MkdirTemp("", "pudl_command_populate_")
	if err != nil {
		return Result{}, fmt.Errorf("create populate workspace: %w", err)
	}
	defer req.track(dir)()

	x := &executor{ctx: ctx, modelDir: req.ModelDir, timeout: timeout, budget: &byteBudget{remaining: limits.StagingBytes}, stderr: req.Stderr}
	if x.stderr == nil {
		x.stderr = io.Discard
	}
	res := Result{Records: make([]any, 0)}
	runs := p.Runs
	var items []string
	if p.Each != nil {
		if items, err = x.enumerate(*p.Each, filepath.Join(dir, "each.json")); err != nil {
			return Result{}, err
		}
		res.Inputs = items
		runs = expandRuns(p.Runs, items)
	}
	res.Runs = len(runs)
	for i, run := range runs {
		out := filepath.Join(dir, fmt.Sprintf("run-%d.json", i))
		failure, err := x.attempt(run, out)
		if err != nil {
			return Result{}, fmt.Errorf("populate run %d (%s): %w", i, strings.Join(run.Argv, " "), err)
		}
		var got []any
		if failure == nil {
			if got, err = decodeCommandRecords(out, run.Set); err != nil {
				failure = &FailedInput{Error: err.Error()}
			}
		}
		if failure != nil {
			failure.Run, failure.Argv = i, run.Argv
			if items != nil {
				failure.Item = items[i/len(p.Runs)]
			}
			if p.FailFast {
				return Result{}, failure.Err()
			}
			res.Failed = append(res.Failed, *failure)
			continue
		}
		res.Records = append(res.Records, got...)
	}
	return res, nil
}

func (r Request) track(dir string) func() {
	if r.TrackTempDir != nil {
		return r.TrackTempDir(dir)
	}
	return func() { _ = os.RemoveAll(dir) }
}

// executor runs one populate's commands under a shared byte budget.
type executor struct {
	ctx      context.Context
	modelDir string
	timeout  time.Duration
	budget   *byteBudget
	stderr   io.Writer
}

// attempt runs one command with stdout spooled to out. A command that fails
// is returned as a FailedInput (without its run, argv or item); err is a
// failure of the populate itself: the context ended or the byte budget ran
// out.
func (x *executor) attempt(run systemmodel.CommandRun, out string) (*FailedInput, error) {
	tail, err := x.execute(run, out)
	if err == nil {
		return nil, nil
	}
	if ctxErr := x.ctx.Err(); ctxErr != nil {
		return nil, fmt.Errorf("%w (%w)", err, ctxErr)
	}
	if x.budget.exceeded {
		return nil, errStagingLimit
	}
	return &FailedInput{ExitStatus: exitStatus(err), Error: err.Error(), Stderr: tail}, nil
}

// execute runs one command with stdout spooled to out and stderr passed
// through, returning the tail of its stderr.
func (x *executor) execute(run systemmodel.CommandRun, out string) (string, error) {
	workDir := x.modelDir
	if run.Dir != "" {
		workDir = run.Dir
		if !filepath.IsAbs(workDir) {
			workDir = filepath.Join(x.modelDir, workDir)
		}
	}
	program := run.Argv[0]
	if strings.ContainsRune(program, filepath.Separator) && !filepath.IsAbs(program) {
		program = filepath.Join(workDir, program)
	}
	runCtx, cancel := context.WithTimeout(x.ctx, x.timeout)
	defer cancel()

	file, err := os.Create(out)
	if err != nil {
		return "", err
	}
	defer file.Close()
	tail := &tailBuffer{max: stderrTail}
	command := proc.Command(runCtx, proc.DefaultGrace, program, run.Argv[1:]...)
	command.Dir = workDir
	command.Stdout = &budgetWriter{w: file, budget: x.budget}
	command.Stderr = io.MultiWriter(x.stderr, tail)
	if err := command.Run(); err != nil {
		if stopped := proc.Stopped(x.ctx, runCtx); stopped != nil {
			err = fmt.Errorf("%w (%v)", err, stopped)
		}
		return strings.TrimSpace(tail.String()), err
	}
	return "", nil
}

// decodeCommandRecords reads a run's stdout: a stream of JSON values, where an
// array contributes its elements and an object is one record. Numbers stay
// exact. set is applied to every record.
func decodeCommandRecords(path string, set map[string]any) ([]any, error) {
	assignments := flattenSet(nil, set)
	var records []any
	err := decodeJSONStream(path, func(v any) error {
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
	})
	return records, err
}

// decodeJSONStream calls add for each value of a stream of JSON values, and
// for each element of a value that is an array. Numbers stay exact.
func decodeJSONStream(path string, add func(any) error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	dec := json.NewDecoder(file)
	dec.UseNumber()
	for {
		var v any
		if err := dec.Decode(&v); err == io.EOF {
			return nil
		} else if err != nil {
			return fmt.Errorf("stdout is not JSON: %w", err)
		}
		values := []any{v}
		if arr, ok := v.([]any); ok {
			values = arr
		}
		for _, item := range values {
			if err := add(item); err != nil {
				return err
			}
		}
	}
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

var errStagingLimit = fmt.Errorf("command output exceeds the staging byte limit")

// byteBudget bounds the total stdout all runs may spool.
type byteBudget struct {
	remaining int64
	exceeded  bool
}

type budgetWriter struct {
	w      io.Writer
	budget *byteBudget
}

func (b *budgetWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > b.budget.remaining {
		b.budget.exceeded = true
		return 0, errStagingLimit
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
