package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gg/internal/grep"
)

// withFakeRunGrep swaps runGrep for fn for the duration of the calling
// test, restoring the original afterward. This is the seam that lets
// PROCESS.md §3.5's exit-code contract ("must be 0/1/2 regardless of
// OS; Cobra must not swallow the no-matches-vs-real-error distinction")
// be verified for every code without a real search engine wired up yet.
func withFakeRunGrep(t *testing.T, fn func(context.Context, grep.Options) (int, error)) {
	t.Helper()
	orig := runGrep
	runGrep = fn
	t.Cleanup(func() { runGrep = orig })
}

func TestExecute_VersionExitsZero(t *testing.T) {
	if got := Execute(context.Background(), []string{"--version"}); got != 0 {
		t.Errorf("Execute(--version) = %d, want 0", got)
	}
}

func TestExecute_HelpExitsZero(t *testing.T) {
	if got := Execute(context.Background(), []string{"--help"}); got != 0 {
		t.Errorf("Execute(--help) = %d, want 0", got)
	}
}

func TestExecute_NoArgsExitsTwo(t *testing.T) {
	if got := Execute(context.Background(), []string{}); got != 2 {
		t.Errorf("Execute(no args) = %d, want 2", got)
	}
}

func TestExecute_PropagatesExitCodeFromRunGrep(t *testing.T) {
	tests := []struct {
		name string
		code int
		err  error
	}{
		{"match found", grep.ExitMatch, nil},
		{"no match", grep.ExitNoMatch, nil},
		{"real error", grep.ExitError, errors.New("boom")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
				return tt.code, tt.err
			})

			got := Execute(context.Background(), []string{"pattern", "file.txt"})
			if got != tt.code {
				t.Errorf("Execute() = %d, want %d", got, tt.code)
			}
		})
	}
}

func TestExecute_PassesArgsThroughAsOptions(t *testing.T) {
	var gotOpts grep.Options
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		gotOpts = opts
		return grep.ExitMatch, nil
	})

	Execute(context.Background(), []string{"mypattern", "a.txt", "b.txt"})

	if len(gotOpts.Patterns) != 1 || gotOpts.Patterns[0] != "mypattern" {
		t.Errorf("Patterns = %v, want [mypattern]", gotOpts.Patterns)
	}
	if len(gotOpts.Files) != 2 || gotOpts.Files[0] != "a.txt" || gotOpts.Files[1] != "b.txt" {
		t.Errorf("Files = %v, want [a.txt b.txt]", gotOpts.Files)
	}
}

// TestExecute_ThreadsContextToRunGrep guards PROCESS.md §3.5's Ctrl+C
// wiring at the plumbing level: whatever context.Context Execute is
// given must be the one that reaches runGrep, not context.Background()
// or some other substitute — otherwise a future cancellation-aware
// engine would never actually see a Ctrl+C-triggered cancellation.
func TestExecute_ThreadsContextToRunGrep(t *testing.T) {
	type ctxKey struct{}
	want := "sentinel-value"
	ctx := context.WithValue(context.Background(), ctxKey{}, want)

	var gotValue any
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		gotValue = ctx.Value(ctxKey{})
		return grep.ExitMatch, nil
	})

	Execute(ctx, []string{"pattern"})

	if gotValue != want {
		t.Errorf("context value seen by runGrep = %v, want %v (context not threaded through)", gotValue, want)
	}
}

// TestExecute_ShortHMeansNoFilenameNotHelp is the concrete proof of the
// registerFlags workaround documented in root.go: -h must reach RunE as
// --no-filename, not trigger Cobra's help output.
func TestExecute_ShortHMeansNoFilenameNotHelp(t *testing.T) {
	var gotOpts grep.Options
	ran := false
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		ran = true
		gotOpts = opts
		return grep.ExitMatch, nil
	})

	Execute(context.Background(), []string{"-h", "pattern", "file.txt"})

	if !ran {
		t.Fatal("runGrep was never called — -h triggered help instead of being parsed as --no-filename")
	}
	if !gotOpts.NoFilename {
		t.Error("Options.NoFilename = false, want true from -h")
	}
}

