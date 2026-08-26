package grep

import (
	"context"
	"errors"
	"testing"
)

func TestRun_CancelledContextReturnsErrorImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	code, err := Run(ctx, Options{Pattern: "x"})

	if code != ExitError {
		t.Errorf("code = %d, want ExitError", code)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestRun_ValidContextReachesStub(t *testing.T) {
	// With a live (non-cancelled) context, Run should get past the
	// context check and hit the not-yet-implemented stub — pinning down
	// that the ctx.Err() check doesn't accidentally short-circuit every
	// call, only genuinely cancelled ones.
	code, err := Run(context.Background(), Options{Pattern: "x"})

	if code != ExitError {
		t.Errorf("code = %d, want ExitError", code)
	}
	if err == nil {
		t.Error("err = nil, want the not-yet-implemented stub error")
	}
	if errors.Is(err, context.Canceled) {
		t.Error("err should not be context.Canceled for a live context")
	}
}
