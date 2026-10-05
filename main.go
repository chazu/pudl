package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/chazu/pudl/cmd"
)

func main() {
	// The first SIGINT/SIGTERM cancels the invocation instead of killing it, so
	// a run can stop mu politely and record how it ended. See
	// cmd.ExecuteContext for what a second signal does.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cmd.ExecuteContext(ctx)
}
