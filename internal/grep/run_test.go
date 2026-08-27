package grep

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- shared test helpers -----------------------------------------------

func mustWriteTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func runCapture(t *testing.T, opts Options) (code int, stdout, stderr string, err error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	opts.Stdout = &outBuf
	opts.Stderr = &errBuf
	code, err = Run(context.Background(), opts)
	return code, outBuf.String(), errBuf.String(), err
}

// withStdin temporarily replaces os.Stdin for the duration of the
// calling test, restoring it afterward — the same pattern used in
// internal/source's tests.
func withStdin(t *testing.T, content string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })

	go func() {
		_, _ = w.WriteString(content)
		w.Close()
	}()
}

// --- basic matching ------------------------------------------------------

func TestRun_BasicMatch(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "a.txt", "hello\nworld\ngoodbye\n")

	code, stdout, stderr, err := runCapture(t, Options{Patterns: []string{"world"}, Files: []string{path}})

	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitMatch {
		t.Errorf("code = %d, want ExitMatch", code)
	}
	if stdout != "world\n" {
		t.Errorf("stdout = %q, want %q (no filename prefix for a single file)", stdout, "world\n")
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestRun_NoMatch(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "a.txt", "hello\nworld\n")

	code, stdout, _, err := runCapture(t, Options{Patterns: []string{"nonexistent"}, Files: []string{path}})

	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitNoMatch {
		t.Errorf("code = %d, want ExitNoMatch", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestRun_InvalidPatternIsAFatalReturnedError(t *testing.T) {
	code, _, stderr, err := runCapture(t, Options{Patterns: []string{"(unclosed"}, Files: []string{"whatever"}})

	if code != ExitError {
		t.Errorf("code = %d, want ExitError", code)
	}
	if err == nil {
		t.Error("err = nil, want a fatal error for an invalid pattern")
	}
	// This is the one-shot-fatal-error case: Run must NOT have printed
	// its own per-file diagnostic, since internal/cli's generic handler
	// is responsible for surfacing this particular error.
	if stderr != "" {
		t.Errorf("stderr = %q, want empty (fatal errors are returned, not self-reported)", stderr)
	}
}

func TestRun_MissingFileIsReportedNotReturned(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.txt")

	code, stdout, stderr, err := runCapture(t, Options{Patterns: []string{"x"}, Files: []string{missing}})

	if code != ExitError {
		t.Errorf("code = %d, want ExitError", code)
	}
	if err != nil {
		t.Errorf("err = %v, want nil (per-file errors are reported via Stderr, not returned)", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "No such file or directory") {
		t.Errorf("stderr = %q, want it to contain %q", stderr, "No such file or directory")
	}
}

// --- multi-file / filename prefixing -------------------------------------

func TestRun_MultipleFilesShowFilenamePrefixInGivenOrder(t *testing.T) {
	dir := t.TempDir()
	f1 := mustWriteTempFile(t, dir, "a.txt", "match\n")
	f2 := mustWriteTempFile(t, dir, "b.txt", "match\n")

	_, stdout, _, err := runCapture(t, Options{Patterns: []string{"match"}, Files: []string{f1, f2}})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := f1 + ":match\n" + f2 + ":match\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestRun_SingleFileHasNoFilenamePrefix(t *testing.T) {
	dir := t.TempDir()
	f1 := mustWriteTempFile(t, dir, "a.txt", "match\n")

	_, stdout, _, err := runCapture(t, Options{Patterns: []string{"match"}, Files: []string{f1}})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stdout != "match\n" {
		t.Errorf("stdout = %q, want %q", stdout, "match\n")
	}
}

func TestRun_WithFilenameForcesPrefixEvenForOneFile(t *testing.T) {
	dir := t.TempDir()
	f1 := mustWriteTempFile(t, dir, "a.txt", "match\n")

	_, stdout, _, err := runCapture(t, Options{Patterns: []string{"match"}, Files: []string{f1}, WithFilename: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := f1 + ":match\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestRun_NoFilenameSuppressesPrefixEvenForMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	f1 := mustWriteTempFile(t, dir, "a.txt", "match\n")
	f2 := mustWriteTempFile(t, dir, "b.txt", "match\n")

	_, stdout, _, err := runCapture(t, Options{Patterns: []string{"match"}, Files: []string{f1, f2}, NoFilename: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stdout != "match\nmatch\n" {
		t.Errorf("stdout = %q, want %q", stdout, "match\nmatch\n")
	}
}

// --- matching-flag plumbing (already unit-tested in internal/matcher;
// these just confirm Options actually reaches it) ------------------------

func TestRun_IgnoreCase(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "a.txt", "Hello World\n")

	_, stdout, _, err := runCapture(t, Options{Patterns: []string{"hello"}, Files: []string{path}, IgnoreCase: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stdout != "Hello World\n" {
		t.Errorf("stdout = %q, want the line to match case-insensitively", stdout)
	}
}

func TestRun_FixedStringTreatsMetacharactersLiterally(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "a.txt", "a.b\naxb\n")

	_, stdout, _, err := runCapture(t, Options{Patterns: []string{"a.b"}, Files: []string{path}, Fixed: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stdout != "a.b\n" {
		t.Errorf("stdout = %q, want only the literal \"a.b\" line (dot must not mean \"any char\")", stdout)
	}
}

func TestRun_WordRegexp(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "a.txt", "category\nthe cat sat\n")

	_, stdout, _, err := runCapture(t, Options{Patterns: []string{"cat"}, Files: []string{path}, WordRegexp: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stdout != "the cat sat\n" {
		t.Errorf("stdout = %q, want only the whole-word match", stdout)
	}
}

func TestRun_LineRegexp(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "a.txt", "hello world\nhello\n")

	_, stdout, _, err := runCapture(t, Options{Patterns: []string{"hello"}, Files: []string{path}, LineRegexp: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stdout != "hello\n" {
		t.Errorf("stdout = %q, want only the exact whole-line match", stdout)
	}
}

func TestRun_InvertMatch(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "a.txt", "keep\nskip\nkeep\n")

	code, stdout, _, err := runCapture(t, Options{Patterns: []string{"skip"}, Files: []string{path}, InvertMatch: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitMatch {
		t.Errorf("code = %d, want ExitMatch", code)
	}
	if stdout != "keep\nkeep\n" {
		t.Errorf("stdout = %q, want %q", stdout, "keep\nkeep\n")
	}
}

// --- output shaping -------------------------------------------------------

func TestRun_LineNumber(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "a.txt", "no\nmatch\nno\nmatch\n")

	_, stdout, _, err := runCapture(t, Options{Patterns: []string{"match"}, Files: []string{path}, LineNumber: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stdout != "2:match\n4:match\n" {
		t.Errorf("stdout = %q, want %q", stdout, "2:match\n4:match\n")
	}
}

func TestRun_OnlyMatching(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "a.txt", "cat and dog\n")

	_, stdout, _, err := runCapture(t, Options{Patterns: []string{"cat|dog"}, Files: []string{path}, OnlyMatching: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stdout != "cat\ndog\n" {
		t.Errorf("stdout = %q, want %q", stdout, "cat\ndog\n")
	}
}

func TestRun_Count(t *testing.T) {
	dir := t.TempDir()
	f1 := mustWriteTempFile(t, dir, "a.txt", "match\nno\nmatch\n")
	f2 := mustWriteTempFile(t, dir, "b.txt", "no\nno\n")

	code, stdout, _, err := runCapture(t, Options{Patterns: []string{"match"}, Files: []string{f1, f2}, Count: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitMatch {
		t.Errorf("code = %d, want ExitMatch", code)
	}
	want := f1 + ":2\n" + f2 + ":0\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestRun_FilesWithMatches(t *testing.T) {
	dir := t.TempDir()
	f1 := mustWriteTempFile(t, dir, "a.txt", "match\n")
	f2 := mustWriteTempFile(t, dir, "b.txt", "nope\n")

	code, stdout, _, err := runCapture(t, Options{Patterns: []string{"match"}, Files: []string{f1, f2}, FilesWithMatches: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitMatch {
		t.Errorf("code = %d, want ExitMatch", code)
	}
	if stdout != f1+"\n" {
		t.Errorf("stdout = %q, want %q", stdout, f1+"\n")
	}
}

func TestRun_FilesWithoutMatch(t *testing.T) {
	dir := t.TempDir()
	f1 := mustWriteTempFile(t, dir, "a.txt", "match\n")
	f2 := mustWriteTempFile(t, dir, "b.txt", "nope\n")

	code, stdout, _, err := runCapture(t, Options{Patterns: []string{"match"}, Files: []string{f1, f2}, FilesWithoutMatch: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// ExitMatch here because f1 raw-matched — not because -L printed
	// f2's name. See TestRun_FilesWithoutMatch_ExitCodeReflectsRawMatching
	// below for the case that actually pins down the distinction.
	if code != ExitMatch {
		t.Errorf("code = %d, want ExitMatch", code)
	}
	if stdout != f2+"\n" {
		t.Errorf("stdout = %q, want %q", stdout, f2+"\n")
	}
}

// TestRun_FilesWithoutMatch_ExitCodeReflectsRawMatching is the
// regression test for a real bug, found and corrected against real GNU
// grep (Git for Windows' grep.exe) as an oracle, not just against gg's
// own previously "plausible-looking" output: -l/-L's exit code is
// always identical to plain grep's own — 0 iff the pattern matched
// *some* line in *some* file — completely independent of which
// filenames -l/-L itself chooses to print.
//
// A prior version of this code got this backwards specifically for -L,
// setting the exit-code flag from `!matched` (this file's selection
// criterion) instead of `matched` (raw line-matching, the same
// criterion every other mode uses). That version's own regression test
// only checked a scenario where the two criteria happen to coincide
// (every file matches, so -L prints nothing — and, coincidentally,
// nothing matched-for-exit-code-purposes was true in that version's
// backwards logic too) without ever cross-checking against real grep,
// which actually exits 0 in that exact scenario (something *did*
// match), not the 1 the old test asserted.
func TestRun_FilesWithoutMatch_ExitCodeReflectsRawMatching(t *testing.T) {
	tests := []struct {
		name    string
		content map[string]string // filename -> content
		want    int
	}{
		{
			// A single file that does NOT match: -L prints its name
			// (there's something to report), but nothing ever matched,
			// so the exit code is still ExitNoMatch. This is the
			// concrete case the old code got wrong.
			name:    "single non-matching file: printed but ExitNoMatch",
			content: map[string]string{"a.txt": "nope\n"},
			want:    ExitNoMatch,
		},
		{
			// Both files match: -L prints nothing, but something DID
			// match, so the exit code is ExitMatch — the old code
			// asserted ExitNoMatch here.
			name:    "every file matches: nothing printed but ExitMatch",
			content: map[string]string{"a.txt": "match\n", "b.txt": "match\n"},
			want:    ExitMatch,
		},
		{
			// Neither file matches: -L prints both, and (consistently
			// with the single-file case) nothing matched anywhere, so
			// ExitNoMatch.
			name:    "no file matches: both printed, ExitNoMatch",
			content: map[string]string{"a.txt": "nope\n", "b.txt": "nope\n"},
			want:    ExitNoMatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			var files []string
			for _, name := range []string{"a.txt", "b.txt"} {
				content, ok := tt.content[name]
				if !ok {
					continue
				}
				files = append(files, mustWriteTempFile(t, dir, name, content))
			}

			code, _, _, err := runCapture(t, Options{Patterns: []string{"match"}, Files: files, FilesWithoutMatch: true})
			if err != nil {
				t.Fatalf("Run error: %v", err)
			}
			if code != tt.want {
				t.Errorf("code = %d, want %d", code, tt.want)
			}
		})
	}
}

// TestRun_FilesWithMatches_ExitCodeMatchesSelectionCriterion documents
// that -l was never affected by the -L bug above: -l's own selection
// criterion (list files that DO match) and the correct exit-code
// criterion (did the pattern match anywhere) are the same thing, so
// there was nothing to get backwards here. Kept as an explicit
// regression test rather than assumed from -L's fix.
func TestRun_FilesWithMatches_ExitCodeMatchesSelectionCriterion(t *testing.T) {
	dir := t.TempDir()
	f1 := mustWriteTempFile(t, dir, "a.txt", "nope\n")
	f2 := mustWriteTempFile(t, dir, "b.txt", "nope\n")

	code, stdout, _, err := runCapture(t, Options{Patterns: []string{"match"}, Files: []string{f1, f2}, FilesWithMatches: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty (no file matched, so -l selects nothing)", stdout)
	}
	if code != ExitNoMatch {
		t.Errorf("code = %d, want ExitNoMatch", code)
	}
}

func TestRun_Quiet_NoOutputButCorrectExitCode(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "a.txt", "match\n")

	code, stdout, stderr, err := runCapture(t, Options{Patterns: []string{"match"}, Files: []string{path}, Quiet: true})
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

// TestRun_Quiet_StopsSearchingAfterFirstMatch proves the early-exit
// optimization AND its correctness together: file2 doesn't exist, which
// would normally force ExitError — but because file1 (searched first)
// already matches under -q, gg must never even look at file2.
func TestRun_Quiet_StopsSearchingAfterFirstMatch(t *testing.T) {
	dir := t.TempDir()
	f1 := mustWriteTempFile(t, dir, "a.txt", "match\n")
	f2 := filepath.Join(dir, "does-not-exist.txt")

	code, stdout, stderr, err := runCapture(t, Options{Patterns: []string{"match"}, Files: []string{f1, f2}, Quiet: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitMatch {
		t.Errorf("code = %d, want ExitMatch (f2's missing-file error must never be reached)", code)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("stdout=%q stderr=%q, want both empty", stdout, stderr)
	}
}

// --- directory traversal --------------------------------------------------

func TestRun_RecursiveWalksSubdirectories(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	mustWriteTempFile(t, dir, "top.txt", "no\n")
	deep := mustWriteTempFile(t, sub, "deep.txt", "match\n")

	code, stdout, _, err := runCapture(t, Options{Patterns: []string{"match"}, Files: []string{dir}, Recursive: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitMatch {
		t.Errorf("code = %d, want ExitMatch", code)
	}
	want := deep + ":match\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestRun_NonRecursiveDirectoryReportsIsADirectory(t *testing.T) {
	dir := t.TempDir()
	mustWriteTempFile(t, dir, "a.txt", "match\n")

	code, stdout, stderr, err := runCapture(t, Options{Patterns: []string{"match"}, Files: []string{dir}})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitError {
		t.Errorf("code = %d, want ExitError", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "Is a directory") {
		t.Errorf("stderr = %q, want it to contain %q", stderr, "Is a directory")
	}
}

func TestRun_IncludeExcludeFilters(t *testing.T) {
	dir := t.TempDir()
	mustWriteTempFile(t, dir, "keep.go", "match\n")
	mustWriteTempFile(t, dir, "skip.md", "match\n")

	_, stdout, _, err := runCapture(t, Options{
		Patterns:  []string{"match"},
		Files:     []string{dir},
		Recursive: true,
		Include:   []string{"*.go"},
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !strings.Contains(stdout, "keep.go") || strings.Contains(stdout, "skip.md") {
		t.Errorf("stdout = %q, want only keep.go", stdout)
	}
}

// --- glob expansion (Windows shells don't do this themselves) -----------

func TestRun_GlobExpansionOnFileArguments(t *testing.T) {
	dir := t.TempDir()
	mustWriteTempFile(t, dir, "a.go", "match\n")
	mustWriteTempFile(t, dir, "b.go", "match\n")
	mustWriteTempFile(t, dir, "c.md", "match\n")

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"match"},
		Files:    []string{filepath.Join(dir, "*.go")},
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !strings.Contains(stdout, "a.go") || !strings.Contains(stdout, "b.go") || strings.Contains(stdout, "c.md") {
		t.Errorf("stdout = %q, want a.go and b.go but not c.md", stdout)
	}
}

// --- stdin -----------------------------------------------------------------

func TestRun_DefaultsToStdinWhenNoFilesGiven(t *testing.T) {
	withStdin(t, "hello\nworld\n")

	code, stdout, _, err := runCapture(t, Options{Patterns: []string{"world"}})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitMatch {
		t.Errorf("code = %d, want ExitMatch", code)
	}
	if stdout != "world\n" {
		t.Errorf("stdout = %q, want %q", stdout, "world\n")
	}
}

func TestRun_DashAmongFilesMeansStdin(t *testing.T) {
	withStdin(t, "from-stdin\n")
	dir := t.TempDir()
	f1 := mustWriteTempFile(t, dir, "a.txt", "from-file\n")

	_, stdout, _, err := runCapture(t, Options{Patterns: []string{"from"}, Files: []string{f1, "-"}})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := f1 + ":from-file\n" + "(standard input):from-stdin\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// --- multi-pattern (-e/-f) --------------------------------------------------

func TestRun_MultiplePatternsFromLiteralList(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "a.txt", "cat\ndog\nfish\n")

	code, stdout, _, err := runCapture(t, Options{Patterns: []string{"cat", "dog"}, Files: []string{path}})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitMatch {
		t.Errorf("code = %d, want ExitMatch", code)
	}
	if stdout != "cat\ndog\n" {
		t.Errorf("stdout = %q, want %q", stdout, "cat\ndog\n")
	}
}

func TestRun_PatternsFromFile(t *testing.T) {
	dir := t.TempDir()
	target := mustWriteTempFile(t, dir, "a.txt", "cat\ndog\nfish\n")
	patFile := mustWriteTempFile(t, dir, "patterns.txt", "cat\ndog\n")

	_, stdout, _, err := runCapture(t, Options{PatternFiles: []string{patFile}, Files: []string{target}})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stdout != "cat\ndog\n" {
		t.Errorf("stdout = %q, want %q", stdout, "cat\ndog\n")
	}
}

func TestRun_PatternsFileAndLiteralCombine(t *testing.T) {
	dir := t.TempDir()
	target := mustWriteTempFile(t, dir, "a.txt", "cat\ndog\nfish\n")
	patFile := mustWriteTempFile(t, dir, "patterns.txt", "dog\n")

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"cat"}, PatternFiles: []string{patFile}, Files: []string{target},
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stdout != "cat\ndog\n" {
		t.Errorf("stdout = %q, want %q", stdout, "cat\ndog\n")
	}
}

// TestRun_MissingPatternFileIsFatalReturnedError mirrors
// TestRun_InvalidPatternIsAFatalReturnedError: with no readable pattern
// list, the whole command fails, and internal/cli's generic "gg: <err>"
// handler is responsible for printing it — Run itself must not also
// print a per-file diagnostic.
func TestRun_MissingPatternFileIsFatalReturnedError(t *testing.T) {
	dir := t.TempDir()
	target := mustWriteTempFile(t, dir, "a.txt", "cat\n")
	missing := filepath.Join(dir, "does-not-exist.txt")

	code, _, stderr, err := runCapture(t, Options{PatternFiles: []string{missing}, Files: []string{target}})
	if code != ExitError {
		t.Errorf("code = %d, want ExitError", code)
	}
	if err == nil {
		t.Error("err = nil, want a fatal error for a missing pattern file")
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty (fatal errors are returned, not self-reported)", stderr)
	}
}

// TestRun_EmptyPatternFileWithNoLiteralPatternsNeverMatches is the
// end-to-end regression test for the single most surprising piece of
// -e/-f behavior, verified directly against real GNU grep: zero total
// patterns means "match nothing," not "match everything" the way an
// actually-empty regex pattern would.
func TestRun_EmptyPatternFileWithNoLiteralPatternsNeverMatches(t *testing.T) {
	dir := t.TempDir()
	target := mustWriteTempFile(t, dir, "a.txt", "anything\nat all\n")
	empty := mustWriteTempFile(t, dir, "empty.txt", "")

	code, stdout, _, err := runCapture(t, Options{PatternFiles: []string{empty}, Files: []string{target}})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitNoMatch {
		t.Errorf("code = %d, want ExitNoMatch", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

// --- color -----------------------------------------------------------------

func TestRun_ColorAlwaysForcesHighlightingEvenToANonFile(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "a.txt", "the cat sat\n")

	_, stdout, _, err := runCapture(t, Options{Patterns: []string{"cat"}, Files: []string{path}, Color: ColorAlways})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := "the \x1b[1;31mcat\x1b[0m sat\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestRun_ColorAutoDisabledForNonFileWriter(t *testing.T) {
	// runCapture's Stdout is a *bytes.Buffer, not a *os.File, so
	// ColorAuto's terminal check can never succeed for it — proving
	// resolveColor fails closed rather than assuming.
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "a.txt", "the cat sat\n")

	_, stdout, _, err := runCapture(t, Options{Patterns: []string{"cat"}, Files: []string{path}, Color: ColorAuto})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("stdout = %q, want no ANSI codes under ColorAuto to a non-file writer", stdout)
	}
}

// --- binary files ------------------------------------------------------------

func TestRun_BinaryFilePrintsMatchMessageNotRawLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	content := append([]byte("prefix\x00needle\x00suffix"), 0)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	code, stdout, _, err := runCapture(t, Options{Patterns: []string{"needle"}, Files: []string{path}})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitMatch {
		t.Errorf("code = %d, want ExitMatch", code)
	}
	want := "Binary file " + path + " matches\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// mustWriteBinaryFile is mustWriteTempFile's raw-bytes counterpart, for
// content that needs an embedded NUL to trip encoding.Sniff's binary
// heuristic — a plain string literal can't hold that unambiguously
// across the different test call sites below.
func mustWriteBinaryFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

// TestRun_BinaryText_TreatsBinaryFileExactlyLikeText is -a/--text's
// core behavior, verified directly against real GNU grep: raw line
// content prints, NUL bytes and all, instead of the "Binary file X
// matches" message.
func TestRun_BinaryText_TreatsBinaryFileExactlyLikeText(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteBinaryFile(t, dir, "data.bin", []byte("prefix\x00needle\x00suffix\nsecond\x00line\n"))

	code, stdout, _, err := runCapture(t, Options{Patterns: []string{"needle"}, Files: []string{path}, Binary: BinaryText})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitMatch {
		t.Errorf("code = %d, want ExitMatch", code)
	}
	want := "prefix\x00needle\x00suffix\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestRun_BinaryText_ContextLinesWorkNormally verifies -a composes with
// -A/-B/-C — a binary-detected file treated as text gets ordinary
// context-line behavior, verified directly against real GNU grep
// (`grep -a -C 1`).
func TestRun_BinaryText_ContextLinesWorkNormally(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteBinaryFile(t, dir, "data.bin", []byte("prefix\x00needle\x00suffix\nsecond\x00line\n"))

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"needle"}, Files: []string{path}, Binary: BinaryText,
		ContextEnabled: true, ContextAfter: 1,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := "prefix\x00needle\x00suffix\nsecond\x00line\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestRun_BinaryText_InvertMatchWorksNormally verifies -a composes with
// -v, verified directly against real GNU grep (`grep -a -v`).
func TestRun_BinaryText_InvertMatchWorksNormally(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteBinaryFile(t, dir, "data.bin", []byte("prefix\x00needle\x00suffix\nsecond\x00line\n"))

	_, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"needle"}, Files: []string{path}, Binary: BinaryText, InvertMatch: true,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := "second\x00line\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestRun_BinarySkip_TreatsFileAsNoMatchWithoutReadingIt is -I's core
// behavior, verified directly against real GNU grep: a binary-detected
// file contributes nothing at all — no output, no match — as if it were
// never searched.
func TestRun_BinarySkip_TreatsFileAsNoMatchWithoutReadingIt(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteBinaryFile(t, dir, "data.bin", []byte("prefix\x00needle\x00suffix\n"))

	code, stdout, stderr, err := runCapture(t, Options{Patterns: []string{"needle"}, Files: []string{path}, Binary: BinarySkip})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitNoMatch {
		t.Errorf("code = %d, want ExitNoMatch", code)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("stdout=%q stderr=%q, want both empty", stdout, stderr)
	}
}

// TestRun_BinarySkip_OtherFilesStillSearchedNormally verifies -I only
// skips the binary file(s), continuing to search the rest of a
// multi-file run exactly as if -I hadn't been given — verified directly
// against real GNU grep.
func TestRun_BinarySkip_OtherFilesStillSearchedNormally(t *testing.T) {
	dir := t.TempDir()
	binPath := mustWriteBinaryFile(t, dir, "data.bin", []byte("needle\x00here\n"))
	textPath := mustWriteTempFile(t, dir, "text.txt", "needle here\n")

	code, stdout, _, err := runCapture(t, Options{
		Patterns: []string{"needle"}, Files: []string{binPath, textPath}, Binary: BinarySkip, LineNumber: true,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitMatch {
		t.Errorf("code = %d, want ExitMatch", code)
	}
	want := textPath + ":1:needle here\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestRun_BinarySkip_CountReportsZero verifies -I's -c interaction,
// verified directly against real GNU grep (`grep -Ic` reports 0 for a
// binary file, not the raw in-file match count).
func TestRun_BinarySkip_CountReportsZero(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteBinaryFile(t, dir, "data.bin", []byte("needle\x00here\nneedle\x00again\n"))

	_, stdout, _, err := runCapture(t, Options{Patterns: []string{"needle"}, Files: []string{path}, Binary: BinarySkip, Count: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stdout != "0\n" {
		t.Errorf("stdout = %q, want %q", stdout, "0\n")
	}
}

// TestRun_BinarySkip_FilesWithMatchesNeverListsIt verifies -I's -l
// interaction, verified directly against real GNU grep.
func TestRun_BinarySkip_FilesWithMatchesNeverListsIt(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteBinaryFile(t, dir, "data.bin", []byte("needle\x00here\n"))

	code, stdout, _, err := runCapture(t, Options{Patterns: []string{"needle"}, Files: []string{path}, Binary: BinarySkip, FilesWithMatches: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitNoMatch {
		t.Errorf("code = %d, want ExitNoMatch", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

// TestRun_BinarySkip_FilesWithoutMatchListsItButExitsNoMatch is the
// precise interaction this feature's own -L exit-code fix (see
// TestRun_FilesWithoutMatch_ExitCodeReflectsRawMatching) had to get
// right: -I -L prints the skipped file's name (nothing else disqualifies
// it as "not matching"), but the overall exit code must still be
// ExitNoMatch, since nothing was ever actually matched — verified
// directly against real GNU grep.
func TestRun_BinarySkip_FilesWithoutMatchListsItButExitsNoMatch(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteBinaryFile(t, dir, "data.bin", []byte("needle\x00here\n"))

	code, stdout, _, err := runCapture(t, Options{Patterns: []string{"needle"}, Files: []string{path}, Binary: BinarySkip, FilesWithoutMatch: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitNoMatch {
		t.Errorf("code = %d, want ExitNoMatch", code)
	}
	if stdout != path+"\n" {
		t.Errorf("stdout = %q, want %q", stdout, path+"\n")
	}
}

// TestRun_BinarySkip_InvertMatchMakesNoDifference verifies -I's -v
// interaction: the file is skipped outright regardless of -v, verified
// directly against real GNU grep (`grep -I -v` on a binary file still
// exits 1, same as without -v).
func TestRun_BinarySkip_InvertMatchMakesNoDifference(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteBinaryFile(t, dir, "data.bin", []byte("needle\x00here\n"))

	code, stdout, _, err := runCapture(t, Options{Patterns: []string{"needle"}, Files: []string{path}, Binary: BinarySkip, InvertMatch: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitNoMatch {
		t.Errorf("code = %d, want ExitNoMatch", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}
