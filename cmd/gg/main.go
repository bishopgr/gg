// Command gg is a cross-platform grep implementation. See PROCESS.md at
// the module root for the design and build plan.
package main

import (
	"context"
	"os"
	"os/signal"

	"gg/internal/cli"
)

func main() {
	// PROCESS.md §3.5: Ctrl+C on Windows arrives as os.Interrupt via
	// Go's runtime translation of CTRL_C_EVENT — signal.NotifyContext
	// handles that the same way it handles SIGINT on Unix, so this
	// needs no OS-specific branch. stop() unregisters the signal
	// handler so a second Ctrl+C (if the graceful path ever hangs)
	// falls through to the default terminate-immediately behavior
	// instead of being silently absorbed forever.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	os.Exit(cli.Execute(ctx, os.Args[1:]))
}
