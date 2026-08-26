package source

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpen_RegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "content.txt")
	if err := os.WriteFile(path, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rc, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "hello\nworld\n" {
		t.Errorf("content = %q, want %q", got, "hello\nworld\n")
	}
}

func TestOpen_MissingFileReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.txt")
	if _, err := Open(path); err == nil {
		t.Fatal("expected an error for a missing file, got nil")
	}
}

func TestOpen_Dash_ReadsFromStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	rc, err := Open("-")
	if err != nil {
		t.Fatalf("Open(\"-\"): %v", err)
	}

	go func() {
		_, _ = w.WriteString("piped content")
		w.Close()
	}()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "piped content" {
		t.Errorf("content = %q, want %q", got, "piped content")
	}

	// Close must be a safe no-op: it must not close the process's real
	// stdin (which we've swapped out, but the contract holds either way).
	if err := rc.Close(); err != nil {
		t.Errorf("Close() on the stdin wrapper = %v, want nil", err)
	}
	r.Close()
}

// TestOpen_AllowsConcurrentWrite proves the baseline (unaffected) case:
// even plain os.Open already allows a concurrent writer on Windows —
// see open_windows.go's doc comment for the specifics this pins down.
func TestOpen_AllowsConcurrentWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(path, []byte("line1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rc, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	w, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("concurrent write-open failed while gg held the file open for reading: %v", err)
	}
	if _, err := w.WriteString("line2\n"); err != nil {
		t.Fatalf("concurrent write failed: %v", err)
	}
	w.Close()
}

// TestOpen_AllowsDeleteWhileOpen is the regression test for the actual
// fix: with plain os.Open on this Go version, this fails with "The
// process cannot access the file because it is being used by another
// process." Open must succeed here.
func TestOpen_AllowsDeleteWhileOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(path, []byte("line1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rc, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	if err := os.Remove(path); err != nil {
		t.Errorf("os.Remove while gg held the file open for reading: %v (FILE_SHARE_DELETE regression?)", err)
	}
}

// TestOpen_AllowsRenameWhileOpen mirrors the delete case for the other
// half of typical log-rotation (rename-then-recreate).
func TestOpen_AllowsRenameWhileOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(path, []byte("line1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rc, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	renamed := path + ".1"
	if err := os.Rename(path, renamed); err != nil {
		t.Errorf("os.Rename while gg held the file open for reading: %v (FILE_SHARE_DELETE regression?)", err)
	}
}

// TestOpen_StillReadableAfterConcurrentDelete proves the fix is useful,
// not just harmless: gg keeps reading the (now-unlinked-on-disk-name)
// file's original content via its still-open handle, exactly like POSIX
// unlink-while-open semantics.
func TestOpen_StillReadableAfterConcurrentDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	want := "line1\nline2\n"
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rc, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	if err := os.Remove(path); err != nil {
		t.Fatalf("os.Remove: %v", err)
	}

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll after concurrent delete: %v", err)
	}
	if string(got) != want {
		t.Errorf("content after concurrent delete = %q, want %q", got, want)
	}
}
