package matcher

import (
	"strings"
	"testing"
)

// benchLine is representative log-line shape: a mostly-unique 80-byte
// line with a match target near the end, so a Find on it exercises a
// realistic amount of scanning rather than matching at position 0.
func benchLine(matchTarget string, matches bool) []byte {
	prefix := strings.Repeat("x", 60)
	if matches {
		return []byte(prefix + " " + matchTarget)
	}
	return []byte(prefix + " " + strings.Repeat("y", len(matchTarget)))
}

// BenchmarkFixedMatcher_Find_Match/_NoMatch measure the plain
// bytes.Index-based literal scan (-F, no modifiers) in both the common
// "most lines don't match" case and the match case, per PROCESS.md §4
// step 8's call to profile before optimizing.
func BenchmarkFixedMatcher_Find_Match(b *testing.B) {
	m := newFixedMatcher("needle")
	line := benchLine("needle", true)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.Find(line)
	}
}

func BenchmarkFixedMatcher_Find_NoMatch(b *testing.B) {
	m := newFixedMatcher("needle")
	line := benchLine("needle", false)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.Find(line)
	}
}

// BenchmarkRegexMatcher_Find_Match/_NoMatch is the default (non -F)
// matching path's counterpart.
func BenchmarkRegexMatcher_Find_Match(b *testing.B) {
	m, err := newRegexMatcher(`needle\d+`, Options{})
	if err != nil {
		b.Fatalf("newRegexMatcher: %v", err)
	}
	line := benchLine("needle123", true)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.Find(line)
	}
}

func BenchmarkRegexMatcher_Find_NoMatch(b *testing.B) {
	m, err := newRegexMatcher(`needle\d+`, Options{})
	if err != nil {
		b.Fatalf("newRegexMatcher: %v", err)
	}
	line := benchLine("needle123", false)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.Find(line)
	}
}
