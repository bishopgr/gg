package grep

// Every expected-output string in this file was cross-checked directly
// against real GNU grep (Git for Windows' grep.exe) against the same
// fixture content, not derived from the man page or intuition — see the
// commit introducing -A/-B/-C support for the full transcript this was
// verified against.

import (
	"strings"
	"testing"
)

// ctxFixture is the 10-line fixture used across GNU-grep-verified
// context tests: two matches (lines 3 and 7) four lines apart, enough
// room on both sides to exercise leading/trailing context, merging, and
// separators without any single test needing its own bespoke file.
const ctxFixture = "line1\nline2\nMATCH_A\nline4\nline5\nline6\nMATCH_B\nline8\nline9\nline10\n"

func TestRun_Context_AfterOnly(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "f.txt", ctxFixture)

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"MATCH"}, Files: []string{path},
		ContextEnabled: true, ContextAfter: 2,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := "MATCH_A\nline4\nline5\n--\nMATCH_B\nline8\nline9\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestRun_Context_BeforeOnly(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "f.txt", ctxFixture)

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"MATCH"}, Files: []string{path},
		ContextEnabled: true, ContextBefore: 2,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := "line1\nline2\nMATCH_A\n--\nline5\nline6\nMATCH_B\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestRun_Context_BeforeAndAfterMergeWhenGroupsOverlap(t *testing.T) {
	// before=after=2 against matches 4 lines apart: each match's context
	// window touches the other's, so the two groups merge into one
	// unbroken run with no "--" separator, and shared lines (5,6) are
	// printed exactly once, not twice.
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "f.txt", ctxFixture)

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"MATCH"}, Files: []string{path},
		ContextEnabled: true, ContextBefore: 2, ContextAfter: 2,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := "line1\nline2\nMATCH_A\nline4\nline5\nline6\nMATCH_B\nline8\nline9\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestRun_Context_LineNumbersUseDashForContextAndColonForMatch(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "f.txt", ctxFixture)

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"MATCH"}, Files: []string{path}, LineNumber: true,
		ContextEnabled: true, ContextBefore: 2, ContextAfter: 2,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := "1-line1\n2-line2\n3:MATCH_A\n4-line4\n5-line5\n6-line6\n7:MATCH_B\n8-line8\n9-line9\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestRun_Context_FilenamePrefixUsesDashForContextAndColonForMatch(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "f.txt", ctxFixture)

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"MATCH"}, Files: []string{path}, LineNumber: true, WithFilename: true,
		ContextEnabled: true, ContextBefore: 1, ContextAfter: 1,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := path + "-2-line2\n" +
		path + ":3:MATCH_A\n" +
		path + "-4-line4\n" +
		"--\n" +
		path + "-6-line6\n" +
		path + ":7:MATCH_B\n" +
		path + "-8-line8\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestRun_Context_ExplicitZeroStillEnablesSeparators is the regression
// test for the single most surprising piece of GNU grep behavior this
// feature had to replicate: -A0 (or -C0) is NOT equivalent to omitting
// context entirely — it still turns on "--" separators between
// non-adjacent matches, even though zero actual context lines print.
func TestRun_Context_ExplicitZeroStillEnablesSeparators(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "f.txt", ctxFixture)

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"MATCH"}, Files: []string{path},
		ContextEnabled: true, ContextBefore: 0, ContextAfter: 0,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := "MATCH_A\n--\nMATCH_B\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestRun_Context_DisabledMatchesPriorBehaviorExactly(t *testing.T) {
	// Without ContextEnabled at all (the zero value), behavior must be
	// byte-identical to before this feature existed: no separators, ever.
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "f.txt", ctxFixture)

	_, stdout, _, err := runCapture(t, Options{Patterns: []string{"MATCH"}, Files: []string{path}})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := "MATCH_A\nMATCH_B\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestRun_Context_InvertedMatchShowsExcludedLineAsContext covers -v's
// interaction with context: the selection criterion flips (context is
// computed around lines that DON'T match), so a line that positively
// matches the pattern can itself surface as a context line around its
// non-matching neighbors, printed with the context ('-') prefix rather
// than being silently dropped.
func TestRun_Context_InvertedMatchShowsExcludedLineAsContext(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "f.txt", ctxFixture)

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"MATCH_A"}, Files: []string{path}, InvertMatch: true, LineNumber: true,
		ContextEnabled: true, ContextAfter: 1,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// Every line except MATCH_A is selected under -v; MATCH_A only shows
	// up because it falls within the trailing-context window of line 2.
	want := "1:line1\n2:line2\n3-MATCH_A\n4:line4\n5:line5\n6:line6\n7:MATCH_B\n8:line8\n9:line9\n10:line10\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestRun_Context_InvertedContextLineHighlightsItsOwnLiteralMatch pins
