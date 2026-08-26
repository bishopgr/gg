package termenv

import (
	"os"
	"path/filepath"
	"testing"
)

// A regular file and a pipe are the two cases we can reliably construct
// in an automated test without an attached interactive console — both
// must report false. Verifying the true/interactive case requires a
// real console or pseudo-console and is exercised manually (see
// README.md TESTING and PROCESS.md §3.3's note on conhost vs. Windows
// Terminal vs. ConPTY).

func TestIsTerminal_RegularFileIsNotATerminal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-terminal.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("os.Create: %v", err)
	}
	defer f.Close()

	if IsTerminal(f) {
		t.Error("IsTerminal(regular file) = true, want false")
	}
}

func TestIsTerminal_PipeIsNotATerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	if IsTerminal(r) {
		t.Error("IsTerminal(pipe read end) = true, want false")
	}
	if IsTerminal(w) {
		t.Error("IsTerminal(pipe write end) = true, want false")
	}
}
