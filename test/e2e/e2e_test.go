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
