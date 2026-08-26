// Package grep is the composition root for the search engine: it wires
// together internal/matcher, internal/lineio, internal/encoding,
// internal/walker, internal/source, internal/output, and
// internal/termenv into the actual behavior described in the README.
// The CLI layer (internal/cli) depends on this package; this package
// must never import internal/cli or Cobra.
package grep

import (
	"context"
	"fmt"
	"io"
	"os"

	"gg/internal/matcher"
	"gg/internal/output"
	"gg/internal/walker"
)

// Exit codes mirror POSIX grep semantics: callers (internal/cli, then
// main.go) propagate these as the process exit code.
const (
	ExitMatch   = 0 // at least one match was found
	ExitNoMatch = 1 // ran successfully, found no matches
	ExitError   = 2 // usage error, missing file, I/O error, etc.
)

// Options captures the parsed CLI flags for a single invocation.
type Options struct {
	// Patterns is the list of search expressions to match, OR'd
	// together — see matcher.Options.Patterns's doc comment. The
	// ordinary single-PATTERN case is a one-element slice; multiple
	// entries come from -e/-f (internal/cli resolves the "positional
	// PATTERN vs. -e/-f" precedence before constructing Options — see
	// PatternFiles below for the other half of that). An empty slice is
	// valid and matches nothing, matching real GNU grep's -e/-f
	// behavior for zero supplied patterns.
	Patterns []string
	// PatternFiles are -f FILE arguments: each file's lines are read
	// and appended to Patterns (in file-then-line order, after every
	// literal Patterns entry) before matching begins — see
	// resolvePatterns. "-" means stdin, the same convention every other
	// FILE argument follows.
	PatternFiles []string
	// Files are the paths to search; empty means read stdin. "-" among
	// Files also means stdin, matching the conventional meaning every
	// Unix-heritage tool gives it.
	Files []string

	// Matching semantics — see matcher.Options for what each does.
	Fixed       bool // -F
	IgnoreCase  bool // -i
	InvertMatch bool // -v
	WordRegexp  bool // -w
	LineRegexp  bool // -x

	// Output shaping.
	LineNumber        bool      // -n
	OnlyMatching      bool      // -o
	Count             bool      // -c
	FilesWithMatches  bool      // -l
	FilesWithoutMatch bool      // -L
	Quiet             bool      // -q
	NoFilename        bool      // -h
	WithFilename      bool      // -H
	Color             ColorMode // --color

	// Context lines (-A/-B/-C). ContextEnabled is deliberately separate
	// from "ContextBefore > 0 || ContextAfter > 0": GNU grep's own
	// behavior (verified directly against it, not assumed) is that
	// explicitly passing e.g. -A0 or -C0 still turns on "--" group
	// separators between non-adjacent matches, even though zero actual
	// context lines get printed — that's observably different from not
	// passing any context flag at all, which never prints separators.
	// internal/cli sets this from whether -A/-B/-C was present on the
	// command line, not from the resolved values.
	ContextEnabled bool // true if -A, -B, or -C was given at all
	ContextBefore  int  // -B (or -C, if -B wasn't given explicitly)
	ContextAfter   int  // -A (or -C, if -A wasn't given explicitly)

	// Directory traversal — see walker.Options for what each does.
	Recursive      bool // -r or -R
	FollowSymlinks bool // -R specifically
	Include        []string
	Exclude        []string
	ExcludeDir     []string

	// Stdout and Stderr default to os.Stdout/os.Stderr when nil. They
	// exist so callers (tests, and someday a library-style embedding)
	// can capture output without touching the real process streams.
	Stdout io.Writer
	Stderr io.Writer
}

// Run executes a search for opts and returns the exit code the process
// should use, plus an error for anything that should be reported to the
// user. Run is the single entry point the CLI layer calls into — no
// terminal/color/os.Exit policy belongs below this boundary, and no flag
// parsing belongs above it.
//
// Run distinguishes two kinds of failure deliberately:
//   - A single, whole-command-failing error (bad pattern, cancelled
//     context) is returned as Run's error, for internal/cli's generic
//     "gg: <err>" handler to print once.
//   - Per-file problems encountered while searching multiple files
//     (missing file, permission denied, a directory given without -r)
//     are reported directly to opts.Stderr as they're found — exactly
//     the way GNU grep prints one diagnostic line per bad file and
//     keeps going — and are never returned as Run's error, so they
//     don't also get a redundant generic wrapper line from the CLI.
//     They still force the final exit code to ExitError.
//
// ctx carries graceful-shutdown signaling (PROCESS.md §3.5).
func Run(ctx context.Context, opts Options) (int, error) {
	if err := ctx.Err(); err != nil {
		return ExitError, err
	}

	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	patterns, err := resolvePatterns(opts.Patterns, opts.PatternFiles)
	if err != nil {
		return ExitError, err
	}

	m, err := matcher.New(matcher.Options{
		Patterns:   patterns,
		Fixed:      opts.Fixed,
		IgnoreCase: opts.IgnoreCase,
		WordRegexp: opts.WordRegexp,
		LineRegexp: opts.LineRegexp,
	})
	if err != nil {
		return ExitError, err
	}

	roots := opts.Files
	if len(roots) == 0 {
		roots = []string{"-"}
	}
	roots, err = walker.ExpandGlobs(roots)
	if err != nil {
		return ExitError, err
	}

	// PROCESS.md §3.4/grep convention: show filenames when there's more
	// than one root or when recursing (a recursive search over a
	// directory that happens to contain one file still gets a filename
	// prefix in real grep), unless -h/-H override it. -h wins over -H
	// when both are given — an arbitrary but documented tie-break for an
	// unusual combination.
	showFilename := len(roots) > 1 || opts.Recursive
	if opts.WithFilename {
		showFilename = true
	}
	if opts.NoFilename {
		showFilename = false
	}

	printer := output.New(stdout, output.Options{
		ShowFilename: showFilename,
		LineNumber:   opts.LineNumber,
		OnlyMatching: opts.OnlyMatching,
		Color:        resolveColor(opts.Color, stdout),
	})
	defer printer.Flush()

	eng := &engine{
		ctx:     ctx,
		matcher: m,
		opts:    opts,
		mode:    selectMode(opts),
		printer: printer,
		stderr:  stderr,
	}
	if opts.ContextEnabled {
		eng.context = newContextTracker(opts.ContextBefore, opts.ContextAfter)
	}

	for _, root := range roots {
		if ctx.Err() != nil {
			eng.hadError = true
			break
		}
		if root == "-" {
			eng.searchStdin()
		} else {
			eng.searchPath(root)
		}
		if eng.stopped {
			break
		}
	}

	if err := printer.Flush(); err != nil {
		return ExitError, fmt.Errorf("gg: writing output: %w", err)
	}

	switch {
	case eng.hadError:
		return ExitError, nil
	case eng.hadMatch:
		return ExitMatch, nil
	default:
		return ExitNoMatch, nil
	}
}