// TestExecute_ShortVMeansInvertMatchNotVersion is the equivalent proof
// for -v: it must reach RunE as --invert-match, not print the version.
func TestExecute_ShortVMeansInvertMatchNotVersion(t *testing.T) {
	var gotOpts grep.Options
	ran := false
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		ran = true
		gotOpts = opts
		return grep.ExitMatch, nil
	})

	Execute(context.Background(), []string{"-v", "pattern", "file.txt"})

	if !ran {
		t.Fatal("runGrep was never called — -v triggered version instead of being parsed as --invert-match")
	}
	if !gotOpts.InvertMatch {
		t.Error("Options.InvertMatch = false, want true from -v")
	}
}

func TestExecute_ColorFlagParsing(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want grep.ColorMode
	}{
		{"default (flag absent)", []string{"pattern", "f"}, grep.ColorAuto},
		{"explicit auto", []string{"--color=auto", "pattern", "f"}, grep.ColorAuto},
		{"bare --color means always", []string{"--color", "pattern", "f"}, grep.ColorAlways},
		{"explicit always", []string{"--color=always", "pattern", "f"}, grep.ColorAlways},
		{"explicit never", []string{"--color=never", "pattern", "f"}, grep.ColorNever},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotOpts grep.Options
			withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
				gotOpts = opts
				return grep.ExitMatch, nil
			})

			code := Execute(context.Background(), tt.args)
			if code != grep.ExitMatch {
				t.Fatalf("Execute(%v) = %d, want ExitMatch (args should have parsed cleanly)", tt.args, code)
			}
			if gotOpts.Color != tt.want {
				t.Errorf("Options.Color = %v, want %v", gotOpts.Color, tt.want)
			}
		})
	}
}

func TestExecute_FilesWithMatchesAndWithoutMatchAreMutuallyExclusive(t *testing.T) {
	code := Execute(context.Background(), []string{"-l", "-L", "pattern", "file.txt"})
	if code != grep.ExitError {
		t.Errorf("Execute(-l -L) = %d, want ExitError (mutually exclusive flags)", code)
	}
}

func TestExecute_NoFilenameAndWithFilenameAreMutuallyExclusive(t *testing.T) {
	code := Execute(context.Background(), []string{"-h", "-H", "pattern", "file.txt"})
	if code != grep.ExitError {
		t.Errorf("Execute(-h -H) = %d, want ExitError (mutually exclusive flags)", code)
	}
}

// TestExecute_FlagsReachOptions is a broader sanity check that the bulk
// boolean/slice flags all actually reach grep.Options — narrower than
// the dedicated tests above, but cheap insurance against a copy-paste
// mistake in the flags->Options translation for the flags that don't
// have their own dedicated test.
func TestExecute_FlagsReachOptions(t *testing.T) {
	var gotOpts grep.Options
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		gotOpts = opts
		return grep.ExitMatch, nil
	})

	Execute(context.Background(), []string{
		"-i", "-F", "-w", "-x", "-n", "-o", "-c",
		"-r", "--include=*.go", "--exclude=*_test.go", "--exclude-dir=vendor",
		"pattern", "file.txt",
	})

	switch {
	case !gotOpts.IgnoreCase:
		t.Error("IgnoreCase not set")
	case !gotOpts.Fixed:
		t.Error("Fixed not set")
	case !gotOpts.WordRegexp:
		t.Error("WordRegexp not set")
	case !gotOpts.LineRegexp:
		t.Error("LineRegexp not set")
	case !gotOpts.LineNumber:
		t.Error("LineNumber not set")
	case !gotOpts.OnlyMatching:
		t.Error("OnlyMatching not set")
	case !gotOpts.Count:
		t.Error("Count not set")
	case !gotOpts.Recursive:
		t.Error("Recursive not set")
	case len(gotOpts.Include) != 1 || gotOpts.Include[0] != "*.go":
		t.Errorf("Include = %v, want [*.go]", gotOpts.Include)
	case len(gotOpts.Exclude) != 1 || gotOpts.Exclude[0] != "*_test.go":
		t.Errorf("Exclude = %v, want [*_test.go]", gotOpts.Exclude)
	case len(gotOpts.ExcludeDir) != 1 || gotOpts.ExcludeDir[0] != "vendor":
		t.Errorf("ExcludeDir = %v, want [vendor]", gotOpts.ExcludeDir)
	}
}

