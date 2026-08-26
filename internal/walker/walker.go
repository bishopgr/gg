// Package walker implements recursive directory traversal for gg,
// scoped to the concerns PROCESS.md §3.4 calls out explicitly: symlink
// recursion policy, case-aware include/exclude matching, and paths
// reported exactly as the OS gives them (no forced '/' conversion).
//
// Like matcher, lineio, and encoding, this package reports facts and
// leaves policy to its caller: it does not decide whether to print a
// filename prefix, warn about binary files, or format anything — it
// just calls VisitFunc for every candidate path Options selects.
package walker

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrIsDir is passed to VisitFunc when root names a directory but
// Options.Recursive is false. Real grep refuses to search a directory's
// contents without -r/-R rather than silently doing a shallow scan (it
// doesn't have one) — Walk mirrors that instead of inventing behavior
// grep itself doesn't have.
var ErrIsDir = errors.New("is a directory")

// Options configures a walk. The zero value is a valid, conservative
// configuration: non-recursive, symlinks not followed, no filters.
type Options struct {
	// Recursive descends into subdirectories (-r/-R). If false and root
	// is a directory, Walk reports ErrIsDir instead of descending.
	Recursive bool
	// FollowSymlinks controls whether symlinks/junctions *encountered
	// while recursing* are followed. Default false (PROCESS.md §3.4):
	// avoids infinite loops from cyclic links/junctions and matches
	// grep's own non-`-R` default. A root passed directly to Walk is
	// always dereferenced regardless of this setting — matching the
	// usual convention that explicit command-line arguments are always
	// followed, the same way `cat somelink` reads through a symlink
	// without needing any special flag.
	FollowSymlinks bool
	// Include, if non-empty, restricts matched files to those whose
	// base name matches at least one of these filepath.Match glob
	// patterns (--include). Only applies to files discovered while
	// recursing — never to a root passed directly to Walk.
	Include []string
	// Exclude restricts matched files to those whose base name does not
	// match any of these glob patterns (--exclude). Same scope as Include.
	Exclude []string
	// ExcludeDir prevents descending into directories whose base name
	// matches any of these glob patterns (--exclude-dir).
	ExcludeDir []string
}

// VisitFunc is called for every candidate file Options selects, in
// deterministic lexical order (os.ReadDir's documented sort). err is
// non-nil if path itself couldn't be accessed (missing, permission
// denied, broken symlink); VisitFunc decides whether to continue
// (return nil — Walk then skips just that path/subtree and carries on)
// or abort the whole walk (return any non-nil error, which Walk
// propagates to its own caller unchanged).
type VisitFunc func(path string, err error) error

// Walk visits every file under root that opts selects. Paths passed to
// visit are built with filepath.Join from root, so they use the
// OS-native separator (backslash on Windows) and preserve root's own
// form (relative stays relative, absolute stays absolute) — deliberate,
// per PROCESS.md §3.4: grep prints paths as given/discovered, and
// force-converting to '/' would be a gratuitous difference from every
// other Windows CLI tool.
func Walk(root string, opts Options, visit VisitFunc) error {
	if err := validatePatterns(opts); err != nil {
		return err
	}

	// Command-line-given roots are always dereferenced, regardless of
	// FollowSymlinks — see the Options.FollowSymlinks doc.
	info, err := os.Stat(root)
	if err != nil {
		return visit(root, err)
	}

	if !info.IsDir() {
		return visit(root, nil)
	}
	if !opts.Recursive {
		return visit(root, ErrIsDir)
	}

	visited := map[string]bool{canonical(root): true}
	return walkDir(root, opts, visited, visit)
}

func walkDir(dir string, opts Options, visited map[string]bool, visit VisitFunc) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return visit(dir, err)
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())

		if entry.Type()&fs.ModeSymlink != 0 {
			if err := visitSymlink(path, entry.Name(), opts, visited, visit); err != nil {
				return err
			}
			continue
		}

		if entry.IsDir() {
			if matchAny(opts.ExcludeDir, entry.Name()) {
				continue
			}
			if err := walkDir(path, opts, visited, visit); err != nil {
				return err
			}
			continue
		}

		if !includeExcludeOK(opts, entry.Name()) {
			continue
		}
		if err := visit(path, nil); err != nil {
			return err
		}
	}
	return nil
}

// visitSymlink handles a directory entry that is itself a symlink
// (or, on Windows, any other reparse point Go's os package reports with
// fs.ModeSymlink — see the README caveat on junctions). Under the
// default policy it is skipped entirely: neither read as a file nor
// descended into. This is deliberately simpler than "follow to files
// but not directories" — a single, easy-to-state rule.
func visitSymlink(path, name string, opts Options, visited map[string]bool, visit VisitFunc) error {
	if !opts.FollowSymlinks {
		return nil
	}

	target, err := os.Stat(path) // follows the link
	if err != nil {
		return visit(path, err)
	}

	if !target.IsDir() {
		if !includeExcludeOK(opts, name) {
			return nil
		}
		return visit(path, nil)
	}

	key := canonical(path)
	if visited[key] {
		return nil // cycle guard: this resolved directory is already on the path
	}
	visited[key] = true

	if matchAny(opts.ExcludeDir, name) {
		return nil
	}
	return walkDir(path, opts, visited, visit)
}

func includeExcludeOK(opts Options, name string) bool {
	if len(opts.Include) > 0 && !matchAny(opts.Include, name) {
		return false
	}
	return !matchAny(opts.Exclude, name)
}

// canonical returns an absolute, symlink-resolved form of path, used
// only to detect cycles when FollowSymlinks is enabled. If resolution
// fails (a race, or a permission error on some ancestor), it falls back
// to an absolute (unresolved) form, and finally to path itself — cycle
// detection degrades gracefully rather than erroring the whole walk.
func canonical(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// validatePatterns fails fast on a malformed --include/--exclude/
// --exclude-dir glob, rather than having it silently match nothing for
// the rest of the walk (filepath.Match's ErrBadPattern is easy to swallow
// accidentally deep in a recursive matcher — checking it up front avoids
// that trap entirely).
func validatePatterns(opts Options) error {
	for _, group := range [][]string{opts.Include, opts.Exclude, opts.ExcludeDir} {
		for _, p := range group {
			if _, err := filepath.Match(p, ""); err != nil {
				return fmt.Errorf("gg: bad glob pattern %q: %w", p, err)
			}
		}
	}
	return nil
}
