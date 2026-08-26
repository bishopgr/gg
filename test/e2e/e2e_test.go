// Package e2e is the black-box layer PROCESS.md §5 calls for: it builds
// the real gg.exe and invokes it as a subprocess, so it actually
// validates process/exit-code/stdio-redirection behavior (§3.3/§3.5)
// that no in-process unit test can — internal/grep's tests call Run
// directly and never touch Cobra's argument parsing, the real OS
// process boundary, or actual stdio redirection.
package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gg-e2e-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	name := "gg"
	if runtime.GOOS == "windows" {
		name = "gg.exe"
	}
	binPath = filepath.Join(dir, name)

	build := exec.Command("go", "build", "-o", binPath, "./cmd/gg")
	build.Dir = moduleRoot()
	if out, err := build.CombinedOutput(); err != nil {
		panic("building gg for e2e tests failed: " + err.Error() + "\n" + string(out))
	}

	os.Exit(m.Run())
}

func moduleRoot() string {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		panic(err)
	}
	return root
}

func testdataPath(name string) string {
	return filepath.Join(moduleRoot(), "test", "testdata", name)
}

type result struct {
	stdout, stderr string
	exitCode       int
}

func run(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("failed to run %s: %v", binPath, err)
		}
		exitCode = exitErr.ExitCode()
	}

	return result{stdout: outBuf.String(), stderr: errBuf.String(), exitCode: exitCode}
}

func TestE2E_MatchFound(t *testing.T) {
	r := run(t, "", "line two", testdataPath("lf.txt"))
	if r.exitCode != 0 {
		t.Errorf("exit code = %d, want 0; stderr=%q", r.exitCode, r.stderr)
	}
	if r.stdout != "line two\n" {
		t.Errorf("stdout = %q, want %q", r.stdout, "line two\n")
	}
}

func TestE2E_NoMatch(t *testing.T) {
	r := run(t, "", "nonexistent-pattern-xyz", testdataPath("lf.txt"))
	if r.exitCode != 1 {
		t.Errorf("exit code = %d, want 1", r.exitCode)
	}
	if r.stdout != "" {
		t.Errorf("stdout = %q, want empty", r.stdout)
	}
}

func TestE2E_MissingFileExitsTwoWithDiagnostic(t *testing.T) {
	missing := filepath.Join(moduleRoot(), "test", "testdata", "does-not-exist.txt")
	r := run(t, "", "pattern", missing)
	if r.exitCode != 2 {
		t.Errorf("exit code = %d, want 2", r.exitCode)
	}
	if !strings.Contains(r.stderr, "No such file or directory") {
		t.Errorf("stderr = %q, want it to mention the file is missing", r.stderr)
	}
}

func TestE2E_NoArgsExitsTwo(t *testing.T) {
	r := run(t, "")
	if r.exitCode != 2 {
		t.Errorf("exit code = %d, want 2", r.exitCode)
	}
}

func TestE2E_VersionIsLongFlagOnlyAndExitsZero(t *testing.T) {
	r := run(t, "", "--version")
	if r.exitCode != 0 {
		t.Errorf("exit code = %d, want 0", r.exitCode)
	}
	if !strings.HasPrefix(r.stdout, "gg version") {
		t.Errorf("stdout = %q, want it to start with %q", r.stdout, "gg version")
	}
}

func TestE2E_HelpIsLongFlagOnlyAndExitsZero(t *testing.T) {
	r := run(t, "", "--help")
	if r.exitCode != 0 {
		t.Errorf("exit code = %d, want 0", r.exitCode)
	}
	if !strings.Contains(r.stdout, "Usage:") {
		t.Errorf("stdout = %q, want it to contain usage text", r.stdout)
	}
}

// TestE2E_ShortHMeansNoFilenameNotHelp is the real-process version of
// the in-process cli test: -h must not trigger help output at all.
func TestE2E_ShortHMeansNoFilenameNotHelp(t *testing.T) {
	r := run(t, "", "-rh", "line one", filepath.Join(moduleRoot(), "test", "testdata"))
	if r.exitCode != 0 {
		t.Errorf("exit code = %d, want 0; stderr=%q", r.exitCode, r.stderr)
	}
	if strings.Contains(r.stdout, "Usage:") {
		t.Error("stdout contains help usage text — -h triggered help instead of --no-filename")
	}
	// -h suppresses filenames even though multiple files matched
	// recursively, so every printed line should be exactly "line one".
	for _, line := range strings.Split(strings.TrimRight(r.stdout, "\n"), "\n") {
		if line != "line one" {
			t.Errorf("line %q contains a filename prefix, want -h to have suppressed it", line)
		}
	}
}

