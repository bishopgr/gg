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

// Reader splits an io.Reader into Lines, recognizing '\n', '\r\n', and
// bare '\r' as terminators (PROCESS.md §3.1).
//
// It streams: there is no maximum line length (unlike bufio.Scanner's
// default token-size cap), and it never buffers more than the current
// line plus bufio's internal read-ahead window.
//
// The current implementation scans byte-by-byte via bufio.Reader.ReadByte,
// which favors simplicity/correctness over throughput. PROCESS.md §4 step
// 8 (performance pass) is the deliberate point to revisit this — e.g.
// bulk-scanning buffered chunks with bytes.IndexAny — once there is a
// passing test suite and a benchmark to protect.
type Reader struct {
	br  *bufio.Reader
	buf bytes.Buffer // accumulates the current line; reused across calls
}

// NewReader returns a Reader that reads lines from r.
func NewReader(r io.Reader) *Reader {
	return &Reader{br: bufio.NewReaderSize(r, 64*1024)}
}

// Reset discards any buffered state and reconfigures the Reader to read
// from newR. This lets one Reader (and its internal buffers) be reused
// across many files — e.g. during a recursive directory search — instead
// of allocating a fresh Reader, and fresh internal buffers, per file.
func (r *Reader) Reset(newR io.Reader) {
	r.br.Reset(newR)
	r.buf.Reset()
}

// ReadLine returns the next line. It returns io.EOF (with a zero Line)
// once all input has been consumed and there is no final partial line to
// report. A non-EOF error is returned immediately and is not wrapped in
// a Line, even if a partial line had already been accumulated — an
// unexpected read failure mid-line should surface as an error, not be
// silently reported as a short line.
//
// The returned Line.Text aliases an internal buffer that is overwritten
// by the next call to ReadLine; copy it if the caller needs to retain it.
func (r *Reader) ReadLine() (Line, error) {
	r.buf.Reset()

	for {
		b, err := r.br.ReadByte()
		if err != nil {
			if err == io.EOF && r.buf.Len() > 0 {
				// Final line with no trailing terminator.
				return Line{Text: r.buf.Bytes(), Terminator: NoTerminator}, nil
			}
			return Line{}, err
		}

		switch b {
		case '\n':
			return Line{Text: r.buf.Bytes(), Terminator: LF}, nil

		case '\r':
			next, err := r.br.ReadByte()
			switch {
			case err == nil && next == '\n':
				return Line{Text: r.buf.Bytes(), Terminator: CRLF}, nil
			case err == nil:
				// Not a newline: it belongs to the next line's content,
				// so push it back rather than consuming it.
				_ = r.br.UnreadByte()
				return Line{Text: r.buf.Bytes(), Terminator: CR}, nil
			default:
				// EOF (or a real error, which will resurface on the next
				// ReadLine call) right after the '\r': treat it as a
				// bare-CR-terminated final line.
				return Line{Text: r.buf.Bytes(), Terminator: CR}, nil
			}

		default:
			r.buf.WriteByte(b)
		}
	}
}
