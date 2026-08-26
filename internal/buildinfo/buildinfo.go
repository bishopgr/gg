// Package buildinfo holds build-time metadata populated via -ldflags at
// compile time (see scripts/build.ps1). Defaults are used for `go run`
// and unversioned `go build` invocations during development.
package buildinfo

import "fmt"

var (
	// Version is a semver tag or `git describe` output.
	Version = "dev"
	// Commit is the short git commit hash.
	Commit = "none"
	// Date is the UTC build timestamp (RFC3339).
	Date = "unknown"
)

// String returns a human-readable summary of the build metadata, e.g.
// "v0.1.0 (commit=abc1234, built=2026-08-26T00:00:00Z)".
func String() string {
	return fmt.Sprintf("%s (commit=%s, built=%s)", Version, Commit, Date)
}