// TestE2E_ShortVMeansInvertMatchNotVersion is the real-process version
// of the in-process cli test.
func TestE2E_ShortVMeansInvertMatchNotVersion(t *testing.T) {
	r := run(t, "", "-v", "line two", testdataPath("lf.txt"))
	if r.exitCode != 0 {
		t.Errorf("exit code = %d, want 0; stderr=%q", r.exitCode, r.stderr)
	}
	if strings.HasPrefix(r.stdout, "gg version") {
		t.Error("stdout looks like version output — -v triggered version instead of --invert-match")
	}
	want := "line one\nline three\n"
	if r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestE2E_StdinDefaultWhenNoFilesGiven(t *testing.T) {
	r := run(t, "hello\nworld\n", "world")
	if r.exitCode != 0 {
		t.Errorf("exit code = %d, want 0; stderr=%q", r.exitCode, r.stderr)
	}
	if r.stdout != "world\n" {
		t.Errorf("stdout = %q, want %q", r.stdout, "world\n")
	}
}

func TestE2E_RecursiveSearchUsesNativePathSeparator(t *testing.T) {
	r := run(t, "", "-r", "line one", filepath.Join(moduleRoot(), "test", "testdata"))
	if r.exitCode != 0 {
		t.Errorf("exit code = %d, want 0; stderr=%q", r.exitCode, r.stderr)
	}
	if runtime.GOOS == "windows" && !strings.Contains(r.stdout, "testdata\\") {
		t.Errorf("stdout = %q, want it to contain a backslash-separated path on Windows", r.stdout)
	}
}

// TestE2E_OutputAlwaysEndsLinesWithBareLF is the real-process proof of
// the PROCESS.md §3.1 decision: gg must never write '\r\n', even on
// Windows, even though the process is running natively there.
func TestE2E_OutputAlwaysEndsLinesWithBareLF(t *testing.T) {
	r := run(t, "", "-r", "line", filepath.Join(moduleRoot(), "test", "testdata"))
	if r.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", r.exitCode, r.stderr)
	}
	if strings.Contains(r.stdout, "\r") {
		t.Errorf("stdout contains a CR byte — output must always be bare-LF-terminated, got %q", r.stdout)
	}
}

func TestE2E_MutuallyExclusiveFlagsExitTwo(t *testing.T) {
	r := run(t, "", "-l", "-L", "pattern", testdataPath("lf.txt"))
	if r.exitCode != 2 {
		t.Errorf("exit code = %d, want 2", r.exitCode)
	}
}

func TestE2E_ColorAlwaysEmitsAnsiCodesEvenWhenRedirected(t *testing.T) {
	// Redirected to a pipe (as this test does), --color=auto would stay
	// off — --color=always must force it on regardless.
	r := run(t, "", "--color=always", "two", testdataPath("lf.txt"))
	if r.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", r.exitCode, r.stderr)
	}
	if !strings.Contains(r.stdout, "\x1b[1;31m") {
		t.Errorf("stdout = %q, want it to contain the ANSI highlight start code", r.stdout)
	}
}

func TestE2E_ColorAutoDoesNotEmitAnsiWhenRedirected(t *testing.T) {
	r := run(t, "", "two", testdataPath("lf.txt")) // --color defaults to auto
	if r.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", r.exitCode, r.stderr)
	}
	if strings.Contains(r.stdout, "\x1b[") {
		t.Errorf("stdout = %q, want no ANSI codes when redirected under --color=auto", r.stdout)
	}
}

// --- context lines (-A/-B/-C) ----------------------------------------------
//
// These pin down -A/-B/-C at the real process boundary — flag parsing
// through Cobra, real stdio, real exit codes — on top of the already-
// thorough in-process coverage in internal/grep and internal/cli.
// Expected output was verified directly against real GNU grep (Git for
// Windows' grep.exe) against identical input, not derived from the man
// page.

