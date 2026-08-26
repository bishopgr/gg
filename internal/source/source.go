// Package source opens the byte stream gg is about to search: a named
// file, or standard input when given "-", matching the conventional
// meaning every Unix-heritage tool (grep, cat, sed, ...) already gives
// that argument.
//
// This addresses PROCESS.md §3.6 directly: opening a file gg is about to
// read through should not impose Windows' more restrictive default
// file-sharing semantics on other processes — notably, log rotation
// (rename or delete a file that's still open elsewhere) — the way plain
// os.Open does. See open_windows.go for the verified specifics.
package source

import "os"

// Open returns a ReadCloser for path, or standard input when path is
// "-". The returned Close is always safe to call and never closes the
// process's real os.Stdin, so mixing "-" with real file arguments in a
// future multi-file invocation can't accidentally sever stdin for
// everything else.
func Open(path string) (ReadCloser, error) {
	if path == "-" {
		return stdinReader{}, nil
	}
	return openFile(path)
}

// ReadCloser is the minimal interface Open returns — deliberately just
// io.Reader + io.Closer (an unnamed alias would do, but a named type
// documents intent and gives callers something to reference).
type ReadCloser interface {
	Read(p []byte) (n int, err error)
	Close() error
}

type stdinReader struct{}

func (stdinReader) Read(p []byte) (int, error) { return os.Stdin.Read(p) }
func (stdinReader) Close() error               { return nil }
