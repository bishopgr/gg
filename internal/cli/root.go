// Package cli wires Cobra commands to the grep engine. This package owns
// flag parsing and process-facing concerns (usage text, --version)
// only; all search logic lives in internal/grep.
package cli

import (
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

// Execute builds and runs the root command for the given args (typically
// os.Args[1:]) and returns the process exit code to use. Only main.go
// calls os.Exit; no command logic below this does.
func Execute(args []string) int {
	exitCode := unsetExitCode
	root := newRootCmd(&exitCode)
	root.SetArgs(args)

	if err := root.Execute(); err != nil {
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

			code, err := grep.Run(opts)
			*exitCode = code
			return err
		},
	}

	cmd.SetVersionTemplate("gg version {{.Version}}\n")

	return cmd
}
