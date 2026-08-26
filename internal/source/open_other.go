//go:build !windows

package source

import "os"

// openFile is a thin wrapper over os.Open: POSIX systems already allow
// unlinking or renaming a file while another process holds it open for
// reading, so no extra flags are needed here — see open_windows.go for
// why Windows is different.
func openFile(path string) (ReadCloser, error) {
	return os.Open(path)
}
