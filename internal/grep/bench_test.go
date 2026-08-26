package grep

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// benchFixture builds a synthetic log-shaped file with n lines, one in
// every hitEvery lines containing needle, and returns its path. This is
// the end-to-end counterpart to internal/lineio's and internal/matcher's
// per-component benchmarks — it exercises the full
// source -> encoding.Sniff -> lineio -> matcher -> output pipeline
// exactly as a real invocation would, per PROCESS.md §4 step 8's call
// for benchmarks "over a generated large fixture."
func benchFixture(b *testing.B, n, hitEvery int) string {
	b.Helper()
	dir := b.TempDir()
	path := filepath.Join(dir, "bench.log")

	f, err := os.Create(path)
	if err != nil {
		b.Fatalf("create fixture: %v", err)
	}
	defer f.Close()

	var sb strings.Builder
	sb.Grow(n * 90)
	for i := 0; i < n; i++ {
		sb.WriteString("2024-01-01T00:00:00Z INFO worker-")
		sb.WriteString(strconv.Itoa(i % 64))
		if hitEvery > 0 && i%hitEvery == 0 {
			sb.WriteString(" needle-pattern-found")
		} else {
			sb.WriteString(" nothing interesting here at all")
		}
		sb.WriteByte('\n')
	}
	if _, err := f.WriteString(sb.String()); err != nil {
		b.Fatalf("write fixture: %v", err)
	}
	return path
}

// BenchmarkRun_Regex_SparseMatches is the realistic common case: a
// regex pattern, most lines don't match, normal (non-quiet) output mode
// so every line still has to be read, matched, and (for the few hits)
// formatted.
func BenchmarkRun_Regex_SparseMatches(b *testing.B) {
	path := benchFixture(b, 100_000, 500)
	benchmarkRun(b, Options{Pattern: `needle-\w+-found`, Files: []string{path}})
}

// BenchmarkRun_Fixed_SparseMatches is the -F counterpart, exercising
// fixedMatcher's plain bytes.Index scan instead of regexp.
func BenchmarkRun_Fixed_SparseMatches(b *testing.B) {
	path := benchFixture(b, 100_000, 500)
	benchmarkRun(b, Options{Pattern: "needle-pattern-found", Fixed: true, Files: []string{path}})
}

// BenchmarkRun_Quiet_FirstLineMatches measures the early-exit path
// (-q): matching on line 1 should make this essentially O(1) in file
// size, not O(n) — a regression here would mean quiet mode stopped
// actually stopping.
func BenchmarkRun_Quiet_FirstLineMatches(b *testing.B) {
	path := benchFixture(b, 100_000, 1) // every line matches, including the first
	benchmarkRun(b, Options{Pattern: "needle-pattern-found", Quiet: true, Files: []string{path}})
}

func benchmarkRun(b *testing.B, opts Options) {
	b.Helper()
	info, err := os.Stat(opts.Files[0])
	if err != nil {
		b.Fatalf("stat fixture: %v", err)
	}
	b.SetBytes(info.Size())
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		o := opts
		o.Stdout = io.Discard
		o.Stderr = io.Discard
		if _, err := Run(context.Background(), o); err != nil {
			b.Fatalf("Run: %v", err)
		}
	}
}
