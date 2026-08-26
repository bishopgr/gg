package encoding

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func TestDetectBOM(t *testing.T) {
	tests := []struct {
		name   string
		prefix []byte
		want   BOM
	}{
		{"empty", nil, NoBOM},
		{"too short for any BOM", []byte{0xEF}, NoBOM},
		{"plain ascii", []byte("hello"), NoBOM},
		{"utf-8 BOM", []byte{0xEF, 0xBB, 0xBF, 'h', 'i'}, UTF8BOM},
		{"utf-8 BOM only, no content", []byte{0xEF, 0xBB, 0xBF}, UTF8BOM},
		{"utf-16 LE BOM", []byte{0xFF, 0xFE, 'h', 0x00}, UTF16LEBOM},
		{"utf-16 BE BOM", []byte{0xFE, 0xFF, 0x00, 'h'}, UTF16BEBOM},
		{"almost a utf-8 BOM", []byte{0xEF, 0xBB, 0x00}, NoBOM},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectBOM(tt.prefix); got != tt.want {
				t.Errorf("detectBOM(%v) = %v, want %v", tt.prefix, got, tt.want)
			}
		})
	}
}

func TestSniff_NoBOM_PlainText(t *testing.T) {
	input := "hello\nworld\n"
	result, r := Sniff(strings.NewReader(input))

	if result.BOM != NoBOM {
		t.Errorf("BOM = %v, want NoBOM", result.BOM)
	}
	if result.Binary {
		t.Error("Binary = true, want false for plain ASCII text")
	}
	assertReaderContent(t, r, input)
}

func TestSniff_UTF8BOM_StrippedNotTranscoded(t *testing.T) {
	input := string(utf8BOMBytes) + "hello\nworld\n"
	result, r := Sniff(strings.NewReader(input))

	if result.BOM != UTF8BOM {
		t.Errorf("BOM = %v, want UTF8BOM", result.BOM)
	}
	if result.Binary {
		t.Error("Binary = true, want false")
	}
	assertReaderContent(t, r, "hello\nworld\n") // BOM consumed, rest untouched
}

func TestSniff_UTF8BOM_OnlyBOMNoContent(t *testing.T) {
	result, r := Sniff(bytes.NewReader(utf8BOMBytes))

	if result.BOM != UTF8BOM {
		t.Errorf("BOM = %v, want UTF8BOM", result.BOM)
	}
	assertReaderContent(t, r, "")
}

func TestSniff_BinaryDetection_NULInPrefix(t *testing.T) {
	// A representative "looks like binary" prefix: printable bytes
	// interrupted by a NUL, no BOM. Mirrors e.g. a gzip/zip header.
	input := []byte("PK\x03\x04\x00\x00some binary-ish payload")
	result, r := Sniff(bytes.NewReader(input))

	if result.BOM != NoBOM {
		t.Errorf("BOM = %v, want NoBOM", result.BOM)
	}
	if !result.Binary {
		t.Error("Binary = false, want true (input contains a NUL byte)")
	}
	// Detection must not consume or alter the data — a higher layer
	// might still choose to read it as text (-a/--text).
	assertReaderContent(t, r, string(input))
}

func TestSniff_UTF16LE_TranscodesAndIsNotFlaggedBinary(t *testing.T) {
	plain := "hello\nworld\ncafé 北京\n"
	encoded := encodeUTF16(t, plain, unicode.LittleEndian)

	result, r := Sniff(bytes.NewReader(encoded))

	if result.BOM != UTF16LEBOM {
		t.Errorf("BOM = %v, want UTF16LEBOM", result.BOM)
	}
	// The critical assertion: UTF-16 text is full of zero bytes (every
	// ASCII character has a zero high byte), but a positively-identified
	// UTF-16 BOM must short-circuit the NUL-byte heuristic.
	if result.Binary {
		t.Error("Binary = true, want false — UTF-16 BOM must suppress the NUL-byte heuristic")
	}
	assertReaderContent(t, r, plain)
}

func TestSniff_UTF16BE_TranscodesAndIsNotFlaggedBinary(t *testing.T) {
	plain := "hello\nworld\ncafé 北京\n"
	encoded := encodeUTF16(t, plain, unicode.BigEndian)

	result, r := Sniff(bytes.NewReader(encoded))

	if result.BOM != UTF16BEBOM {
		t.Errorf("BOM = %v, want UTF16BEBOM", result.BOM)
	}
	if result.Binary {
		t.Error("Binary = true, want false")
	}
	assertReaderContent(t, r, plain)
}

func TestSniff_ShortInputsDoNotPanic(t *testing.T) {
	for _, input := range []string{"", "a", "ab", "\xEF", "\xEF\xBB", "\xFF", "\xFE"} {
		t.Run("input="+input, func(t *testing.T) {
			result, r := Sniff(strings.NewReader(input))
			if result.BOM != NoBOM {
				t.Errorf("BOM = %v, want NoBOM for a too-short-to-match input", result.BOM)
			}
			assertReaderContent(t, r, input)
		})
	}
}

// --- helpers -----------------------------------------------------------

func assertReaderContent(t *testing.T, r io.Reader, want string) {
	t.Helper()
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("io.ReadAll: %v", err)
	}
	if string(got) != want {
		t.Errorf("reader content = %q, want %q", got, want)
	}
}

// encodeUTF16 encodes plain as UTF-16 (with a leading BOM) in the given
// byte order, via golang.org/x/text, so test fixtures are guaranteed
// correct instead of hand-assembled and potentially wrong.
func encodeUTF16(t *testing.T, plain string, bo unicode.Endianness) []byte {
	t.Helper()
	enc := unicode.UTF16(bo, unicode.UseBOM).NewEncoder()
	out, _, err := transform.Bytes(enc, []byte(plain))
	if err != nil {
		t.Fatalf("encodeUTF16: %v", err)
	}
	return out
}
