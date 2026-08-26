package walker

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestWalk_SingleFileRootAlwaysVisitedIgnoringFilters(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "target.txt")
	mustWriteFile(t, file, "x")

	got := collectVisited(t, file, Options{Exclude: []string{"*.txt"}})
	assertPaths(t, got, []string{file})
}

func TestWalk_NonRecursiveDirectoryReportsErrIsDir(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.txt"), "x")

	var gotErr error
	err := Walk(dir, Options{Recursive: false}, func(path string, err error) error {
		if path != dir {
			t.Errorf("visit called with path %q, want root %q", path, dir)
		}
		gotErr = err
		return err
	})
	if !errors.Is(err, ErrIsDir) {
		t.Errorf("Walk() error = %v, want ErrIsDir", err)
	}
	if !errors.Is(gotErr, ErrIsDir) {
		t.Errorf("visit received err = %v, want ErrIsDir", gotErr)
	}
}

func TestWalk_MissingRootReportsError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	var gotErr error
	_ = Walk(missing, Options{}, func(path string, err error) error {
		gotErr = err
		return nil
	})
	if gotErr == nil {
		t.Fatal("expected an error for a missing root, got nil")
	}
}

func TestWalk_RecursiveVisitsAllFilesInLexicalOrder(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "b.txt"), "")
	mustWriteFile(t, filepath.Join(dir, "a.txt"), "")
	mustMkdirAll(t, filepath.Join(dir, "sub"))
	mustWriteFile(t, filepath.Join(dir, "sub", "c.txt"), "")

	got := collectVisited(t, dir, Options{Recursive: true})
	want := []string{
		filepath.Join(dir, "a.txt"),
		filepath.Join(dir, "b.txt"),
		filepath.Join(dir, "sub", "c.txt"),
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("visited paths not in lexical order: %v", got)
	}
	assertPaths(t, got, want)
}

func TestWalk_ExcludeDirPrunesSubtree(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "vendor"))
	mustWriteFile(t, filepath.Join(dir, "vendor", "dep.go"), "")
	mustMkdirAll(t, filepath.Join(dir, "src"))
	mustWriteFile(t, filepath.Join(dir, "src", "main.go"), "")

	got := collectVisited(t, dir, Options{Recursive: true, ExcludeDir: []string{"vendor"}})
	assertPaths(t, got, []string{filepath.Join(dir, "src", "main.go")})
}

func TestWalk_IncludeFilter(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "main.go"), "")
	mustWriteFile(t, filepath.Join(dir, "readme.md"), "")

	got := collectVisited(t, dir, Options{Recursive: true, Include: []string{"*.go"}})
	assertPaths(t, got, []string{filepath.Join(dir, "main.go")})
}

func TestWalk_ExcludeFilter(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "main.go"), "")
	mustWriteFile(t, filepath.Join(dir, "main_test.go"), "")

	got := collectVisited(t, dir, Options{Recursive: true, Exclude: []string{"*_test.go"}})
	assertPaths(t, got, []string{filepath.Join(dir, "main.go")})
}

func TestWalk_PathsUseNativeJoinOfRootAndEntry(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "sub"))
	mustWriteFile(t, filepath.Join(dir, "sub", "file.txt"), "")

	got := collectVisited(t, dir, Options{Recursive: true})
	want := filepath.Join(dir, "sub", "file.txt") // uses filepath.Separator
	assertPaths(t, got, []string{want})
	if !strings.Contains(got[0], string(filepath.Separator)) {
		t.Errorf("path %q does not contain the OS separator %q", got[0], string(filepath.Separator))
	}
}

func TestWalk_BadIncludePatternFailsBeforeAnyVisit(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.txt"), "")

	visited := 0
	err := Walk(dir, Options{Recursive: true, Include: []string{"[unclosed"}}, func(path string, err error) error {
		visited++
		return nil
	})
	if err == nil {
		t.Fatal("expected an error for a malformed Include pattern, got nil")
	}
	if visited != 0 {
		t.Errorf("visit was called %d times, want 0 (should fail before walking)", visited)
	}
}

