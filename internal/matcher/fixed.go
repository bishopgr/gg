package matcher

import (
	"bytes"
	"unicode/utf8"
)

// fixedMatcher finds non-overlapping, case-sensitive occurrences of a
// literal byte pattern via plain scanning (bytes.Index) — no regexp
// compilation or backtracking involved.
type fixedMatcher struct {
	pattern []byte
}

func newFixedMatcher(pattern string) *fixedMatcher {
	return &fixedMatcher{pattern: []byte(pattern)}
}

func (m *fixedMatcher) Find(line []byte) []MatchRange {
	if len(m.pattern) == 0 {
		// An empty pattern is contained in every line. There's no
		// natural single "match range" for that, so — mirroring how
		// Go's regexp engine treats an empty pattern — report a
		// zero-width match at every rune boundary, including the end
		// of the line. See matcher_test.go for the pinned-down
		// behavior this produces.
		return zeroWidthMatches(line)
	}

	var ranges []MatchRange
	offset := 0
	for {
		idx := bytes.Index(line[offset:], m.pattern)
		if idx < 0 {
			break
		}
		start := offset + idx
		end := start + len(m.pattern)
		ranges = append(ranges, MatchRange{Start: start, End: end})
		offset = end // non-overlapping: resume search after this match
	}
	return ranges
}

// zeroWidthMatches returns a zero-width MatchRange at every rune boundary
// in line, including position 0 for an empty line and the final position
// at len(line). This matches what regexp.FindAllIndex does for an empty
// pattern, so fixedMatcher("") and a regexMatcher compiled from "" agree.
func zeroWidthMatches(line []byte) []MatchRange {
	if len(line) == 0 {
		return []MatchRange{{Start: 0, End: 0}}
	}

	ranges := make([]MatchRange, 0, len(line)+1)
	for pos := 0; pos <= len(line); {
		ranges = append(ranges, MatchRange{Start: pos, End: pos})
		if pos == len(line) {
			break
		}
		_, size := utf8.DecodeRune(line[pos:])
		if size <= 0 {
			size = 1 // invalid UTF-8 byte: advance by one byte, not zero
		}
		pos += size
	}
	return ranges
}
