package matcher

import (
	"reflect"
	"testing"
)

func TestRegexMatcher_Find(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		opts    Options
		line    string
		want    []MatchRange
	}{
		{
			"basic literal-looking regex",
			"wor.d", Options{},
			"hello world", []MatchRange{{6, 11}},
		},
		{
			"anchored start",
			"^hello", Options{},
			"hello world", []MatchRange{{0, 5}},
		},
		{
			"anchored start no match mid-line",
			"^world", Options{},
			"hello world", nil,
		},
		{
			"alternation, multiple matches",
			"cat|dog", Options{},
			"a cat and a dog", []MatchRange{{2, 5}, {12, 15}},
		},
		{
			"character class and quantifier",
			"[0-9]+", Options{},
			"room 42, hall 7", []MatchRange{{5, 7}, {14, 15}},
		},
		{
			"ignore case",
			"HELLO", Options{IgnoreCase: true},
			"hello world", []MatchRange{{0, 5}},
		},
		{
			"word regexp excludes substring match",
			"cat", Options{WordRegexp: true},
			"category", nil,
		},
		{
			"word regexp matches standalone word",
			"cat", Options{WordRegexp: true},
			"the cat sat", []MatchRange{{4, 7}},
		},
		{
			"line regexp requires full line match",
			"hello", Options{LineRegexp: true},
			"hello world", nil,
		},
		{
			"line regexp matches whole line exactly",
			"hello world", Options{LineRegexp: true},
			"hello world", []MatchRange{{0, 11}},
		},
		{
			"line regexp takes precedence over word regexp",
			"hello world", Options{LineRegexp: true, WordRegexp: true},
			"hello world", []MatchRange{{0, 11}},
		},
		{
			"ignore case combined with word regexp",
			"CAT", Options{IgnoreCase: true, WordRegexp: true},
			"the Cat sat", []MatchRange{{4, 7}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := newRegexMatcher(tt.pattern, tt.opts)
			if err != nil {
				t.Fatalf("newRegexMatcher(%q, %+v) error: %v", tt.pattern, tt.opts, err)
			}
			got := m.Find([]byte(tt.line))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Find(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestRegexMatcher_InvalidPattern(t *testing.T) {
	_, err := newRegexMatcher("(unclosed", Options{})
	if err == nil {
		t.Fatal("expected an error for an unbalanced regex, got nil")
	}
}

func TestRegexMatcher_EmptyPattern(t *testing.T) {
	// Pinned-down behavior: an empty regex matches (zero-width) at every
	// rune boundary, same shape as fixedMatcher's empty-pattern handling.
	m, err := newRegexMatcher("", Options{})
	if err != nil {
		t.Fatalf("newRegexMatcher(\"\", ...) error: %v", err)
	}
	got := m.Find([]byte("ab"))
	want := []MatchRange{{0, 0}, {1, 1}, {2, 2}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Find(\"ab\") with empty pattern = %v, want %v", got, want)
	}
}
