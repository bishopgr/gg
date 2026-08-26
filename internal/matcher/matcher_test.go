package matcher

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// TestNew_DispatchesToFixedFastPath asserts that a bare -F pattern (no
// -i/-w/-x) gets the byte-scanning fixedMatcher, not a compiled regexp —
// this is the fast path New documents.
func TestNew_DispatchesToFixedFastPath(t *testing.T) {
	m, err := New(Options{Patterns: []string{"hello.world"}, Fixed: true})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if _, ok := m.(*fixedMatcher); !ok {
		t.Fatalf("New() with plain -F returned %T, want *fixedMatcher", m)
	}

	// The dot must be treated literally, not as "any character" — proof
	// this isn't secretly a regexp.
	if got := m.Find([]byte("helloXworld")); got != nil {
		t.Errorf("Find(%q) = %v, want nil (dot must be literal under -F)", "helloXworld", got)
	}
	if got := m.Find([]byte("hello.world")); got == nil {
		t.Errorf("Find(%q) = nil, want a match on the literal dot", "hello.world")
	}
}

// TestNew_FixedWithModifiersUsesRegexEngine asserts that combining -F
// with -i/-w/-x routes through the regex engine (for correct Unicode case
// folding and boundary handling) while still treating the pattern as a
// literal string, not a regular expression.
func TestNew_FixedWithModifiersUsesRegexEngine(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{"fixed + ignore case", Options{Patterns: []string{"a.b"}, Fixed: true, IgnoreCase: true}},
		{"fixed + word regexp", Options{Patterns: []string{"a.b"}, Fixed: true, WordRegexp: true}},
		{"fixed + line regexp", Options{Patterns: []string{"a.b"}, Fixed: true, LineRegexp: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := New(tt.opts)
			if err != nil {
				t.Fatalf("New(%+v) error: %v", tt.opts, err)
			}
			if _, ok := m.(*regexMatcher); !ok {
				t.Fatalf("New(%+v) returned %T, want *regexMatcher", tt.opts, m)
			}
			// "a.b" must still mean the literal three characters "a.b",
			// not "a" + any-char + "b" — regexp.QuoteMeta must have run.
			if got := m.Find([]byte("aXb")); got != nil {
				t.Errorf("Find(%q) = %v, want nil (dot must stay literal)", "aXb", got)
			}
		})
	}
}

func TestNew_InvalidRegexReturnsNilInterfaceAndError(t *testing.T) {
	m, err := New(Options{Patterns: []string{"(unclosed"}})
	if err == nil {
		t.Fatal("expected an error for an unbalanced regex, got nil")
	}
	// Guards against the typed-nil-in-interface bug: m must compare
	// equal to nil, not be a non-nil interface wrapping a nil pointer.
	if m != nil {
		t.Fatalf("expected a nil Matcher on error, got %#v", m)
	}
}

// --- multi-pattern (-e/-f) ---------------------------------------------

// TestNew_EmptyPatternsNeverMatches is the regression test for the one
// genuinely surprising piece of -e/-f behavior verified directly against
// real GNU grep: an empty Patterns slice (e.g. -f on an empty file with
// no -e at all) matches *nothing*, not "matches every line" the way
// compiling a literal empty-string regex would.
func TestNew_EmptyPatternsNeverMatches(t *testing.T) {
	m, err := New(Options{Patterns: nil})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	for _, line := range [][]byte{[]byte(""), []byte("anything")} {
		if got := m.Find(line); got != nil {
			t.Errorf("Find(%q) = %v, want nil (zero patterns must never match)", line, got)
		}
	}
}

func TestNew_MultiplePatternsOrTogether(t *testing.T) {
	m, err := New(Options{Patterns: []string{"cat", "dog"}})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if got := m.Find([]byte("the cat sat")); got == nil {
		t.Error("Find(\"the cat sat\") = nil, want a match on \"cat\"")
	}
	if got := m.Find([]byte("the dog ran")); got == nil {
		t.Error("Find(\"the dog ran\") = nil, want a match on \"dog\"")
	}
	if got := m.Find([]byte("the fish swam")); got != nil {
		t.Errorf("Find(\"the fish swam\") = %v, want nil", got)
	}
}

