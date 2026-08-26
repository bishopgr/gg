package cli

import (
	"context"
	"errors"
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

	if gotOpts.Pattern != "mypattern" {
		t.Errorf("Pattern = %q, want %q", gotOpts.Pattern, "mypattern")
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
