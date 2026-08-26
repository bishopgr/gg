package matcher

import (
	"reflect"
	"regexp"
	"testing"
)

// TestNew_DispatchesToFixedFastPath asserts that a bare -F pattern (no
// -i/-w/-x) gets the byte-scanning fixedMatcher, not a compiled regexp —
// this is the fast path New documents.
func TestNew_DispatchesToFixedFastPath(t *testing.T) {
	m, err := New(Options{Pattern: "hello.world", Fixed: true})
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
		{"fixed + ignore case", Options{Pattern: "a.b", Fixed: true, IgnoreCase: true}},
		{"fixed + word regexp", Options{Pattern: "a.b", Fixed: true, WordRegexp: true}},
		{"fixed + line regexp", Options{Pattern: "a.b", Fixed: true, LineRegexp: true}},
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
	m, err := New(Options{Pattern: "(unclosed"})
	if err == nil {
		t.Fatal("expected an error for an unbalanced regex, got nil")
	}
	// Guards against the typed-nil-in-interface bug: m must compare
	// equal to nil, not be a non-nil interface wrapping a nil pointer.
	if m != nil {
		t.Fatalf("expected a nil Matcher on error, got %#v", m)
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
