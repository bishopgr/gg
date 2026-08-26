package lineio

import (
	"bytes"
	"strings"
	"testing"
)

// genLines builds n lines of roughly lineLen bytes each, joined with the
// given terminator — synthetic but representative of real log-file
// shapes (short-to-medium lines, mostly-printable ASCII). Used by every
// benchmark below so they all exercise the same input shape and only
// the terminator/line-length varies between cases.
func genLines(n, lineLen int, term string) []byte {
	var buf bytes.Buffer
	buf.Grow(n * (lineLen + len(term)))
	line := strings.Repeat("x", lineLen-1) + "y" // avoid a degenerate all-'x' line
	for i := 0; i < n; i++ {
		buf.WriteString(line)
		buf.WriteString(term)
	}
	return buf.Bytes()
}

// BenchmarkReadLine_LF measures the steady-state cost of splitting a
// large LF-terminated stream into lines — the common case on every
// platform gg targets, and the one PROCESS.md §4 step 8 calls out
// specifically ("avoid per-line allocations in the hot path").
func BenchmarkReadLine_LF(b *testing.B) {
	benchmarkReadLine(b, "\n")
}

// BenchmarkReadLine_CRLF is the Windows-authored-file counterpart: same
// content, different terminator, so any regression specific to the CRLF
// path (e.g. from the ReadByte/UnreadByte pushback) shows up on its own.
func BenchmarkReadLine_CRLF(b *testing.B) {
	benchmarkReadLine(b, "\r\n")
}

// BenchmarkReadLine_BareCR is the classic-Mac-OS9 edge case: no '\n'
// anywhere in the entire input. Worth benchmarking specifically, not
// just table-testing for correctness, because the terminator-detection
// scan below is asymmetric: finding '\n' first (the common case) means
// the '\r' search is bounded by '\n's position, but when '\n' never
// appears at all, bounding degrades to "search the whole buffered
// chunk," which is only proportional to *that line's own length* when
// '\r' occurs frequently (as in real classic-Mac text) — see the
// ReadLine doc comment for the full reasoning and the accepted,
// documented limit of this asymmetry.
func BenchmarkReadLine_BareCR(b *testing.B) {
	benchmarkReadLine(b, "\r")
}

func benchmarkReadLine(b *testing.B, term string) {
	const (
		lines   = 50_000
		lineLen = 80
	)
	data := genLines(lines, lineLen, term)

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(data)
		rd := NewReader(r)
		for {
			_, err := rd.ReadLine()
			if err != nil {
				break // io.EOF: expected end of this iteration's input
			}
		}
	}
}

// BenchmarkReadLine_Reuse measures the Reset path specifically — the
// shape internal/grep actually uses across a multi-file recursive
// search (one Reader, many files) rather than allocating a fresh Reader
// per file.
func BenchmarkReadLine_Reuse(b *testing.B) {
	const (
		lines   = 50_000
		lineLen = 80
	)
	data := genLines(lines, lineLen, "\n")

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()

	rd := NewReader(bytes.NewReader(data))
	for i := 0; i < b.N; i++ {
		rd.Reset(bytes.NewReader(data))
		for {
			_, err := rd.ReadLine()
			if err != nil {
				break
			}
		}
	}
}
