package matcher

import "regexp"

// regexMatcher delegates matching to Go's RE2-based regexp engine. It is
// used directly for non-fixed patterns, and indirectly for `-F` patterns
// that need case-folding or word/line-boundary wrapping (see New).
type regexMatcher struct {
	re *regexp.Regexp
}

// newRegexMatcher compiles pattern after applying the boundary wrapping
// and case-folding implied by opts. pattern is assumed to already be
// regexp-escaped by the caller if it originated as a literal (-F) pattern.
func newRegexMatcher(pattern string, opts Options) (*regexMatcher, error) {
	original := pattern // for error reporting, before boundary/case wrapping

	switch {
	case opts.LineRegexp:
		// -x: the *entire line* must match. Takes precedence over -w
		// (see Options.LineRegexp doc) since this is strictly stronger.
		pattern = "^(?:" + pattern + ")$"
	case opts.WordRegexp:
		// -w: the match must be flanked by non-word characters or the
		// start/end of the line. \b is ASCII word-boundary in RE2.
		pattern = `\b(?:` + pattern + `)\b`
	}

	if opts.IgnoreCase {
		// (?i) at the very start applies case-insensitivity to the
		// whole pattern, including the \b/^$ wrapping above (harmless,
		// since those aren't letter-based) per RE2 flag scoping rules.
		pattern = "(?i)" + pattern
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, compileError(original, err)
	}
	return &regexMatcher{re: re}, nil
}

func (m *regexMatcher) Find(line []byte) []MatchRange {
	idx := m.re.FindAllIndex(line, -1)
	if len(idx) == 0 {
		return nil
	}

	ranges := make([]MatchRange, len(idx))
	for i, pair := range idx {
		ranges[i] = MatchRange{Start: pair[0], End: pair[1]}
	}
	return ranges
}
