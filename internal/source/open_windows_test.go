//go:build windows

package source

import (
	"io"
	"testing"
)

// TestOpen_NULDevice pins down PROCESS.md §3.6's reserved-device-name
// question: no special-casing is needed. Verified empirically — see
// PROCESS.md §3.6 — os.Stat("NUL") succeeds (a non-directory character
// device, size 0) and reading it yields immediate EOF with no error,
// the same harmless outcome Unix grep gets from /dev/null.
func TestOpen_NULDevice(t *testing.T) {
	rc, err := Open("NUL")
	if err != nil {
		t.Fatalf("Open(\"NUL\"): %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll(NUL): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("read %d bytes from NUL, want 0", len(got))
	}
}
