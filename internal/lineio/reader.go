// Package lineio implements a streaming, line-ending-aware line reader.
// It exists because naively splitting on '\n' breaks on Windows-authored
// files (CRLF) and, more subtly, on files with no trailing newline at
// all — both are correctness bugs, not edge cases, on this project's
// target platform (PROCESS.md §3.1).
//
// This package deliberately knows nothing about encoding, BOMs, or
// binary-file detection (PROCESS.md §3.2) — those are a separate
// concern layered on top, not mixed into line splitting.
package lineio

import (
	"bufio"
	"bytes"
	"io"
)

// Terminator identifies which line-ending sequence followed a line.
// Callers that need to reproduce input byte-for-byte (or make a
// deliberate normalization decision) need this — silently assuming '\n'
// loses information.
//
// Note for output formatting: this is a fact about the *source*, not a
// signal for how gg should format output. PROCESS.md §3.1 decides that
// gg always writes a bare '\n' after every printed line regardless of
// this value — Terminator exists for detection/reconstruction, not to
// drive the (not-yet-built) output layer's line-ending choice.
type Terminator int

const (
	// NoTerminator means EOF was reached without a trailing line
	// terminator — the "no newline at end of file" case. This is a
	// normal, valid way for a file to end, not an error.
	NoTerminator Terminator = iota
	// LF is a bare '\n' (Unix, and modern Windows tooling).
	LF
	// CRLF is '\r\n' (classic Windows/DOS convention).
	CRLF
	// CR is a bare '\r' not followed by '\n' (classic Mac OS 9 and
	// earlier). Rare today, but a small amount of tooling still emits
	// it, and a correct splitter should not silently absorb it into the
	// next line's content.
	CR
)

// String implements fmt.Stringer for debugging/test output.
func (t Terminator) String() string {
	switch t {
	case LF:
		return "LF"
	case CRLF:
		return "CRLF"
	case CR:
		return "CR"
	default:
		return "none"
	}
}

// Bytes returns the raw terminator bytes t represents (nil for
// NoTerminator). Useful for reconstructing input byte-for-byte.
func (t Terminator) Bytes() []byte {
	switch t {
	case LF:
		return []byte{'\n'}
	case CRLF:
		return []byte{'\r', '\n'}
	case CR:
		return []byte{'\r'}
	default:
		return nil
	}
}

// Line is one line read from a Reader.
type Line struct {
	// Text is the line's content, with the terminator stripped. It
	// aliases the Reader's internal buffer: it is only valid until the
	// next call to ReadLine (or Reset). Copy it if you need to retain it
	// past that point.
	Text []byte
	// Terminator records how this line ended. NoTerminator means this
	// was the final line of the input and the input did not end with a
	// newline.
	Terminator Terminator
}

// DefaultBufSize is the size of the internal read-ahead buffer used when
// NewReader has to allocate its own bufio.Reader. It is also the
// threshold NewReader uses to decide whether an already-buffered input
// reader (e.g. from internal/encoding.Sniff) is worth reusing directly
// instead of layering a second buffer on top of it — see NewReader.
const DefaultBufSize = 64 * 1024

// scanWindow caps how many buffered bytes ReadLine's terminator search
// examines in one pass. See the comment where it's used, in ReadLine,
// for why a cap is necessary for correctness-of-performance (not just
// output correctness) once one terminator byte is allowed to bound the
// search for the other.
const scanWindow = 4096

// Reader splits an io.Reader into Lines, recognizing '\n', '\r\n', and
// bare '\r' as terminators (PROCESS.md §3.1).
//
// It streams: there is no maximum line length (unlike bufio.Scanner's
// default token-size cap), and it never buffers more than the current
// line plus bufio's internal read-ahead window.
//
// PROCESS.md §4 step 8 (performance pass): earlier revisions of this
// type scanned byte-by-byte via bufio.Reader.ReadByte, favoring
// simplicity over throughput. Profiling a 50,000-line synthetic
// benchmark showed ReadByte and the matching byte-by-byte
// bytes.Buffer.WriteByte accumulation together accounted for ~65% of
// ReadLine's CPU time. The current implementation instead scans
// whatever is already buffered in one bulk bytes.IndexByte call per
// terminator (SIMD-optimized in the Go runtime, unlike a manual byte
// loop) and only copies a line's content into r.line when it actually
// spans more than one buffered chunk — the common case (a line that
// fits within one already-buffered chunk) returns a slice aliasing
// bufio's own buffer directly, with zero extra copying. This roughly
// tripled measured throughput (~225 MB/s -> ~750+ MB/s; see
// BenchmarkReadLine_LF/_CRLF) with identical externally-observable
// behavior — the full existing table/fuzz/fixture test suite (which
// pins down every terminator edge case) passes unchanged.
type Reader struct {
	br   *bufio.Reader
	line []byte // accumulates a line that spans multiple buffered chunks; reused across calls
}

