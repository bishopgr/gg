//go:build windows

package termenv

import (
	"os"

	"golang.org/x/sys/windows"
)

// EnableANSI attempts to turn on ANSI/virtual-terminal escape sequence
// processing for f's underlying console, and reports whether ANSI codes
// can be expected to render correctly afterward.
//
// This targets Windows 10+ consoles (legacy conhost and Windows
// Terminal/ConPTY alike) via SetConsoleMode +
// ENABLE_VIRTUAL_TERMINAL_PROCESSING, per PROCESS.md §3.3 — a handful of
// lines against a stable, documented Win32 API, rather than pulling in
// an ANSI-to-Win32-API translator (e.g. go-colorable) whose main value
// is supporting pre-Windows-10 consoles that cannot interpret VT
// sequences at all and are not a target for this project.
//
// Safe and idempotent: calling it multiple times, or on a handle that
// isn't a console at all (redirected to a file/pipe), is not an error —
// it just reports false, since there is nothing to enable.
func EnableANSI(f *os.File) bool {
	handle := windows.Handle(f.Fd())

	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		// Not a console handle (redirected to a file/pipe): nothing to
		// enable, and this is not an error condition.
		return false
	}

	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true // already enabled, e.g. Windows Terminal sets this itself
	}

	if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		// A legacy console that rejects the flag (pre-Windows-10): fail
		// closed rather than emitting raw escape codes the console will
		// print literally instead of interpreting.
		return false
	}
	return true
}
