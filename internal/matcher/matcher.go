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
	"strings"
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
	// Patterns is the list of search expressions to match, OR'd
	// together (-e/-f: any line matching *any* pattern in the list is a
	// match) — the normal single-PATTERN case is just a one-element
	// slice. Each is a literal string when Fixed is set, otherwise a
	// regular expression in Go's RE2 syntax (see
	// https://github.com/google/re2/wiki/Syntax). RE2 does not support
	// backreferences or lookaround — full PCRE (`-P`) is an explicit
	// non-goal for v1 (PROCESS.md §1).
	//
	// An empty slice is a valid, deliberate configuration — not an
	// error — matching real GNU grep's own behavior when -e/-f are
	// given but supply zero patterns between them (e.g. an empty -f
	// file and no -e at all): the result matches nothing, not "matches
	// every line" the way compiling a literal empty-string pattern
	// would. See New's doc comment.
	Patterns []string
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

// New builds a Matcher for opts. It returns an error if any pattern is
// not a valid regular expression (only possible when Fixed is false,
// since literal patterns are always valid).
func New(opts Options) (Matcher, error) {
	if len(opts.Patterns) == 0 {
		// Verified against real GNU grep, not assumed: -e/-f supplying
		// zero patterns between them (e.g. an empty -f file and no -e)
		// makes every search a guaranteed no-match, not a compiled
		// empty-string pattern (which would zero-width-match every
		// line instead — the opposite of what real grep actually does
		// here).
		return neverMatcher{}, nil
	}

	if len(opts.Patterns) == 1 && opts.Fixed && !opts.IgnoreCase && !opts.WordRegexp && !opts.LineRegexp {
		// Fast path: no case-folding or boundary wrapping needed, so a
		// plain byte scan is both simpler and cheaper than compiling a
		// regexp. This is the common case for real-world `-F` usage.
		return newFixedMatcher(opts.Patterns[0]), nil
	}

	// Multiple patterns (-e/-f) are combined into one alternation and
	// compiled as a single regexp, rather than running len(Patterns)
	// independent matchers per line: simpler (no match-range merging
	// across matchers needed to satisfy Matcher's ascending/non-
	// overlapping contract) and lets a single -x/-w/-i wrap apply to
	// the whole alternation at once, exactly like real grep. Each
	// pattern is validated individually first so a bad pattern among
	// several is reported by *itself*, not as an unreadable fragment of
	// the final joined regexp.
	parts := make([]string, len(opts.Patterns))
	for i, p := range opts.Patterns {
		if opts.Fixed {
			p = regexp.QuoteMeta(p)
			// Fixed patterns are always valid regexps once escaped; no
			// need to validate them individually below.
			parts[i] = p
			continue
		}
		if _, err := regexp.Compile(p); err != nil {
			return nil, compileError(p, err)
		}
		parts[i] = p
	}
	joined := strings.Join(parts, "|")

	m, err := newRegexMatcher(joined, opts)
	if err != nil {
		// Return a bare nil, not (*regexMatcher)(nil) boxed in the
		// Matcher interface — a typed nil there would make the returned
		// interface value non-nil and trip up any `m != nil` check by
		// the caller.
		return nil, err
	}
	return m, nil
}

// neverMatcher matches nothing, ever. It exists solely for New's
// zero-patterns case — see its doc comment.
type neverMatcher struct{}

func (neverMatcher) Find(line []byte) []MatchRange { return nil }

func compileError(pattern string, err error) error {
	return fmt.Errorf("gg: invalid pattern %q: %w", pattern, err)
}
