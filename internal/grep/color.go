package grep

import (
	"fmt"
	"io"
	"os"
	"strings"

	"gg/internal/termenv"
)

// ColorMode mirrors GNU grep's --color=[auto|always|never].
type ColorMode int

const (
	// ColorAuto enables color only when writing to a terminal (the
	// default when --color is not given at all).
	ColorAuto ColorMode = iota
	// ColorAlways forces color even when redirected to a file or pipe.
	// This is also what a bare `--color` (no `=value`) means in GNU
	// grep — deliberately not the same as the flag's default.
	ColorAlways
	// ColorNever disables color unconditionally.
	ColorNever
)

// ParseColorMode parses a --color flag value. An empty string is
// treated as "auto" (the default when the flag is entirely absent);
// callers that want bare `--color` to mean "always" (GNU grep's
// convention) should set that via pflag's NoOptDefVal rather than
// relying on this function to guess — see internal/cli.
func ParseColorMode(s string) (ColorMode, error) {
	switch strings.ToLower(s) {
	case "", "auto":
		return ColorAuto, nil
	case "always":
		return ColorAlways, nil
	case "never":
		return ColorNever, nil
	default:
		return ColorAuto, fmt.Errorf("gg: invalid --color value %q (want auto, always, or never)", s)
	}
}

// resolveColor decides whether color should actually be used, combining
// the requested mode with the real terminal state of stdout — the
// policy decision internal/termenv and internal/encoding's sibling
// packages explicitly defer to their caller (see their package docs).
func resolveColor(mode ColorMode, stdout io.Writer) bool {
	switch mode {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	default: // ColorAuto
		f, ok := stdout.(*os.File)
		if !ok {
			return false // not a real file/console handle at all (e.g. a test buffer)
		}
		if !termenv.IsTerminal(f) {
			return false
		}
		return termenv.EnableANSI(f)
	}
}
