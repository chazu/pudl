package cmd

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// muRunner is the subprocess seam between PUDL's run policy and mu's execution
// engine, for the phases outside the convergence loop.
//
// `acute.Executor` already abstracts the converge loop's observe/plan/apply, but
// its production implementation — and every other phase — reached
// `exec.Command("mu", ...)` directly, so the acceptance matrix in the
// architecture report ("observe-only differential run", "observe-only inventory
// run", "converge to clean") could not run end to end without a real mu, a
// cluster, and a network. That is the gap this closes: the subprocess boundary is
// part of the run's domain behaviour, so it is an injectable contract rather than
// a call.
//
// It is passed explicitly rather than kept in a package variable. A swappable
// global would be a shared mutable seam that parallel tests race on, and the
// cost of a parameter is one word at four call sites.
type muRunner interface {
	// Observe runs `mu observe --json <target>` against a config file and returns
	// stdout. Machine output is on stdout, diagnostics on stderr (invariant 7),
	// and stderr is folded into the error on failure.
	Observe(configPath, target string) ([]byte, error)

	// Build runs `mu build [flags...] <target>` against a config file and returns
	// stdout.
	Build(configPath, target string, flags ...string) ([]byte, error)
}

// execMu is the production runner: it invokes the real mu binary, preserving
// exactly the argument order and error wrapping the direct calls used.
type execMu struct{}

func (execMu) Observe(configPath, target string) ([]byte, error) {
	out, err := runMu([]string{"observe", "--config", configPath, "--json", target})
	if err != nil {
		return nil, fmt.Errorf("mu observe %s: %w", target, err)
	}
	return out, nil
}

func (execMu) Build(configPath, target string, flags ...string) ([]byte, error) {
	args := append([]string{"build"}, flags...)
	args = append(args, "--config", configPath, target)
	return runMu(args)
}

// runMu executes mu, returning stdout and folding stderr into any error.
func runMu(args []string) ([]byte, error) {
	command := exec.Command("mu", args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
