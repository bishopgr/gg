package matcher

import (
	"reflect"
	"testing"
)

func TestFixedMatcher_Find(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		line    string
		want    []MatchRange
	}{
		{"no match", "xyz", "hello world", nil},
		{"single match", "world", "hello world", []MatchRange{{6, 11}}},
		{"match at start", "hello", "hello world", []MatchRange{{0, 5}}},
		{"whole line match", "hello", "hello", []MatchRange{{0, 5}}},
		{
			"multiple non-overlapping matches",
			"aa", "aaaa",
			[]MatchRange{{0, 2}, {2, 4}},
		},
		{
			"overlapping occurrences only counted once, non-overlapping",
			"aba", "abababa",
			// scan resumes at end of each match: 0-3, then from 3: "baba" -> match at 4-7
			[]MatchRange{{0, 3}, {4, 7}},
		},
		{"case sensitive: no match on different case", "Hello", "hello world", nil},
		{
			"multiple occurrences across line",
			"cat", "the cat sat on the cat mat",
			[]MatchRange{{4, 7}, {19, 22}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newFixedMatcher(tt.pattern)
			got := m.Find([]byte(tt.line))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Find(%q) with pattern %q = %v, want %v", tt.line, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestFixedMatcher_EmptyPattern(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []MatchRange
	}{
		{"empty line", "", []MatchRange{{0, 0}}},
		{"ascii line", "ab", []MatchRange{{0, 0}, {1, 1}, {2, 2}}},
		{
			// "é" is 2 bytes (0xC3 0x89) in UTF-8; zero-width matches
			// should land on rune boundaries, not every byte.
			"multi-byte utf8", "aé",
			[]MatchRange{{0, 0}, {1, 1}, {3, 3}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newFixedMatcher("")
			got := m.Find([]byte(tt.line))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Find(%q) with empty pattern = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestFixedMatcher_EmptyLine(t *testing.T) {
	m := newFixedMatcher("x")
	if got := m.Find([]byte("")); got != nil {
		t.Errorf("Find(%q) = %v, want nil (no match on empty line)", "", got)
	}
}
