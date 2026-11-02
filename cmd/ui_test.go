package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUIRefusesNonLoopbackWithoutAllowRemote(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "ui", "--addr", "0.0.0.0:0", "--no-open")
	require.Error(t, r.Err)
	require.Contains(t, r.Stdout+r.Stderr+r.Err.Error(), "--allow-remote")
}

func TestUIRejectsMalformedAddr(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "ui", "--addr", "nope", "--no-open")
	require.Error(t, r.Err)
}