func TestExecute_DereferenceRecursiveImpliesRecursiveAndFollowSymlinks(t *testing.T) {
	var gotOpts grep.Options
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		gotOpts = opts
		return grep.ExitMatch, nil
	})

	Execute(context.Background(), []string{"-R", "pattern", "dir"})

	if !gotOpts.Recursive {
		t.Error("Recursive = false, want true from -R")
	}
	if !gotOpts.FollowSymlinks {
		t.Error("FollowSymlinks = false, want true from -R")
	}
}

// --- context lines (-A/-B/-C) ----------------------------------------------

func TestExecute_ContextFlagsReachOptions(t *testing.T) {
	var gotOpts grep.Options
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		gotOpts = opts
		return grep.ExitMatch, nil
	})

	Execute(context.Background(), []string{"-A", "2", "-B", "3", "pattern", "file.txt"})

	if !gotOpts.ContextEnabled {
		t.Error("ContextEnabled = false, want true")
	}
	if gotOpts.ContextAfter != 2 {
		t.Errorf("ContextAfter = %d, want 2", gotOpts.ContextAfter)
	}
	if gotOpts.ContextBefore != 3 {
		t.Errorf("ContextBefore = %d, want 3", gotOpts.ContextBefore)
	}
}

func TestExecute_ContextNotGivenLeavesItDisabled(t *testing.T) {
	var gotOpts grep.Options
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		gotOpts = opts
		return grep.ExitMatch, nil
	})

	Execute(context.Background(), []string{"pattern", "file.txt"})

	if gotOpts.ContextEnabled {
		t.Error("ContextEnabled = true, want false when no -A/-B/-C was given")
	}
}

// TestExecute_ContextOnlyGivesBothBeforeAndAfter mirrors GNU grep: -C
// alone is shorthand for -A NUM -B NUM.
func TestExecute_ContextOnlyGivesBothBeforeAndAfter(t *testing.T) {
	var gotOpts grep.Options
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		gotOpts = opts
		return grep.ExitMatch, nil
	})

	Execute(context.Background(), []string{"-C", "3", "pattern", "file.txt"})

	if !gotOpts.ContextEnabled {
		t.Error("ContextEnabled = false, want true")
	}
	if gotOpts.ContextBefore != 3 || gotOpts.ContextAfter != 3 {
		t.Errorf("ContextBefore/After = %d/%d, want 3/3", gotOpts.ContextBefore, gotOpts.ContextAfter)
	}
}

// TestExecute_ExplicitAfterOrBeforeWinsOverContext is the precedence
// regression test verified directly against real GNU grep: an
// explicitly-given -A or -B always wins for its own direction over -C,
// regardless of the order the flags appear on the command line.
func TestExecute_ExplicitAfterOrBeforeWinsOverContext(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantBefore int
		wantAfter  int
	}{
		{"A after C", []string{"-C", "3", "-A", "1"}, 3, 1},
		{"A before C", []string{"-A", "1", "-C", "3"}, 3, 1},
		{"B after C", []string{"-C", "3", "-B", "1"}, 1, 3},
		{"B before C", []string{"-B", "1", "-C", "3"}, 1, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotOpts grep.Options
			withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
				gotOpts = opts
				return grep.ExitMatch, nil
			})

			args := append(append([]string{}, tt.args...), "pattern", "file.txt")
			code := Execute(context.Background(), args)
			if code != grep.ExitMatch {
				t.Fatalf("Execute(%v) = %d, want ExitMatch", args, code)
			}
			if gotOpts.ContextBefore != tt.wantBefore || gotOpts.ContextAfter != tt.wantAfter {
				t.Errorf("ContextBefore/After = %d/%d, want %d/%d",
					gotOpts.ContextBefore, gotOpts.ContextAfter, tt.wantBefore, tt.wantAfter)
			}
		})
	}
}