// TestNew_MultiplePatternsFixedTreatsEachAsLiteral verifies -F applies to
// every pattern in the list individually, not just the first — verified
// directly against real GNU grep (`grep -F -e "a.b" -e "date"`).
func TestNew_MultiplePatternsFixedTreatsEachAsLiteral(t *testing.T) {
	m, err := New(Options{Patterns: []string{"a.b", "date"}, Fixed: true})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if got := m.Find([]byte("aXb")); got != nil {
		t.Errorf("Find(\"aXb\") = %v, want nil (dot must stay literal under -F)", got)
	}
	if got := m.Find([]byte("a.b")); got == nil {
		t.Error("Find(\"a.b\") = nil, want a literal match")
	}
	if got := m.Find([]byte("update")); got == nil {
		t.Error("Find(\"update\") = nil, want a match on the literal \"date\"")
	}
}

// TestNew_MultiplePatternsWithLineRegexp verifies -x anchors the whole
// alternation, requiring the entire line to equal *some* pattern in the
// list — verified directly against real GNU grep (`grep -x -e apple -e
// cherry`).
func TestNew_MultiplePatternsWithLineRegexp(t *testing.T) {
	m, err := New(Options{Patterns: []string{"apple", "cherry"}, LineRegexp: true})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if got := m.Find([]byte("apple")); got == nil {
		t.Error("Find(\"apple\") = nil, want a whole-line match")
	}
	if got := m.Find([]byte("apple pie")); got != nil {
		t.Errorf("Find(\"apple pie\") = %v, want nil (must match the whole line)", got)
	}
}

// TestNew_MultiplePatternsWithWordRegexp verifies -w applies word
// boundaries to the whole alternation — verified directly against real
// GNU grep (`grep -w -e cat -e dog`).
func TestNew_MultiplePatternsWithWordRegexp(t *testing.T) {
	m, err := New(Options{Patterns: []string{"cat", "dog"}, WordRegexp: true})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if got := m.Find([]byte("category error")); got != nil {
		t.Errorf("Find(\"category error\") = %v, want nil (cat must be a whole word)", got)
	}
	if got := m.Find([]byte("a dog ran")); got == nil {
		t.Error("Find(\"a dog ran\") = nil, want a whole-word match on \"dog\"")
	}
}

func TestNew_MultiplePatternsInvalidRegexIsAttributedToItsOwnPattern(t *testing.T) {
	_, err := New(Options{Patterns: []string{"valid", "(unclosed"}})
	if err == nil {
		t.Fatal("expected an error for the unbalanced second pattern, got nil")
	}
	if !strings.Contains(err.Error(), "(unclosed") {
		t.Errorf("error = %q, want it to mention the specific bad pattern %q", err.Error(), "(unclosed")
	}
}

// TestFixedAndRegexAgree cross-validates the two implementations: for any
// plain literal pattern (no -i/-w/-x), fixedMatcher and a regexMatcher
// built from regexp.QuoteMeta(pattern) must report identical ranges. This
// pins down that the "fast path" and "general path" in New are actually
// equivalent where their behavior is supposed to overlap.
func TestFixedAndRegexAgree(t *testing.T) {
	cases := []struct {
		pattern, line string
	}{
		{"cat", "the cat sat on the cat mat"},
		{"a.b", "a.b axb a.b.a.b"},
		{"", "abc"},
		{"x", ""},
		{"([special])", "text with ([special]) chars"},
		{"aa", "aaaaa"},
	}

	for _, tc := range cases {
		fixed := newFixedMatcher(tc.pattern)
		re, err := newRegexMatcher(regexp.QuoteMeta(tc.pattern), Options{})
		if err != nil {
			t.Fatalf("newRegexMatcher(QuoteMeta(%q), ...) error: %v", tc.pattern, err)
		}

		got := fixed.Find([]byte(tc.line))
		want := re.Find([]byte(tc.line))
		if !reflect.DeepEqual(got, want) {
			t.Errorf("pattern %q, line %q: fixedMatcher=%v, regexMatcher=%v (disagree)",
				tc.pattern, tc.line, got, want)
		}
	}
}
