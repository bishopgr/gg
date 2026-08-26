//go:build windows

package source

import (
	"os"

	"golang.org/x/sys/windows"
)

// openFile opens path for reading with FILE_SHARE_DELETE included
// alongside the usual FILE_SHARE_READ | FILE_SHARE_WRITE that plain
// os.Open uses.
//
// This is not a defensive guess: Go 1.26's own
// src/syscall/syscall_windows.go sets
// `sharemode := uint32(FILE_SHARE_READ | FILE_SHARE_WRITE)` for a
// regular os.Open — FILE_SHARE_DELETE is not included. Verified
// empirically too: with plain os.Open, a concurrent os.Remove or
// os.Rename of the same file fails with "The process cannot access the
// file because it is being used by another process," while a concurrent
// *write* to it succeeds. POSIX has no equivalent restriction — you can
// unlink or rename a file out from under a process that still has it
// open for reading.
//
// That gap matters specifically for a grep-like tool: log rotation
// (rename-then-recreate, or delete-then-recreate) is exactly the
// pattern of "another process modifies the file while gg is reading
// it," and a Windows-first tool searching live logs should not be the
// reason that rotation fails.
func openFile(path string) (ReadCloser, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}

	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}

	return os.NewFile(uintptr(handle), path), nil
}
