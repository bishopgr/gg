package grep

import (
	"errors"
	"io/fs"

	"gg/internal/walker"
)

// describeError translates an error into the short, conventional wording
// GNU grep uses for its own per-file diagnostics, so gg's messages read
// the same on Windows as on Unix rather than exposing whatever raw OS
// error text happened to come back (e.g. Windows' own "The system
// cannot find the file specified." vs. Unix's "No such file or
// directory") — checked via the portable fs.ErrNotExist/fs.ErrPermission
// sentinels rather than matching message text, so this works
// identically on every OS.
func describeError(err error) string {
	switch {
	case errors.Is(err, walker.ErrIsDir):
		return "Is a directory"
	case errors.Is(err, fs.ErrNotExist):
		return "No such file or directory"
	case errors.Is(err, fs.ErrPermission):
		return "Permission denied"
	default:
		return err.Error()
	}
}
