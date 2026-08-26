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
