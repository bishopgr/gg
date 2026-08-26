// Package encoding inspects the start of a byte stream to detect a
// byte-order mark (BOM) and decide whether the stream looks like binary
// data, per PROCESS.md §3.2. It is a separate concern from
// internal/lineio deliberately: lineio only ever sees UTF-8 (or raw
// binary) bytes with any BOM already stripped, and knows nothing about
// encodings.
//
// Detection only: this package does not decide *policy* (e.g. whether a
// binary file should be skipped with a "Binary file X matches" message,
// or force-read as text under -a/--text) — that belongs to the grep
// engine, once it exists, which is better placed to know about flags.
package encoding

import (
	"bufio"
	"bytes"
	"io"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// BOM identifies which byte-order mark, if any, was found at the start
// of a stream.
type BOM int

const (
	// NoBOM means no recognized byte-order mark was present.
	NoBOM BOM = iota
	// UTF8BOM is the 3-byte UTF-8 BOM (EF BB BF). It carries no
	// endianness information; it exists only as a signature. It is
	// stripped but does not trigger transcoding — the bytes after it
	// are already UTF-8.
	UTF8BOM
	// UTF16LEBOM is the 2-byte UTF-16 little-endian BOM (FF FE).
	UTF16LEBOM
	// UTF16BEBOM is the 2-byte UTF-16 big-endian BOM (FE FF).
	UTF16BEBOM
)

func (b BOM) String() string {
	switch b {
	case UTF8BOM:
		return "UTF-8 BOM"
	case UTF16LEBOM:
		return "UTF-16LE BOM"
	case UTF16BEBOM:
		return "UTF-16BE BOM"
	default:
		return "none"
	}
}

var (
	utf8BOMBytes    = []byte{0xEF, 0xBB, 0xBF}
	utf16LEBOMBytes = []byte{0xFF, 0xFE}
	utf16BEBOMBytes = []byte{0xFE, 0xFF}
)

// sniffLen is how many leading bytes are inspected to decide whether a
// stream looks binary. This mirrors Git's buffer_is_binary heuristic (an
// 8000-byte prefix) rather than GNU grep's exact, version-dependent
// buffering — it's simple, well-precedented, and enough to catch the
// overwhelming majority of real binary files (images, archives,
// executables) from their first chunk.
const sniffLen = 8000

// sniffBufSize is the capacity of the bufio.Reader Sniff constructs —
// deliberately larger than sniffLen. It's set to match
// lineio.DefaultBufSize (duplicated as a literal rather than imported,
// to avoid this package depending on lineio just for a tuning constant)
// so that lineio.NewReader, given Sniff's returned Reader directly, can
// recognize it as already adequately buffered and reuse it instead of
// wrapping it in a second bufio.Reader. PROCESS.md §4 step 8
// (performance pass): profiling the full source->Sniff->lineio pipeline
// showed the previous sniffLen-sized (8000-byte) buffer capping every
// downstream fill to at most 8000 bytes per underlying Read call, since
// an outer bufio.Reader's fill can't get more from an io.Reader than
// that Reader's own Read call returns in one shot — turning what should
// be one 64KB read into several smaller ones.
const sniffBufSize = lineioDefaultBufSize

// lineioDefaultBufSize mirrors internal/lineio.DefaultBufSize — see
// sniffBufSize's doc comment for why this is a literal duplicate
// instead of an import.
const lineioDefaultBufSize = 64 * 1024

// Result describes what Sniff found at the start of a stream.
type Result struct {
	// BOM is the byte-order mark detected and stripped, if any.
	BOM BOM
	// Binary is true if a NUL byte was found in the sniffed prefix,
	// after any BOM. It is always false when BOM is UTF16LEBOM or
	// UTF16BEBOM — see the note in Sniff about why the NUL-byte
	// heuristic cannot be applied to UTF-16 content.
	Binary bool
}

// Sniff inspects the start of r to detect a BOM and decide whether the
// stream looks binary, returning a Result plus a Reader that continues
// exactly where the original content does: any detected BOM is
// consumed, and UTF-16 content is transcoded to UTF-8 so every
// downstream consumer (lineio, matcher) only ever has to deal with UTF-8
// or arbitrary binary bytes — never UTF-16.
//
// Sniff peeks at most sniffLen bytes ahead of what it returns and never
// reads the rest of r eagerly, so it is safe to use on arbitrarily large
// or slow/streaming inputs.
func Sniff(r io.Reader) (Result, io.Reader) {
	br := bufio.NewReaderSize(r, sniffBufSize)
	prefix, _ := br.Peek(sniffLen) // best-effort: short reads/EOF are fine

	bom := detectBOM(prefix)
	n := bomLen(bom)
	if n > 0 {
		// n bytes are guaranteed to be in the buffer: detectBOM only
		// returns a match when prefix is at least that long.
		_, _ = br.Discard(n)
	}

	switch bom {
	case UTF16LEBOM:
		// A UTF-16 BOM positively identifies this as text, so skip the
		// NUL-byte heuristic entirely: every ASCII character in UTF-16
		// has a zero high or low byte, which would otherwise flag
		// nearly all UTF-16 text as "binary."
		return Result{BOM: bom}, transcodeUTF16(br, unicode.LittleEndian)
	case UTF16BEBOM:
		return Result{BOM: bom}, transcodeUTF16(br, unicode.BigEndian)
	default:
		return Result{
			BOM:    bom,
			Binary: bytes.IndexByte(prefix[n:], 0) >= 0,
		}, br
	}
}

func detectBOM(prefix []byte) BOM {
	switch {
	case bytes.HasPrefix(prefix, utf8BOMBytes):
		return UTF8BOM
	case bytes.HasPrefix(prefix, utf16LEBOMBytes):
		return UTF16LEBOM
	case bytes.HasPrefix(prefix, utf16BEBOMBytes):
		return UTF16BEBOM
	default:
		return NoBOM
	}
}

func bomLen(b BOM) int {
	switch b {
	case UTF8BOM:
		return len(utf8BOMBytes)
	case UTF16LEBOM, UTF16BEBOM:
		return 2
	default:
		return 0
	}
}

// transcodeUTF16 wraps r in a streaming UTF-16-to-UTF-8 decoder. The BOM
// has already been consumed by the caller, so the endianness must be
// supplied explicitly (unicode.IgnoreBOM: a second, mid-stream BOM-like
// sequence is decoded as data, not treated as another signature).
func transcodeUTF16(r io.Reader, bo unicode.Endianness) io.Reader {
	dec := unicode.UTF16(bo, unicode.IgnoreBOM).NewDecoder()
	return transform.NewReader(r, dec)
}
