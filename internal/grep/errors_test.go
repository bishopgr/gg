package grep

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"gg/internal/walker"
)

func TestDescribeError_IsADirectory(t *testing.T) {
	if got := describeError(walker.ErrIsDir); got != "Is a directory" {
		t.Errorf("describeError(ErrIsDir) = %q, want %q", got, "Is a directory")
	}
}

func TestDescribeError_NotExist(t *testing.T) {
	_, err := os.Open(filepath.Join(t.TempDir(), "nope.txt"))
	if err == nil {
		t.Fatal("expected an error opening a missing file")
	}
	if got := describeError(err); got != "No such file or directory" {
		t.Errorf("describeError(not-exist) = %q, want %q", got, "No such file or directory")
	}
}

func TestDescribeError_FallsBackToErrorText(t *testing.T) {
	err := fmt.Errorf("something unusual happened")
	if got := describeError(err); got != "something unusual happened" {
		t.Errorf("describeError(unknown) = %q, want %q", got, "something unusual happened")
	}
}

func TestDescribeError_WrappedNotExistStillMatches(t *testing.T) {
	wrapped := fmt.Errorf("context: %w", fs.ErrNotExist)
	if got := describeError(wrapped); got != "No such file or directory" {
		t.Errorf("describeError(wrapped not-exist) = %q, want %q", got, "No such file or directory")
	}
}