const ctxFixture = "line1\nline2\nMATCH_A\nline4\nline5\nline6\nMATCH_B\nline8\nline9\nline10\n"

func TestE2E_ContextBeforeAndAfter(t *testing.T) {
	r := run(t, ctxFixture, "-C", "2", "MATCH")
	if r.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", r.exitCode, r.stderr)
	}
	want := "line1\nline2\nMATCH_A\nline4\nline5\nline6\nMATCH_B\nline8\nline9\n"
	if r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestE2E_ContextSeparatorBetweenNonAdjacentMatches(t *testing.T) {
	r := run(t, ctxFixture, "-A", "1", "MATCH")
	if r.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", r.exitCode, r.stderr)
	}
	want := "MATCH_A\nline4\n--\nMATCH_B\nline8\n"
	if r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

// TestE2E_ContextExplicitZeroStillSeparates is the real-process proof of
// the most surprising piece of this feature: -A0 is not the same as
// omitting context entirely — it still separates non-adjacent matches
// with "--", verified directly against real GNU grep.
func TestE2E_ContextExplicitZeroStillSeparates(t *testing.T) {
	r := run(t, ctxFixture, "-A0", "MATCH")
	if r.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", r.exitCode, r.stderr)
	}
	want := "MATCH_A\n--\nMATCH_B\n"
	if r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

// TestE2E_ContextBeforeWinsOverContextRegardlessOfFlagOrder is the
// real-process version of the -A/-B-over--C precedence rule internal/cli
// implements — verified directly against real GNU grep, which resolves
// "-C 3 -A 1" and "-A 1 -C 3" identically (after=1, before=3) rather
// than letting whichever flag comes last on the command line win.
func TestE2E_ContextBeforeWinsOverContextRegardlessOfFlagOrder(t *testing.T) {
	want := "line1\nline2\nMATCH_A\nline4\n"

	r1 := run(t, ctxFixture, "-A", "1", "-C", "3", "MATCH_A")
	if r1.stdout != want {
		t.Errorf("(-A 1 -C 3) stdout = %q, want %q", r1.stdout, want)
	}

	r2 := run(t, ctxFixture, "-C", "3", "-A", "1", "MATCH_A")
	if r2.stdout != want {
		t.Errorf("(-C 3 -A 1) stdout = %q, want %q", r2.stdout, want)
	}
}

func TestE2E_ContextNegativeLengthExitsTwo(t *testing.T) {
	r := run(t, ctxFixture, "-A", "-1", "MATCH")
	if r.exitCode != 2 {
		t.Errorf("exit code = %d, want 2", r.exitCode)
	}
}

// --- multi-pattern (-e/-f) ----------------------------------------------
//
// Expected output verified directly against real GNU grep (Git for
// Windows' grep.exe) against identical input.