// TestExecute_ExplicitZeroContextStillEnablesIt covers
// Options.ContextEnabled's whole reason for existing: -A0 must reach
// grep.Options as ContextEnabled=true (only the resolved value is 0),
// not indistinguishable from never passing -A/-B/-C at all — verified
// this is genuinely different GNU grep behavior in internal/grep's own
// tests.
func TestExecute_ExplicitZeroContextStillEnablesIt(t *testing.T) {
	var gotOpts grep.Options
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		gotOpts = opts
		return grep.ExitMatch, nil
	})

	Execute(context.Background(), []string{"-A0", "pattern", "file.txt"})

	if !gotOpts.ContextEnabled {
		t.Error("ContextEnabled = false, want true even for an explicit -A0")
	}
	if gotOpts.ContextAfter != 0 {
		t.Errorf("ContextAfter = %d, want 0", gotOpts.ContextAfter)
	}
}

func TestExecute_NegativeContextLengthIsAnError(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"-A", []string{"-A", "-1", "pattern", "file.txt"}},
		{"-B", []string{"-B", "-1", "pattern", "file.txt"}},
		{"-C", []string{"-C", "-1", "pattern", "file.txt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := Execute(context.Background(), tt.args)
			if code != grep.ExitError {
				t.Errorf("Execute(%v) = %d, want ExitError", tt.args, code)
			}
		})
	}
}

// --- multi-pattern (-e/-f) ---------------------------------------------

func TestExecute_SingleERegexpReachesPatterns(t *testing.T) {
	var gotOpts grep.Options
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		gotOpts = opts
		return grep.ExitMatch, nil
	})

	Execute(context.Background(), []string{"-e", "foo", "file.txt"})

	if len(gotOpts.Patterns) != 1 || gotOpts.Patterns[0] != "foo" {
		t.Errorf("Patterns = %v, want [foo]", gotOpts.Patterns)
	}
	if len(gotOpts.Files) != 1 || gotOpts.Files[0] != "file.txt" {
		t.Errorf("Files = %v, want [file.txt]", gotOpts.Files)
	}
}

func TestExecute_RepeatedERegexpAccumulates(t *testing.T) {
	var gotOpts grep.Options
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		gotOpts = opts
		return grep.ExitMatch, nil
	})

	Execute(context.Background(), []string{"-e", "foo", "-e", "bar", "file.txt"})

	want := []string{"foo", "bar"}
	if len(gotOpts.Patterns) != len(want) || gotOpts.Patterns[0] != want[0] || gotOpts.Patterns[1] != want[1] {
		t.Errorf("Patterns = %v, want %v", gotOpts.Patterns, want)
	}
}

func TestExecute_FileFlagReachesPatternFiles(t *testing.T) {
	var gotOpts grep.Options
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		gotOpts = opts
		return grep.ExitMatch, nil
	})

	Execute(context.Background(), []string{"-f", "patterns.txt", "file.txt"})

	if len(gotOpts.PatternFiles) != 1 || gotOpts.PatternFiles[0] != "patterns.txt" {
		t.Errorf("PatternFiles = %v, want [patterns.txt]", gotOpts.PatternFiles)
	}
	if len(gotOpts.Patterns) != 0 {
		t.Errorf("Patterns = %v, want empty (no -e given)", gotOpts.Patterns)
	}
}

// TestExecute_ERegexpMeansEveryPositionalArgIsAFile is the core
// precedence rule verified directly against real GNU grep: once -e (or
// -f) is given, the positional argument that would normally be PATTERN
// is treated as a FILE instead, not silently dropped and not still
// consumed as an extra pattern.
func TestExecute_ERegexpMeansEveryPositionalArgIsAFile(t *testing.T) {
	var gotOpts grep.Options
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		gotOpts = opts
		return grep.ExitMatch, nil
	})

	Execute(context.Background(), []string{"-e", "foo", "looks-like-a-pattern", "file.txt"})

	want := []string{"looks-like-a-pattern", "file.txt"}
	if len(gotOpts.Files) != len(want) || gotOpts.Files[0] != want[0] || gotOpts.Files[1] != want[1] {
		t.Errorf("Files = %v, want %v", gotOpts.Files, want)
	}
	if len(gotOpts.Patterns) != 1 || gotOpts.Patterns[0] != "foo" {
		t.Errorf("Patterns = %v, want [foo] (the positional arg must not also become a pattern)", gotOpts.Patterns)
	}
}

