package encoding

import (
	"bytes"
	"io"
	"testing"
)

// FuzzSniff feeds arbitrary bytes through Sniff and drains the returned
// Reader. The property under test is simply: never panic, never hang,
// and — for the non-UTF-16 path, where Sniff does not transform bytes,
// only strip a BOM prefix — reproduce the remaining input exactly.
//
// The UTF-16 path is exercised but not content-asserted here: arbitrary
// fuzz bytes following a UTF-16 BOM are not guaranteed to be valid
// UTF-16 (e.g. unpaired surrogates), so the decoder may legitimately
// emit replacement characters or a decode error. That's correct
// behavior, not a bug, so it isn't a fuzz failure.
func FuzzSniff(f *testing.F) {
	seeds := [][]byte{
		{},
		[]byte("hello\nworld\n"),
		utf8BOMBytes,
		append(append([]byte{}, utf8BOMBytes...), "hello"...),
		utf16LEBOMBytes,
		append(append([]byte{}, utf16LEBOMBytes...), 'h', 0x00, 'i', 0x00),
		utf16BEBOMBytes,
		append(append([]byte{}, utf16BEBOMBytes...), 0x00, 'h', 0x00, 'i'),
		[]byte("PK\x03\x04\x00\x00binary"),
		bytes.Repeat([]byte{0x00}, 20),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		result, r := Sniff(bytes.NewReader(data))

		got, err := io.ReadAll(r)
		if err != nil {
			// A decode error is only expected on the UTF-16 path with
			// malformed input; anything else is a bug.
			if result.BOM != UTF16LEBOM && result.BOM != UTF16BEBOM {
				t.Fatalf("unexpected read error for BOM=%v: %v", result.BOM, err)
			}
			return
		}

		if result.BOM == UTF16LEBOM || result.BOM == UTF16BEBOM {
			return // transcoded; no byte-for-byte relationship to assert
		}

		want := data[bomLen(result.BOM):]
		if !bytes.Equal(got, want) {
			t.Fatalf("non-UTF-16 path altered bytes: got %v, want %v (input %v, BOM %v)",
				got, want, data, result.BOM)
		}
	})
}
