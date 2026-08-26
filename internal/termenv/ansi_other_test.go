//go:build !windows

package termenv

import (
	"os"
	"path/filepath"
	"testing"
)

// On non-Windows, EnableANSI is documented as an always-true no-op: it
// reports nothing about whether f is actually a terminal (that's
// IsTerminal's job), only that there is no Windows-style console mode
// to toggle.
func TestEnableANSI_AlwaysTrueOnNonWindows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "some-file.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("os.Create: %v", err)
	}
	defer f.Close()

	if !EnableANSI(f) {
		t.Error("EnableANSI(regular file) = false, want true on non-Windows")
	}
}
