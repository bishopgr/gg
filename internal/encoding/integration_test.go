package encoding_test

import (
	"bufio"
	"bytes"
	"io"
	"testing"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"gg/internal/encoding"
	"gg/internal/lineio"
)

// TestSniffThenLineio_NoDoubleBuffering pins down the PROCESS.md §4
// step 8 fix directly at the real composition point (rather than only
// in internal/lineio's own white-box test): the *bufio.Reader Sniff
// hands back for non-UTF-16 content must be sized so that
// lineio.NewReader recognizes and reuses it instead of layering a
// second buffer on top, which would otherwise cap every downstream
// fill at whatever Sniff's own buffer returns per Read.
func TestSniffThenLineio_NoDoubleBuffering(t *testing.T) {
	_, r := encoding.Sniff(bytes.NewReader([]byte("plain ascii, no BOM\n")))

	br, ok := r.(*bufio.Reader)
	if !ok {
		t.Fatalf("Sniff's returned Reader is %T, want *bufio.Reader (required for lineio.NewReader's reuse check)", r)
	}
	if br.Size() < lineio.DefaultBufSize {
		t.Fatalf("Sniff's buffer capacity = %d, want >= lineio.DefaultBufSize (%d) so lineio.NewReader reuses it directly", br.Size(), lineio.DefaultBufSize)
	}

	lr := lineio.NewReader(r)
	if lr.Buffered() != br.Buffered() {
		// Buffered() delegates straight through when the same
		// *bufio.Reader is reused; a mismatch means lineio.NewReader
		// wrapped it in a second bufio.Reader instead of reusing it.
		t.Fatalf("lineio.NewReader appears to have wrapped Sniff's Reader in a second buffer (Buffered() = %d, want %d)", lr.Buffered(), br.Buffered())
	}
}

// TestSniffThenLineio proves the intended composition from PROCESS.md
// §3.2: Sniff transcodes/strips first, and its output Reader feeds
// directly into lineio.NewReader, so a UTF-16LE file with Windows CRLF
// line endings splits into the exact same lines a plain UTF-8/LF file
// would.
func TestSniffThenLineio(t *testing.T) {
	plain := "first line\r\nsecond line\r\nthird line\r\n"

	enc := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder()
	encoded, _, err := transform.Bytes(enc, []byte(plain))
	if err != nil {
		t.Fatalf("encode UTF-16LE fixture: %v", err)
	}

	result, r := encoding.Sniff(bytes.NewReader(encoded))
	if result.BOM != encoding.UTF16LEBOM {
		t.Fatalf("BOM = %v, want UTF16LEBOM", result.BOM)
	}
	if result.Binary {
		t.Fatal("Binary = true, want false")
	}

	lr := lineio.NewReader(r)
	var got []string
	for {
		line, err := lr.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("ReadLine: %v", err)
		}
		if line.Terminator != lineio.CRLF {
			t.Errorf("line %q terminator = %v, want CRLF", line.Text, line.Terminator)
		}
		got = append(got, string(line.Text))
	}

	want := []string{"first line", "second line", "third line"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %v, want %d lines %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}
