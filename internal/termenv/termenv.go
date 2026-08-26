// Package termenv provides the low-level, platform-specific mechanisms
// PROCESS.md §3.3 calls for: detecting whether a stream is an
// interactive terminal, and — on Windows — enabling ANSI/virtual-
// terminal escape sequence processing on the console.
//
// This package is detection/mechanism only, the same way
// internal/encoding only detects and internal/matcher only matches: it
// does not decide whether color *should* be used. Combining these facts
// with a --color=auto|always|never flag value is a policy decision that
// belongs to the future output layer (§4 step 7), not here.
package termenv

import (
	"os"

	"golang.org/x/term"
)

// IsTerminal reports whether f is connected to an interactive terminal,
// as opposed to a regular file, a pipe, or other redirected output.
//
// This delegates to golang.org/x/term rather than a hand-rolled isatty
// check specifically because Windows terminal detection is not a single
// ioctl the way it is on Unix — x/term handles the legacy-conhost vs.
// ConPTY vs. redirected-handle distinctions correctly (PROCESS.md §3.3)
// so this package doesn't have to re-implement that.
func IsTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
