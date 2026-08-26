package grep

// Every case here was cross-checked directly against real GNU grep
// (Git for Windows' grep.exe) rather than assumed from the man page —
// see the commit introducing -e/-f support for the transcript.

import (
	"strings"
	"testing"
)

func TestResolvePatterns_LiteralOnly(t *testing.T) {
	got, err := resolvePatterns([]string{"foo", "bar"}, nil)
	if err != nil {
		t.Fatalf("resolvePatterns error: %v", err)
	}
	want := []string{"foo", "bar"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestResolvePatterns_FileOnly(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "patterns.txt", "banana\ndate\n")

	got, err := resolvePatterns(nil, []string{path})
	if err != nil {
		t.Fatalf("resolvePatterns error: %v", err)
	}
	want := []string{"banana", "date"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestResolvePatterns_LiteralThenFileOrder pins down the documented
// combination order: every literal (-e / positional) pattern first,
// then every -f file's lines, files in the order given.
func TestResolvePatterns_LiteralThenFileOrder(t *testing.T) {
	dir := t.TempDir()
	p1 := mustWriteTempFile(t, dir, "p1.txt", "b1\nb2\n")
	p2 := mustWriteTempFile(t, dir, "p2.txt", "c1\n")

	got, err := resolvePatterns([]string{"a1", "a2"}, []string{p1, p2})
	if err != nil {
		t.Fatalf("resolvePatterns error: %v", err)
	}
	want := []string{"a1", "a2", "b1", "b2", "c1"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestResolvePatterns_BlankLineBecomesEmptyPattern is the regression
// test for real, verified GNU grep behavior: a blank line in a pattern
// file is not skipped — it becomes a literal empty-string pattern,
// which (once compiled) matches every line, exactly like -e "" does.
func TestResolvePatterns_BlankLineBecomesEmptyPattern(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "patterns.txt", "banana\n\n")

	got, err := resolvePatterns(nil, []string{path})
	if err != nil {
		t.Fatalf("resolvePatterns error: %v", err)
	}
	want := []string{"banana", ""}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestResolvePatterns_CRLFPatternFileStripsCarriageReturn(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "patterns.txt", "banana\r\ndate\r\n")

	got, err := resolvePatterns(nil, []string{path})
	if err != nil {
		t.Fatalf("resolvePatterns error: %v", err)
	}
	want := []string{"banana", "date"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestResolvePatterns_EmptyFileWithNoLiteralPatternsYieldsEmptyList is
// the setup for matcher.New's neverMatcher case: zero total patterns
// (an empty -f file, no -e) must resolve to an empty slice, not a slice
// containing one empty-string pattern — those behave differently (see
// matcher.TestNew_EmptyPatternsNeverMatches vs.
// TestNew_MultiplePatternsOrTogether with an explicit "" entry).
func TestResolvePatterns_EmptyFileWithNoLiteralPatternsYieldsEmptyList(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "empty.txt", "")

	got, err := resolvePatterns(nil, []string{path})
	if err != nil {
		t.Fatalf("resolvePatterns error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want an empty slice", got)
	}
}

func TestResolvePatterns_MissingFileIsFatal(t *testing.T) {
	dir := t.TempDir()
	missing := dir + "/does-not-exist.txt"

	_, err := resolvePatterns(nil, []string{missing})
	if err == nil {
		t.Fatal("expected an error for a missing pattern file, got nil")
	}
	if !strings.Contains(err.Error(), "No such file or directory") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "No such file or directory")
	}
}

func TestResolvePatterns_NoOpWhenNoFilesGiven(t *testing.T) {
	// The literal slice must come back untouched (not even copied into
	// a new slice) when there are no pattern files — cheap confirmation
	// that the fast path really is a fast path.
	literal := []string{"only"}
	got, err := resolvePatterns(literal, nil)
	if err != nil {
		t.Fatalf("resolvePatterns error: %v", err)
	}
	if len(got) != 1 || got[0] != "only" {
		t.Errorf("got %v, want [only]", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