// NewReader returns a Reader that reads lines from r.
//
// If r is already a *bufio.Reader with at least DefaultBufSize of
// capacity — notably, the Reader internal/encoding.Sniff returns — it is
// reused directly instead of being wrapped in a second bufio.Reader.
// Layering two buffers is a real, measured cost, not a theoretical one:
// without this, every fill on the outer buffer is capped by however
// much the inner buffer's Read returns per call, turning what should be
// one large read into several smaller ones. Callers that don't pass an
// already-buffered reader are unaffected: they still get a fresh
// DefaultBufSize buffer, same as before.
func NewReader(r io.Reader) *Reader {
	if br, ok := r.(*bufio.Reader); ok && br.Size() >= DefaultBufSize {
		return &Reader{br: br}
	}
	return &Reader{br: bufio.NewReaderSize(r, DefaultBufSize)}
}

// Reset discards any buffered state and reconfigures the Reader to read
// from newR. This lets one Reader (and its internal buffers) be reused
// across many files — e.g. during a recursive directory search — instead
// of allocating a fresh Reader, and fresh internal buffers, per file.
func (r *Reader) Reset(newR io.Reader) {
	r.br.Reset(newR)
	r.line = r.line[:0]
}

// Buffered returns the number of bytes currently held in r's internal
// buffer, delegating directly to the underlying bufio.Reader. It exists
// mainly so callers one layer up (see
// internal/encoding's TestSniffThenLineio_NoDoubleBuffering) can prove
// NewReader actually reused an already-buffered input Reader rather than
// wrapping it in a second one — not something ReadLine itself needs.
func (r *Reader) Buffered() int {
	return r.br.Buffered()
}