func TestWalk_VisitErrorAbortsWalk(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.txt"), "")
	mustWriteFile(t, filepath.Join(dir, "b.txt"), "")

	sentinel := errors.New("stop here")
	var visited []string
	err := Walk(dir, Options{Recursive: true}, func(path string, err error) error {
		visited = append(visited, path)
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Walk() error = %v, want sentinel", err)
	}
	if len(visited) != 1 {
		t.Errorf("visit was called %d times, want exactly 1 (walk should abort immediately)", len(visited))
	}
}

func TestWalk_SymlinkDefaultPolicyIgnoresSymlinks(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "real"))
	mustWriteFile(t, filepath.Join(dir, "real", "inside.txt"), "")
	mustWriteFile(t, filepath.Join(dir, "target.txt"), "")

	requireSymlinkSupport(t)
	mustSymlink(t, filepath.Join(dir, "real"), filepath.Join(dir, "link-to-dir"))
	mustSymlink(t, filepath.Join(dir, "target.txt"), filepath.Join(dir, "link-to-file.txt"))

	got := collectVisited(t, dir, Options{Recursive: true, FollowSymlinks: false})
	assertPaths(t, got, []string{
		filepath.Join(dir, "real", "inside.txt"),
		filepath.Join(dir, "target.txt"),
	})
}

func TestWalk_FollowSymlinksDescendsAndUsesSymlinkRelativePath(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "real"))
	mustWriteFile(t, filepath.Join(dir, "real", "inside.txt"), "")

	requireSymlinkSupport(t)
	mustSymlink(t, filepath.Join(dir, "real"), filepath.Join(dir, "link-to-dir"))

	got := collectVisited(t, dir, Options{Recursive: true, FollowSymlinks: true})
	want := []string{
		filepath.Join(dir, "link-to-dir", "inside.txt"), // via the symlink, not "real"
		filepath.Join(dir, "real", "inside.txt"),
	}
	assertPaths(t, got, want)
}

func TestWalk_FollowSymlinksCycleDoesNotHang(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "a"))
	requireSymlinkSupport(t)
	// a/loop -> dir (an ancestor), creating a cycle when followed.
	mustSymlink(t, dir, filepath.Join(dir, "a", "loop"))

	done := make(chan error, 1)
	go func() {
		done <- Walk(dir, Options{Recursive: true, FollowSymlinks: true}, func(string, error) error {
			return nil
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Walk() error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Walk did not return within 5s — likely stuck in a symlink cycle")
	}
}

// TestWalk_LongPath is the regression test PROCESS.md §3.4 calls for: a
// path well past the classic 260-character Windows limit must still be
// walkable without opting into any special long-path handling, verifying
// this toolchain's runtime already deals with it transparently.
func TestWalk_LongPath(t *testing.T) {
	dir := t.TempDir()
	path := dir
	segment := strings.Repeat("a", 50)
	depth := 0
	for len(path) < 300 {
		path = filepath.Join(path, segment)
		depth++
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll long path (len=%d, depth=%d): %v", len(path), depth, err)
	}
	target := filepath.Join(path, "deep.txt")
	mustWriteFile(t, target, "x")

	got := collectVisited(t, dir, Options{Recursive: true})
	assertPaths(t, got, []string{target})
}

// --- helpers -------------------------------------------------------

func collectVisited(t *testing.T, root string, opts Options) []string {
	t.Helper()
	var got []string
	if err := Walk(root, opts, func(path string, err error) error {
		if err != nil {
			t.Fatalf("unexpected visit error for %s: %v", path, err)
		}
		got = append(got, path)
		return nil
	}); err != nil {
		t.Fatalf("Walk() error: %v", err)
	}
	return got
}

func assertPaths(t *testing.T, got, want []string) {
	t.Helper()
	gotSorted := append([]string(nil), got...)
	wantSorted := append([]string(nil), want...)
	sort.Strings(gotSorted)
	sort.Strings(wantSorted)
	if len(gotSorted) != len(wantSorted) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range gotSorted {
		if gotSorted[i] != wantSorted[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", path, err)
	}
}

// requireSymlinkSupport skips the test if the current process can't
// create symlinks — on Windows this needs Developer Mode or admin
// privileges (SeCreateSymbolicLinkPrivilege), which isn't guaranteed in
// every CI/dev environment.
func requireSymlinkSupport(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	target := filepath.Join(dir, "probe-target")
	mustWriteFile(t, target, "")
	link := filepath.Join(dir, "probe-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation not permitted in this environment: %v", err)
	}
}

func mustSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Fatalf("Symlink(%s, %s): %v", oldname, newname, err)
	}
}