func TestE2E_RepeatedERegexpOrsPatternsTogether(t *testing.T) {
	r := run(t, "apple\nbanana\ncherry\n", "-e", "apple", "-e", "cherry")
	if r.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", r.exitCode, r.stderr)
	}
	want := "apple\ncherry\n"
	if r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestE2E_PatternsFromFile(t *testing.T) {
	dir := t.TempDir()
	patFile := filepath.Join(dir, "patterns.txt")
	if err := os.WriteFile(patFile, []byte("banana\ndate\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	r := run(t, "apple\nbanana\ncherry\ndate\n", "-f", patFile)
	if r.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", r.exitCode, r.stderr)
	}
	want := "banana\ndate\n"
	if r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

// TestE2E_ERegexpMeansPositionalArgIsAFileNotAPattern is the real-
// process proof of the precedence rule: once -e is given, a bare
// positional argument that "looks like" a pattern is instead treated as
// a FILE and, since it doesn't exist, reported as a missing-file error
// — the pattern list is NOT silently extended with it.
func TestE2E_ERegexpMeansPositionalArgIsAFileNotAPattern(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(target, []byte("apple\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	r := run(t, "", "-e", "apple", "not-a-real-file", target)
	if r.exitCode != 2 {
		t.Errorf("exit code = %d, want 2 (the positional arg should be treated as a missing FILE)", r.exitCode)
	}
	if !strings.Contains(r.stderr, "not-a-real-file") {
		t.Errorf("stderr = %q, want it to mention the missing file", r.stderr)
	}
	if !strings.Contains(r.stdout, "apple") {
		t.Errorf("stdout = %q, want it to still contain the match from the real file", r.stdout)
	}
}

// TestE2E_MissingPatternFileExitsTwoWithDiagnostic covers -f's own
// error path at the real process boundary.
func TestE2E_MissingPatternFileExitsTwoWithDiagnostic(t *testing.T) {
	r := run(t, "", "-f", "does-not-exist-patterns.txt", "somefile.txt")
	if r.exitCode != 2 {
		t.Errorf("exit code = %d, want 2", r.exitCode)
	}
	if !strings.Contains(r.stderr, "No such file or directory") {
		t.Errorf("stderr = %q, want it to mention the missing pattern file", r.stderr)
	}
}

// --- binary file handling (-a/--text, -I) --------------------------------
//
// Expected output verified directly against real GNU grep (Git for
// Windows' grep.exe) against identical input.

func mustWriteBinaryFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func TestE2E_DefaultBinaryPrintsMatchMessage(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteBinaryFile(t, dir, "data.bin", []byte("prefix\x00needle\x00suffix\n"))

	r := run(t, "", "needle", path)
	if r.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", r.exitCode, r.stderr)
	}
	want := "Binary file " + path + " matches\n"
	if r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestE2E_TextFlagPrintsRawBinaryContent(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteBinaryFile(t, dir, "data.bin", []byte("prefix\x00needle\x00suffix\n"))

	r := run(t, "", "-a", "needle", path)
	if r.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", r.exitCode, r.stderr)
	}
	want := "prefix\x00needle\x00suffix\n"
	if r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestE2E_SkipBinarySkipsFileEntirely(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteBinaryFile(t, dir, "data.bin", []byte("prefix\x00needle\x00suffix\n"))

	r := run(t, "", "-I", "needle", path)
	if r.exitCode != 1 {
		t.Errorf("exit code = %d, want 1 (skipped file counts as no match)", r.exitCode)
	}
	if r.stdout != "" {
		t.Errorf("stdout = %q, want empty", r.stdout)
	}
}

// TestE2E_SkipBinaryWithFilesWithoutMatchListsButExitsNoMatch is the
// real-process proof of this feature's own -L exit-code fix: -I -L
// prints the skipped binary file's name, but exits 1 (no match), not 0
// — verified directly against real GNU grep.
func TestE2E_SkipBinaryWithFilesWithoutMatchListsButExitsNoMatch(t *testing.T) {
	dir := t.TempDir()
	path := mustWriteBinaryFile(t, dir, "data.bin", []byte("prefix\x00needle\x00suffix\n"))

	r := run(t, "", "-I", "-L", "needle", path)
	if r.exitCode != 1 {
		t.Errorf("exit code = %d, want 1", r.exitCode)
	}
	if r.stdout != path+"\n" {
		t.Errorf("stdout = %q, want %q", r.stdout, path+"\n")
	}
}

// --- -L exit-code fix (independent of -I) --------------------------------
//
// Real-process regression coverage for the -L exit-code bug found and
// fixed alongside -a/-I — see internal/grep/run_test.go's
// TestRun_FilesWithoutMatch_ExitCodeReflectsRawMatching for the full
// oracle-verified matrix this is drawn from.

func TestE2E_FilesWithoutMatch_SingleNonMatchingFileExitsNoMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("nope\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	r := run(t, "", "-L", "match", path)
	if r.exitCode != 1 {
		t.Errorf("exit code = %d, want 1 (printing a filename under -L does not imply a match)", r.exitCode)
	}
	if r.stdout != path+"\n" {
		t.Errorf("stdout = %q, want %q", r.stdout, path+"\n")
	}
}

func TestE2E_FilesWithoutMatch_EveryFileMatchingExitsMatchWithNoOutput(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("match\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	r := run(t, "", "-L", "match", a, b)
	if r.exitCode != 0 {
		t.Errorf("exit code = %d, want 0 (something matched, even though -L prints nothing)", r.exitCode)
	}
	if r.stdout != "" {
		t.Errorf("stdout = %q, want empty", r.stdout)
	}
}
