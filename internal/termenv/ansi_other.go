//go:build !windows

package termenv

import "os"

// EnableANSI is a no-op on non-Windows platforms: terminals there
// already interpret ANSI escape sequences natively, and there is no
// console "mode" to toggle the way Windows has. It always reports true
// so callers have a single code path across platforms — whether output
// is actually going to a terminal worth coloring at all is IsTerminal's
// job, not this function's.
func EnableANSI(f *os.File) bool {
	_ = f
	return true
}