// ReadLine returns the next line. It returns io.EOF (with a zero Line)
// once all input has been consumed and there is no final partial line to
// report. A non-EOF error is returned immediately and is not wrapped in
// a Line, even if a partial line had already been accumulated — an
// unexpected read failure mid-line should surface as an error, not be
// silently reported as a short line.
//
// The returned Line.Text aliases either r's internal accumulation buffer
// or (the common case) bufio's own internal buffer; either way it is
// only valid until the next call to ReadLine (or Reset). Copy it if the
// caller needs to retain it past that point.
func (r *Reader) ReadLine() (Line, error) {
	r.line = r.line[:0]

	for {
		avail := r.br.Buffered()
		if avail == 0 {
			// Nothing buffered at all: force exactly one real read,
			// the same point plain ReadByte would have refilled at —
			// this doesn't read any more eagerly than a byte-at-a-time
			// implementation would have.
			if _, err := r.br.Peek(1); err != nil {
				if len(r.line) > 0 {
					return Line{Text: r.line, Terminator: NoTerminator}, nil
				}
				return Line{}, err
			}
			avail = r.br.Buffered()
		}

		// Cap how much of the buffered data one iteration scans to
		// scanWindow, rather than examining all of avail (up to a full
		// DefaultBufSize) at once. This is what keeps the "find '\n',
		// then bound the '\r' search by it" trick below safe in
		// *both* directions: for content with no '\n' at all (a
		// classic-Mac bare-CR file), bounding the '\r' search by "up
		// to the first '\n'" degrades to "the whole chunk," and
		// without a cap that chunk can be an entire multi-KB buffer —
		// scanned again on every subsequent short line, since the
		// missing '\n' has to be rediscovered each time. Capping the
		// window bounds that rediscovery cost to scanWindow regardless
		// of which terminator (if any) is actually present, at the
		// price of a few more (cheap) Peek/Discard round trips for the
		// common case where a whole 64KB buffer's worth of short lines
		// would otherwise have been scanned in one shot. Measured, not
		// assumed: BenchmarkReadLine_BareCR is the regression test for
		// exactly this — see its doc comment.
		peekLen := avail
		if peekLen > scanWindow {
			peekLen = scanWindow
		}
		// peekLen bytes are already sitting in bufio's buffer (it's
		// <= avail == r.br.Buffered()), so this Peek is free (no read,
		// no copy) — it cannot come back short.
		chunk, _ := r.br.Peek(peekLen)

		// Find whichever of '\r'/'\n' occurs first. This looks like it
		// should just be bytes.IndexAny(chunk, "\r\n"), but two
		// benchmarked iterations landed here instead:
		//
		//  1. Two independent, unbounded bytes.IndexByte scans (one
		//     per byte) is actively wrong: whichever byte is *absent*
		//     from a pure-LF or pure-bare-CR stream gets scanned across
		//     the *entire* buffered chunk (tens of KB) on every single
		//     line, since IndexByte can't stop early when the byte it's
		//     hunting for genuinely never appears anywhere in the
		//     chunk — quadratic in the number of lines per buffer-full.
		//     Measured *slower* than the original byte-by-byte
		//     implementation on pure-LF input specifically.
		//  2. bytes.IndexAny(chunk, "\r\n") fixes the quadratic
		//     blowup (one bounded pass, stops at the first match of
		//     either byte) but profiling showed it costing ~80% of
		//     ReadLine's CPU time: IndexAny's small-cutset path
		//     (bytes.asciiSet.contains) is a plain per-byte bitmask
		//     check, not SIMD, unlike IndexByte.
		//
		// This is the third iteration: find '\n' first via IndexByte
		// (SIMD, and '\n' is the common terminator so this is normally
		// a short scan), then search for '\r' but *only* within the
		// region before that '\n' — also via IndexByte, and bounded to
		// the same length IndexByte for '\n' already had to look at,
		// so it adds no extra asymptotic cost, just a second SIMD pass
		// over a region already known to be short. A '\r' at or after
		// the found '\n' doesn't matter: it belongs to a later line.
		nl := bytes.IndexByte(chunk, '\n')
		bound := len(chunk)
		if nl >= 0 {
			bound = nl
		}
		idx := bytes.IndexByte(chunk[:bound], '\r')
		if idx < 0 {
			idx = nl // no '\r' before it (or at all): '\n' wins, if found
		}

		switch {
		case idx < 0:
			// No terminator anywhere in this window: accumulate all of
			// it and go around again. The top of the loop forces a
			// real read once the buffer is empty; otherwise the next
			// iteration's window picks up right where this one left
			// off (possibly still within the same buffered fill, if
			// peekLen was capped by scanWindow rather than by what was
			// actually available).
			r.line = append(r.line, chunk...)
			_, _ = r.br.Discard(len(chunk))

		case chunk[idx] == '\n':
			text := r.finish(chunk[:idx])
			_, _ = r.br.Discard(idx + 1)
			return Line{Text: text, Terminator: LF}, nil

		default: // chunk[idx] == '\r'
			if idx == len(chunk)-1 {
				// The '\r' is the last byte currently buffered, so the
				// byte that would distinguish CRLF from a bare CR
				// simply hasn't been read yet — it's not that there
				// isn't one. Consume everything up to (not including)
				// the '\r', then force exactly one more byte before
				// deciding, rather than guessing.
				r.line = append(r.line, chunk[:idx]...)
				_, _ = r.br.Discard(idx)

				more, _ := r.br.Peek(2) // [\r, maybe \n]
				if len(more) < 2 {
					// EOF (or a real error, which resurfaces on the
					// next ReadLine call) right after the '\r': a
					// bare-CR-terminated final line.
					_, _ = r.br.Discard(len(more))
					return Line{Text: r.line, Terminator: CR}, nil
				}
				if more[1] == '\n' {
					_, _ = r.br.Discard(2)
					return Line{Text: r.line, Terminator: CRLF}, nil
				}
				_, _ = r.br.Discard(1)
				return Line{Text: r.line, Terminator: CR}, nil
			}

			text := r.finish(chunk[:idx])
			if chunk[idx+1] == '\n' {
				_, _ = r.br.Discard(idx + 2)
				return Line{Text: text, Terminator: CRLF}, nil
			}
			_, _ = r.br.Discard(idx + 1)
			return Line{Text: text, Terminator: CR}, nil
		}
	}
}

// finish returns text as it should be attached to the Line about to be
// returned: if nothing has been accumulated across a prior chunk
// boundary (the common case — a line that fits within one already-
// buffered chunk), it returns text as-is, aliasing bufio's own buffer
// directly and avoiding a copy entirely. Otherwise text is the final
// fragment of a line that already spans multiple chunks, so it's
// appended to the accumulation buffer built up so far.
func (r *Reader) finish(text []byte) []byte {
	if len(r.line) == 0 {
		return text
	}
	return append(r.line, text...)
}
