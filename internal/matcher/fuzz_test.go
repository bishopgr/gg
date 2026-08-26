package matcher

import "testing"

// FuzzNew exercises New's full dispatch logic (fixed fast-path vs. regex
// engine, with every combination of -i/-w/-x) plus Find, against
// arbitrary — including invalid-UTF-8 and invalid-regex — input. The
// property under test is simply: never panic. A compile error from New
// is an expected, valid outcome, not a failure.
func FuzzNew(f *testing.F) {
	seeds := []struct {
		pattern, line                          string
		fixed, ignoreCase, wordRegexp, lineRgx bool
	}{
		{"abc", "abc def", false, false, false, false},
		{"", "", true, false, false, false},
		{"(", "text", false, false, false, false},
		{"[a-z", "text", false, false, false, false},
		{"a.b", "aXb", true, true, false, false},
		{`\b\w+\b`, "hello world", false, false, false, false},
		{"cat", "the cat sat", true, false, true, false},
		{"line", "line", false, false, false, true},
		{string([]byte{0xff, 0xfe, 0x00}), "abc\xffdef", true, false, false, false},
		{".*", "", false, false, false, false},
	}
	for _, s := range seeds {
		f.Add(s.pattern, s.line, s.fixed, s.ignoreCase, s.wordRegexp, s.lineRgx)
	}

	f.Fuzz(func(t *testing.T, pattern, line string, fixed, ignoreCase, wordRegexp, lineRegexp bool) {
		m, err := New(Options{
			Pattern:    pattern,
			Fixed:      fixed,
			IgnoreCase: ignoreCase,
			WordRegexp: wordRegexp,
			LineRegexp: lineRegexp,
		})
		if err != nil {
			if m != nil {
				t.Fatalf("New returned a non-nil Matcher alongside an error: %#v", m)
			}
			return // invalid regex pattern is an expected outcome, not a bug
		}

		ranges := m.Find([]byte(line))

		// Structural invariants Find must uphold for any input.
		prevEnd := -1
		for _, r := range ranges {
			if r.Start < 0 || r.End < r.Start || r.End > len(line) {
				t.Fatalf("out-of-bounds range %+v for line of length %d", r, len(line))
			}
			if r.Start < prevEnd {
				t.Fatalf("ranges not in non-overlapping ascending order: %v", ranges)
			}
			prevEnd = r.End
		}
	})
}