// TestExecute_ERegexpAllowsZeroPositionalArgs covers the other half of
// the same precedence rule: with -e/-f given, a bare PATTERN is no
// longer required at all — zero FILE arguments is valid too (search
// stdin), matching real GNU grep's `grep -e pattern` (no FILE) reading
// from stdin rather than erroring for a "missing PATTERN".
func TestExecute_ERegexpAllowsZeroPositionalArgs(t *testing.T) {
	var ran bool
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		ran = true
		return grep.ExitMatch, nil
	})

	code := Execute(context.Background(), []string{"-e", "foo"})
	if code != grep.ExitMatch {
		t.Errorf("Execute(-e foo) = %d, want ExitMatch", code)
	}
	if !ran {
		t.Fatal("runGrep was never called — zero positional args with -e should still be valid")
	}
}

// TestExecute_NoPatternAndNoERegexpStillRequiresOneArg guards the
// unchanged default behavior: without -e/-f, a bare PATTERN is still
// mandatory.
func TestExecute_NoPatternAndNoERegexpStillRequiresOneArg(t *testing.T) {
	code := Execute(context.Background(), []string{})
	if code != grep.ExitError {
		t.Errorf("Execute(no args) = %d, want ExitError", code)
	}
}

// --- binary file handling (-a/--text, -I) -----------------------------

func TestExecute_DefaultBinaryModeIsUnset(t *testing.T) {
	var gotOpts grep.Options
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		gotOpts = opts
		return grep.ExitMatch, nil
	})

	Execute(context.Background(), []string{"pattern", "file.txt"})

	if gotOpts.Binary != grep.BinaryDefault {
		t.Errorf("Binary = %v, want BinaryDefault", gotOpts.Binary)
	}
}

func TestExecute_TextFlagSetsBinaryText(t *testing.T) {
	tests := [][]string{
		{"-a", "pattern", "file.txt"},
		{"--text", "pattern", "file.txt"},
	}
	for _, args := range tests {
		t.Run(args[0], func(t *testing.T) {
			var gotOpts grep.Options
			withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
				gotOpts = opts
				return grep.ExitMatch, nil
			})

			Execute(context.Background(), args)

			if gotOpts.Binary != grep.BinaryText {
				t.Errorf("Binary = %v, want BinaryText", gotOpts.Binary)
			}
		})
	}
}

func TestExecute_ISetsBinarySkip(t *testing.T) {
	tests := [][]string{
		{"-I", "pattern", "file.txt"},
		{"--skip-binary", "pattern", "file.txt"},
	}
	for _, args := range tests {
		t.Run(args[0], func(t *testing.T) {
			var gotOpts grep.Options
			withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
				gotOpts = opts
				return grep.ExitMatch, nil
			})

			Execute(context.Background(), args)

			if gotOpts.Binary != grep.BinarySkip {
				t.Errorf("Binary = %v, want BinarySkip", gotOpts.Binary)
			}
		})
	}
}

// TestExecute_ITakesPrecedenceOverTextWhenBothGiven documents gg's own
// deliberate, simplified precedence rule for the rare case both flags
// are given together: -I always wins, regardless of which flag appears
// last on the command line. This is a documented divergence from real
// GNU grep, which tracks true command-line order here — reproducing
// that exactly would require inspecting raw argument order ourselves
// for a combination unlikely to ever be given deliberately.
func TestExecute_ITakesPrecedenceOverTextWhenBothGiven(t *testing.T) {
	tests := [][]string{
		{"-a", "-I", "pattern", "file.txt"},
		{"-I", "-a", "pattern", "file.txt"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args[:2], " "), func(t *testing.T) {
			var gotOpts grep.Options
			withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
				gotOpts = opts
				return grep.ExitMatch, nil
			})

			Execute(context.Background(), args)

			if gotOpts.Binary != grep.BinarySkip {
				t.Errorf("Binary = %v, want BinarySkip (-I must win)", gotOpts.Binary)
			}
		})
	}
}

func TestExecute_CancelledContextIsObservedByRunGrep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var sawCancelled bool
	withFakeRunGrep(t, func(ctx context.Context, opts grep.Options) (int, error) {
		sawCancelled = errors.Is(ctx.Err(), context.Canceled)
		return grep.ExitError, ctx.Err()
	})

	got := Execute(ctx, []string{"pattern"})

	if !sawCancelled {
		t.Error("runGrep did not observe the cancelled context")
	}
	if got != grep.ExitError {
		t.Errorf("Execute() = %d, want ExitError", got)
	}
}