// down PrintContextLine's rawRanges plumbing end-to-end: MATCH_A above
// is excluded from selection by -v precisely because it DOES literally
// match, and when it resurfaces as a context line, that literal match
// must still be highlighted — verified directly against real grep.
func TestRun_Context_InvertedContextLineHighlightsItsOwnLiteralMatch(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "f.txt", ctxFixture)

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"MATCH_A"}, Files: []string{path}, InvertMatch: true,
		ContextEnabled: true, ContextAfter: 1, Color: ColorAlways,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !strings.Contains(stdout, "\x1b[1;31mMATCH_A\x1b[0m") {
		t.Errorf("stdout = %q, want the excluded-but-shown-as-context MATCH_A line to be highlighted", stdout)
	}
}

func TestRun_Context_OnlyMatchingDropsContextTextButKeepsSeparator(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "f.txt", ctxFixture)

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"MATCH"}, Files: []string{path}, OnlyMatching: true,
		ContextEnabled: true, ContextBefore: 1, ContextAfter: 1,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := "MATCH\n--\nMATCH\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestRun_Context_OnlyMatchingMergesWithoutSeparatorWhenGroupsOverlap(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "f.txt", ctxFixture)

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"MATCH"}, Files: []string{path}, OnlyMatching: true,
		ContextEnabled: true, ContextBefore: 3, ContextAfter: 3,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := "MATCH\nMATCH\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestRun_Context_CountModeIgnoresContextEntirely covers -c: context
// options must not affect the count, nor cause any context lines or
// separators to leak into count-mode output.
func TestRun_Context_CountModeIgnoresContextEntirely(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "f.txt", ctxFixture)

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"MATCH"}, Files: []string{path}, Count: true,
		ContextEnabled: true, ContextBefore: 2, ContextAfter: 2,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stdout != "2\n" {
		t.Errorf("stdout = %q, want %q", stdout, "2\n")
	}
}

// TestRun_Context_QuietModeUnaffected covers -q: context options must
// not change quiet mode's no-output/early-exit contract.
func TestRun_Context_QuietModeUnaffected(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "f.txt", ctxFixture)

	code, stdout, stderr, err := runCapture(t, Options{
		Patterns: []string{"MATCH"}, Files: []string{path}, Quiet: true,
		ContextEnabled: true, ContextBefore: 2, ContextAfter: 2,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitMatch {
		t.Errorf("code = %d, want ExitMatch", code)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("stdout=%q stderr=%q, want both empty under -q", stdout, stderr)
	}
}

// TestRun_Context_FilesWithMatchesUnaffected covers -l: only the
// filename should print, never context lines or separators.
func TestRun_Context_FilesWithMatchesUnaffected(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "f.txt", ctxFixture)

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"MATCH"}, Files: []string{path}, FilesWithMatches: true,
		ContextEnabled: true, ContextBefore: 2, ContextAfter: 2,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stdout != path+"\n" {
		t.Errorf("stdout = %q, want %q", stdout, path+"\n")
	}
}

// TestRun_Context_SeparatorBetweenDifferentFiles covers the one piece of
// this feature that isn't purely a per-file concern: real GNU grep
// treats a new file's first printed group as non-contiguous with
// whatever the previous file printed, emitting a "--" separator between
// them (verified directly), not just between groups within one file.
func TestRun_Context_SeparatorBetweenDifferentFiles(t *testing.T) {
	dir := t.TempDir()
	f1 := mustWriteTempFile(t, dir, "f.txt", ctxFixture)
	f2 := mustWriteTempFile(t, dir, "g.txt", "x1\nMATCH_C\nx3\n")

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"MATCH"}, Files: []string{f1, f2}, LineNumber: true,
		ContextEnabled: true, ContextBefore: 1, ContextAfter: 1,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := f1 + "-2-line2\n" +
		f1 + ":3:MATCH_A\n" +
		f1 + "-4-line4\n" +
		"--\n" +
		f1 + "-6-line6\n" +
		f1 + ":7:MATCH_B\n" +
		f1 + "-8-line8\n" +
		"--\n" +
		f2 + "-1-x1\n" +
		f2 + ":2:MATCH_C\n" +
		f2 + "-3-x3\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}
