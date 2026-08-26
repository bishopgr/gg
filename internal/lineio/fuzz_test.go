package lineio

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// FuzzReader_RoundTrip is the strongest correctness property this
// package has: for arbitrary bytes (including invalid UTF-8 and embedded
// NULs — Reader operates on raw bytes, not runes or text), concatenating
// every Line's Text and Terminator.Bytes() back together must reproduce
// the original input exactly. If this ever fails, ReadLine is dropping,
// duplicating, or misclassifying bytes somewhere.
func FuzzReader_RoundTrip(f *testing.F) {
	seeds := []string{
		"",
		"a",
		"a\n",
		"a\nb",
		"a\r\nb\r\n",
		"a\rb\rc",
		"\n\n\n",
		"\r\r\r",
		"\r\n\r\n",
		"mixed\r\nendings\nhere\rand\r\nthere",
		"\x00binary\x00bytes\r\nwith\rmixed\nterminators",
		string([]byte{0xff, 0xfe, '\r', '\n', 0x80}), // invalid UTF-8
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data string) {
		r := NewReader(strings.NewReader(data))

		var reconstructed bytes.Buffer
		lineCount := 0
		for {
			line, err := r.ReadLine()
			if err != nil {
				if err == io.EOF {
					break
				}
				t.Fatalf("unexpected error for input %q: %v", data, err)
			}
			reconstructed.Write(line.Text)
			reconstructed.Write(line.Terminator.Bytes())

			lineCount++
			if lineCount > len(data)+1 {
				// Every line consumes at least one byte except possibly
				// the very last (empty, unterminated) one — this bounds
				// out any pathological infinite-loop regression.
				t.Fatalf("more lines (%d) than input bytes (%d) for input %q; possible infinite loop",
					lineCount, len(data), data)
			}
		}

		if reconstructed.String() != data {
			t.Fatalf("round-trip mismatch:\n got: %q\nwant: %q", reconstructed.String(), data)
		}
	})
}
