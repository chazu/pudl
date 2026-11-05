// Package populate runs a model's #CommandObserve populate: plain commands
// whose stdout is a stream of JSON records. It spools and decodes their
// output; ingesting the records into the catalog is the caller's.
package populate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
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
	// Empty lists the runs whose failure matched empty_when, in run order:
	// observed with no records, not failed.
	Empty []EmptyInput
	// Inputs are the enumerated inputs, in order; nil without `each`.
	Inputs []string
	// Fallback is set when the populate's fallback was used: the Result is
	// the fallback's, and this says why.
	Fallback *FallbackUse
}

// Run executes a #CommandObserve populate: the `each` enumeration if any,
// then each run's argv directly (no shell), its stdout spooled to disk,
// decoded as JSON records, and `set` applied.
//
// A run that fails (exits non-zero, times out, or prints something other than
// JSON objects) is a failed input in the Result, and the other runs go on.
// With FailFast it fails the populate instead. A run that exits non-zero with
// stderr matching an EmptyWhen pattern is an empty input: no records, not a
// failure. A run that exits 0 with stderr matching a FailWhen pattern is a
// failed input: it saw only part of its input, so its records are dropped. Run returns an error, and no Result, when the populate cannot
// proceed: a bad timeout or empty_when pattern, a failed enumeration, the
// staging byte limit, or the context ending.
//
// With a Fallback, a run or enumeration that fails with stderr matching
// Fallback.When, or an enumeration that lists nothing, abandons the
// populate's own runs: their records are discarded, and the Result is the
// fallback's, with Fallback recording what triggered it and whether the
// fallback is partial.
func Run(ctx context.Context, p systemmodel.Populate, req Request) (Result, error) {
	var when []*regexp.Regexp
	if p.Fallback != nil {
		var err error
		if when, err = p.Fallback.Patterns(); err != nil {
			return Result{}, err
		}
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

	x := &executor{ctx: ctx, modelDir: req.ModelDir, budget: &byteBudget{remaining: limits.StagingBytes}, stderr: req.Stderr}
	if x.stderr == nil {
		x.stderr = io.Discard
	}
	primary := filepath.Join(dir, "primary")
	res, trigger, err := x.observe(p, primary, when)
	if err != nil || trigger == nil {
		return res, err
	}
	fmt.Fprintf(x.stderr, "pudl: using the populate fallback: %s\n", trigger.Summary())
	// The abandoned runs' output is discarded, so its spool and its share of
	// the staging budget are released for the fallback.
	_ = os.RemoveAll(primary)
	x.budget = &byteBudget{remaining: limits.StagingBytes}
	res, _, err = x.observe(p.Fallback.Observe, filepath.Join(dir, "fallback"), nil)
	if err != nil {
		return Result{}, fmt.Errorf("populate fallback: %w", err)
	}
	trigger.Partial = p.Fallback.Partial
	res.Fallback = trigger
	return res, nil
}

// observe runs one source: a populate's runs or its fallback's. A non-nil
// FallbackUse means the source could not answer and its Result is to be
// discarded; when is nil for a source without a fallback.
func (x *executor) observe(p systemmodel.Populate, dir string, when []*regexp.Regexp) (Result, *FallbackUse, error) {
	emptyWhen, err := p.EmptyPatterns()
	if err != nil {
		return Result{}, nil, err
	}
	failWhen, err := p.FailPatterns()
	if err != nil {
		return Result{}, nil, err
	}
	x.timeout = DefaultTimeout
	if p.Timeout != "" {
		d, err := time.ParseDuration(p.Timeout)
		if err != nil {
			return Result{}, nil, fmt.Errorf("populate timeout: %w", err)
		}
		x.timeout = d
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Result{}, nil, fmt.Errorf("create populate workspace: %w", err)
	}
	res := Result{Records: make([]any, 0)}
	runs := p.Runs
	var items []string
	if p.Each != nil {
		if items, err = x.enumerate(*p.Each, filepath.Join(dir, "each.json")); err != nil {
			if use, ok := eachFallback(when, err); ok {
				return Result{}, use, nil
			}
			return Result{}, nil, err
		}
		if len(items) == 0 && when != nil {
			return Result{}, &FallbackUse{Reason: FallbackNoInputs}, nil
		}
		res.Inputs = items
		runs = expandRuns(p.Runs, items)
	}
	res.Runs = len(runs)
	for i, run := range runs {
		out := filepath.Join(dir, fmt.Sprintf("run-%d.json", i))
		failure, err := x.attempt(run, out, failWhen)
		if err != nil {
			return Result{}, nil, fmt.Errorf("populate run %d (%s): %w", i, strings.Join(run.Argv, " "), err)
		}
		var got []any
		if failure == nil {
			if got, err = decodeCommandRecords(out, run); err != nil {
				failure = &FailedInput{Error: err.Error()}
			}
		}
		if failure != nil {
			failure.Run, failure.Argv = i, run.Argv
			if items != nil {
				failure.Item = items[i/len(p.Runs)]
			}
			if pattern, line, ok := matchStderr(when, failure); ok {
				f := *failure
				return Result{}, &FallbackUse{Reason: FallbackFailedInput, Pattern: pattern, StderrLine: line, Trigger: &f}, nil
			}
			if empty, ok := matchEmpty(emptyWhen, failure); ok {
				res.Empty = append(res.Empty, empty)
				continue
			}
			if p.FailFast {
				return Result{}, nil, failure.Err()
			}
			res.Failed = append(res.Failed, *failure)
			continue
		}
		res.Records = append(res.Records, got...)
	}
	return res, nil, nil
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

// attempt runs one command with stdout spooled to out. A command that fails,
// or exits 0 with stderr matching failWhen, is returned as a FailedInput
// (without its run, argv or item); err is a failure of the populate itself:
// the context ended or the byte budget ran out.
func (x *executor) attempt(run systemmodel.CommandRun, out string, failWhen []*regexp.Regexp) (*FailedInput, error) {
	tail, err := x.execute(run, out)
	if err == nil {
		return matchFail(failWhen, tail), nil
	}
	if ctxErr := x.ctx.Err(); ctxErr != nil {
		return nil, fmt.Errorf("%w (%w)", err, ctxErr)
	}
	if x.budget.exceeded {
		return nil, errStagingLimit
	}
	status := exitStatus(err)
	exited := status > 0 && !errors.Is(err, context.DeadlineExceeded)
	return &FailedInput{ExitStatus: status, Error: err.Error(), Stderr: tail, exited: exited}, nil
}

// execute runs one command with stdout spooled to out and stderr passed
// through, returning the tail of its stderr (also when it succeeds, for
// fail_when).
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
			err = fmt.Errorf("%w (%w)", err, stopped)
		}
		return strings.TrimSpace(tail.String()), err
	}
	return strings.TrimSpace(tail.String()), nil
}

// decodeCommandRecords reads a run's stdout: a stream of JSON values, where an
// array contributes its elements and an object is one record. Numbers stay
// exact. The run's transform, if any, reshapes or drops each record; set is
// then applied to every record kept.
func decodeCommandRecords(path string, run systemmodel.CommandRun) ([]any, error) {
	assignments := flattenSet(nil, run.Set)
	var records []any
	n := 0
	err := decodeJSONStream(path, func(v any) error {
		obj, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("stdout record %d is %T, not a JSON object", n, v)
		}
		n++
		if run.Transform != nil {
			out, keep, err := run.Transform.Apply(obj)
			if err != nil {
				return fmt.Errorf("stdout record %d: %w", n-1, err)
			}
			if !keep {
				return nil
			}
			obj = out
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
