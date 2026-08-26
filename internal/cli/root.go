// Package cli wires Cobra commands to the grep engine. This package owns
// flag parsing and process-facing concerns (usage text, --version)
// only; all search logic lives in internal/grep.
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"gg/internal/buildinfo"
	"gg/internal/grep"
)

// unsetExitCode marks that RunE never ran (e.g. Cobra handled --version,
// --help, or a usage error before reaching it), so Execute must pick the
// right fallback based on whether an error occurred.
const unsetExitCode = -1

// runGrep is the seam RunE calls through. It's a package-level var
// (rather than grep.Run called directly) purely so tests can verify the
// exit-code plumbing — including PROCESS.md §3.5's "must be 0/1/2
// regardless of OS, and Cobra must not swallow that distinction"
// requirement — for every code, without needing a real search engine
// wired up yet. Production code never reassigns it.
var runGrep = grep.Run

// Execute builds and runs the root command for the given args (typically
// os.Args[1:]) and returns the process exit code to use. Only main.go
// calls os.Exit; no command logic below this does.
//
// ctx is threaded through to grep.Run via Cobra's ExecuteContext/
// cmd.Context() so Ctrl+C (PROCESS.md §3.5) can eventually trigger a
// graceful stop once there's real work to cancel — see main.go, which
// derives ctx from signal.NotifyContext.
func Execute(ctx context.Context, args []string) int {
	exitCode := unsetExitCode
	root := newRootCmd(&exitCode)
	root.SetArgs(args)

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "gg:", err)
		if exitCode == unsetExitCode {
			return grep.ExitError // Cobra-level usage error; RunE never ran
		}
		return exitCode
	}
	if exitCode == unsetExitCode {
		return grep.ExitMatch // e.g. --version/--help handled without RunE
	}
	return exitCode
}

func newRootCmd(exitCode *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gg PATTERN [FILE...]",
		Short: "gg is a cross-platform grep implementation",
		Long: "gg searches FILE(s) for lines matching PATTERN and prints them.\n" +
			"When no FILE is given, or FILE is \"-\", gg reads standard input.",
		Version:       buildinfo.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := grep.Options{
				Pattern: args[0],
				Files:   args[1:],
			}

			code, err := runGrep(cmd.Context(), opts)
			*exitCode = code
			return err
		},
	}

	cmd.SetVersionTemplate("gg version {{.Version}}\n")

	return cmd
}
