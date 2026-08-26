package grep

import (
	"context"
	"errors"
	"testing"
)

func TestRun_CancelledContextReturnsErrorImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	code, err := Run(ctx, Options{Patterns: []string{"x"}})

	if code != ExitError {
		t.Errorf("code = %d, want ExitError", code)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestRun_ValidContextProceedsPastTheCheck(t *testing.T) {
	// With a live (non-cancelled) context, Run should get past the
	// context check and actually search — pinning down that ctx.Err()
	// doesn't accidentally short-circuit every call, only genuinely
	// cancelled ones.
	dir := t.TempDir()
	path := mustWriteTempFile(t, dir, "a.txt", "hello\n")

	_, _, _, err := runCapture(t, Options{Patterns: []string{"hello"}, Files: []string{path}})
	if errors.Is(err, context.Canceled) {
		t.Error("err should not be context.Canceled for a live context")
	}
}
