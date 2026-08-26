package walker

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ExpandGlobs expands shell-style glob patterns among args that don't
// already name a literal, existing path — necessary because PowerShell
// and cmd.exe, unlike POSIX shells, never expand globs before invoking a
// process (PROCESS.md §3.4): `gg pattern *.go` reaches gg with the
// literal three characters `*.go` as argv, not a pre-expanded file list.
//
// Precedence per argument:
//  1. If it names an existing file or directory literally, it is kept
//     as-is — even if it contains glob metacharacters. A real file
//     named "a[1].txt" must not be glob-interpreted out from under the
//     user.
//  2. Otherwise, if filepath.Glob finds one or more matches, it expands
//     to those matches, sorted for determinism.
//  3. Otherwise — no literal file, no glob match, including args with
//     no glob metacharacters at all — the argument passes through
//     unchanged, so a later "no such file" error surfaces naturally.
//     This mirrors what a POSIX shell does with a non-matching glob
//     under its default (non-nullglob) behavior, so gg behaves the same
//     way on both a shell that glob-expanded for it and one that didn't.
func ExpandGlobs(args []string) ([]string, error) {
	out := make([]string, 0, len(args))

	for _, arg := range args {
		if _, err := os.Lstat(arg); err == nil {
			out = append(out, arg)
			continue
		}

		matches, err := filepath.Glob(arg)
		if err != nil {
			return nil, fmt.Errorf("gg: bad pattern %q: %w", arg, err)
		}
		if len(matches) == 0 {
			out = append(out, arg)
			continue
		}

		sort.Strings(matches)
		out = append(out, matches...)
	}

	return out, nil
}
