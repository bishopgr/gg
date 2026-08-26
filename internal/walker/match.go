package walker

import (
	"path/filepath"
	"runtime"
	"strings"
)

// matchAny reports whether name matches any of patterns, per
// matchOneCaseAware's case-sensitivity rule.
func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if matchOne(p, name) {
			return true
		}
	}
	return false
}

// matchOne matches a single glob pattern against name, case-aware for
// the current OS.
func matchOne(pattern, name string) bool {
	return matchOneCaseAware(pattern, name, runtime.GOOS == "windows")
}

// matchOneCaseAware matches pattern against name using filepath.Match
// glob syntax, folding case first when caseInsensitive is true.
//
// PROCESS.md §3.4: Windows paths are case-insensitive but
// case-preserving, so a `--exclude=*.LOG` should still exclude
// "error.log" the way it would on the Windows shell/filesystem tools
// users already know; on Unix, filenames are case-sensitive, and a
// case-insensitive match there would be a surprising, wrong difference
// from every native tool (grep, find, ls).
//
// Split out from matchOne so both branches are directly unit-testable
// on a single CI runner, without needing platform-specific build tags
// for what is otherwise identical logic.
func matchOneCaseAware(pattern, name string, caseInsensitive bool) bool {
	if caseInsensitive {
		pattern = strings.ToLower(pattern)
		name = strings.ToLower(name)
	}
	ok, err := filepath.Match(pattern, name)
	return err == nil && ok
}
