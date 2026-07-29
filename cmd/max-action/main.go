// Command max-action sends a single MAX message from a GitHub Actions step.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Fgeeha/max-action/internal/action"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := action.Run(ctx, action.Env{
		Getenv:  os.Getenv,
		Stdout:  os.Stdout,
		Version: version,
	})
	if err != nil {
		// Run has already written the ::error:: annotation.
		os.Exit(1)
	}
}
