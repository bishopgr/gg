// Package grep is the composition root for the search engine: it will
// wire together the matcher, walker, lineio reader, and output printer
// described in PROCESS.md. The CLI layer (internal/cli) depends on this
// package; this package must never import internal/cli or Cobra.
package grep

import "errors"

// Exit codes mirror POSIX grep semantics: callers (internal/cli, then
// main.go) propagate these as the process exit code.
const (
	ExitMatch   = 0 // at least one match was found
	ExitNoMatch = 1 // ran successfully, found no matches
	ExitError   = 2 // usage error, missing file, I/O error, etc.
)

// Options captures the parsed CLI flags relevant to a single invocation.
// It will grow as features land per the PROCESS.md build order (matcher
// selection, walker filters, printer formatting, context lines, etc.).
type Options struct {
	// Pattern is the search expression (regex, or literal when Fixed is set).
	Pattern string
	// Files are the paths to search; empty means read stdin.
	Files []string
}

// Run executes a search for opts and returns the exit code the process
// should use, plus an error for anything that should be reported to the
// user. Run is the single entry point the CLI layer calls into — no
// terminal/color/os.Exit policy belongs below this boundary, and no flag
// parsing belongs above it.
func Run(opts Options) (int, error) {
	// TODO(step 2+): wire internal/matcher, internal/lineio,
	// internal/walker, internal/output per PROCESS.md §4.
	return ExitError, errors.New("gg: search engine not yet implemented")
}
