//go:build windows

package termenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnableANSI_RegularFileReturnsFalse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-console.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("os.Create: %v", err)
	}
	defer f.Close()

	if EnableANSI(f) {
		t.Error("EnableANSI(regular file) = true, want false (no console to enable VT on)")
	}
}

func TestEnableANSI_PipeReturnsFalse(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	if EnableANSI(w) {
		t.Error("EnableANSI(pipe) = true, want false (no console to enable VT on)")
	}
}

// TestEnableANSI_IdempotentOnNonConsole guards the documented contract
// that calling EnableANSI repeatedly is safe (e.g. gg might probe stdout
// once at startup, but should never crash or misbehave if it were ever
// called more than once).
func TestEnableANSI_IdempotentOnNonConsole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-console.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("os.Create: %v", err)
	}
	defer f.Close()

	first := EnableANSI(f)
	second := EnableANSI(f)
	if first != second {
		t.Errorf("EnableANSI not idempotent: first=%v second=%v", first, second)
	}
}
