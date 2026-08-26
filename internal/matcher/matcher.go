// Package matcher implements the pattern-matching core of gg, independent
// of any file/line I/O. Two implementations exist behind the Matcher
// interface: a byte-scanning literal matcher for the common `-F` fast
// path, and a regexp-backed matcher for everything else (including `-F`
// combined with `-i`/`-w`/`-x`, where correctness — Unicode case folding,
// word-boundary semantics — matters more than avoiding regexp overhead).
package matcher

import (
	"fmt"
	"regexp"
)

// MatchRange is a half-open byte range [Start, End) within a line that
// matched the pattern. Start == End denotes a zero-width match.
type MatchRange struct {
	Start, End int
}

// Matcher reports where a pattern matches within a line of text.
//
// Implementations must be safe to reuse across many calls to Find (one
// Matcher is built per invocation and applied to every line of every
// file), must not retain or mutate the line slice passed in, and must
// return ranges in ascending, non-overlapping order.
type Matcher interface {
	// Find returns every non-overlapping match in line, in the order
	// they occur. A nil or empty slice means line does not match.
	Find(line []byte) []MatchRange
}

// Options configures how New builds a Matcher from a raw pattern string.
// It mirrors the grep flags that affect matching semantics; flags that
// only affect *output* (context lines, -o, color, ...) live above this
// package, in internal/grep and internal/output.
type Options struct {
	// Pattern is the search expression: a literal string when Fixed is
	// set, otherwise a regular expression in Go's RE2 syntax (see
	// https://github.com/google/re2/wiki/Syntax). RE2 does not support
	// backreferences or lookaround — full PCRE (`-P`) is an explicit
	// non-goal for v1 (PROCESS.md §1).
	Pattern string
	// Fixed treats Pattern as a literal string (-F / fgrep behavior)
	// rather than a regular expression.
	Fixed bool
	// IgnoreCase performs case-insensitive matching (-i).
	IgnoreCase bool
	// WordRegexp requires the match to be surrounded by non-word
	// characters or line boundaries (-w).
	WordRegexp bool
	// LineRegexp requires the match to span the entire line (-x). Takes
	// precedence over WordRegexp if both are set, since a whole-line
	// match already satisfies word-boundary semantics.
	LineRegexp bool
}

// New builds a Matcher for opts. It returns an error if Pattern is not a
// valid regular expression (only possible when Fixed is false, since
// literal patterns are always valid).
func New(opts Options) (Matcher, error) {
	if opts.Fixed && !opts.IgnoreCase && !opts.WordRegexp && !opts.LineRegexp {
		// Fast path: no case-folding or boundary wrapping needed, so a
		// plain byte scan is both simpler and cheaper than compiling a
		// regexp. This is the common case for real-world `-F` usage.
		return newFixedMatcher(opts.Pattern), nil
	}

	pattern := opts.Pattern
	if opts.Fixed {
		pattern = regexp.QuoteMeta(pattern)
	}

	m, err := newRegexMatcher(pattern, opts)
	if err != nil {
		// Return a bare nil, not (*regexMatcher)(nil) boxed in the
		// Matcher interface — a typed nil there would make the returned
		// interface value non-nil and trip up any `m != nil` check by
		// the caller.
		return nil, err
	}
	return m, nil
}

func compileError(pattern string, err error) error {
	return fmt.Errorf("gg: invalid pattern %q: %w", pattern, err)
}
