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

	code, stdout, stderr, err := runCapture(t, Options{Pattern: "world", Files: []string{path}})

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

	code, stdout, _, err := runCapture(t, Options{Pattern: "nonexistent", Files: []string{path}})

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
	code, _, stderr, err := runCapture(t, Options{Pattern: "(unclosed", Files: []string{"whatever"}})

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

	code, stdout, stderr, err := runCapture(t, Options{Pattern: "x", Files: []string{missing}})

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

	_, stdout, _, err := runCapture(t, Options{Pattern: "match", Files: []string{f1, f2}})
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

	_, stdout, _, err := runCapture(t, Options{Pattern: "match", Files: []string{f1}})
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

	_, stdout, _, err := runCapture(t, Options{Pattern: "match", Files: []string{f1}, WithFilename: true})
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

	_, stdout, _, err := runCapture(t, Options{Pattern: "match", Files: []string{f1, f2}, NoFilename: true})
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

	_, stdout, _, err := runCapture(t, Options{Pattern: "hello", Files: []string{path}, IgnoreCase: true})
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

	_, stdout, _, err := runCapture(t, Options{Pattern: "a.b", Files: []string{path}, Fixed: true})
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

	_, stdout, _, err := runCapture(t, Options{Pattern: "cat", Files: []string{path}, WordRegexp: true})
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

	_, stdout, _, err := runCapture(t, Options{Pattern: "hello", Files: []string{path}, LineRegexp: true})
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

	code, stdout, _, err := runCapture(t, Options{Pattern: "skip", Files: []string{path}, InvertMatch: true})
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

	_, stdout, _, err := runCapture(t, Options{Pattern: "match", Files: []string{path}, LineNumber: true})
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

	_, stdout, _, err := runCapture(t, Options{Pattern: "cat|dog", Files: []string{path}, OnlyMatching: true})
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

	code, stdout, _, err := runCapture(t, Options{Pattern: "match", Files: []string{f1, f2}, Count: true})
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

	code, stdout, _, err := runCapture(t, Options{Pattern: "match", Files: []string{f1, f2}, FilesWithMatches: true})
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

	code, stdout, _, err := runCapture(t, Options{Pattern: "match", Files: []string{f1, f2}, FilesWithoutMatch: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if code != ExitMatch {
		t.Errorf("code = %d, want ExitMatch (f2 has no match, so -L selects and prints it)", code)
	}
	if stdout != f2+"\n" {
		t.Errorf("stdout = %q, want %q", stdout, f2+"\n")
	}
}

// TestRun_FilesWithoutMatch_ExitCodeReflectsSelectionNotRawLineMatching
// is the regression test for a real bug caught by manual exercising
// (see OUTPUT.md): -L's exit code must reflect whether it *printed*
// anything (i.e. whether some file lacked a match), not whether any
// line matched anywhere. Previously, if every searched file contained
// the pattern, -L correctly printed nothing but still exited 0 as if it
// had found something — because the exit-code bookkeeping used the raw
// "this file matched" flag for every mode, including -L, instead of
// -L's own inverted selection criterion.
func TestRun_FilesWithoutMatch_ExitCodeReflectsSelectionNotRawLineMatching(t *testing.T) {
	dir := t.TempDir()
	// Every file matches — -L must select (and thus print) none of them.
	f1 := mustWriteTempFile(t, dir, "a.txt", "match\n")
	f2 := mustWriteTempFile(t, dir, "b.txt", "match\n")

	code, stdout, _, err := runCapture(t, Options{Pattern: "match", Files: []string{f1, f2}, FilesWithoutMatch: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty (every file matched, so -L selects nothing)", stdout)
	}
	if code != ExitNoMatch {
		t.Errorf("code = %d, want ExitNoMatch (nothing was selected by -L)", code)
	}
}

// TestRun_FilesWithMatches_ExitCodeReflectsSelectionNotRawLineMatching
// is -l's mirror image of the above: when NO file matches, -l must
// print nothing and exit ExitNoMatch.
func TestRun_FilesWithMatches_ExitCodeReflectsSelectionNotRawLineMatching(t *testing.T) {
	dir := t.TempDir()
	f1 := mustWriteTempFile(t, dir, "a.txt", "nope\n")
	f2 := mustWriteTempFile(t, dir, "b.txt", "nope\n")

	code, stdout, _, err := runCapture(t, Options{Pattern: "match", Files: []string{f1, f2}, FilesWithMatches: true})
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

	code, stdout, stderr, err := runCapture(t, Options{Pattern: "match", Files: []string{path}, Quiet: true})
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

	code, stdout, stderr, err := runCapture(t, Options{Pattern: "match", Files: []string{f1, f2}, Quiet: true})
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

	code, stdout, _, err := runCapture(t, Options{Pattern: "match", Files: []string{dir}, Recursive: true})
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

	code, stdout, stderr, err := runCapture(t, Options{Pattern: "match", Files: []string{dir}})
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
		Pattern:   "match",
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
		Pattern: "match",
		Files:   []string{filepath.Join(dir, "*.go")},
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

	code, stdout, _, err := runCapture(t, Options{Pattern: "world"})
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

	_, stdout, _, err := runCapture(t, Options{Pattern: "from", Files: []string{f1, "-"}})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	want := f1 + ":from-file\n" + "(standard input):from-stdin\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// --- color -----------------------------------------------------------------

func TestRun_ColorAlwaysForcesHighlightingEvenToANonFile(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "a.txt", "the cat sat\n")

	_, stdout, _, err := runCapture(t, Options{Pattern: "cat", Files: []string{path}, Color: ColorAlways})
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

	_, stdout, _, err := runCapture(t, Options{Pattern: "cat", Files: []string{path}, Color: ColorAuto})
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

	code, stdout, _, err := runCapture(t, Options{Pattern: "needle", Files: []string{path}})
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
