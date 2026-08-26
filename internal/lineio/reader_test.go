package lineio

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadLine_Table(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Line
	}{
		{
			"empty input produces no lines",
			"",
			nil,
		},
		{
			"single LF-terminated line",
			"a\n",
			[]Line{{Text: []byte("a"), Terminator: LF}},
		},
		{
			"LF, no trailing newline",
			"a\nb",
			[]Line{
				{Text: []byte("a"), Terminator: LF},
				{Text: []byte("b"), Terminator: NoTerminator},
			},
		},
		{
			"CRLF-terminated lines",
			"a\r\nb\r\n",
			[]Line{
				{Text: []byte("a"), Terminator: CRLF},
				{Text: []byte("b"), Terminator: CRLF},
			},
		},
		{
			"CRLF, no trailing newline",
			"a\r\nb",
			[]Line{
				{Text: []byte("a"), Terminator: CRLF},
				{Text: []byte("b"), Terminator: NoTerminator},
			},
		},
		{
			"bare CR (classic Mac) terminated lines",
			"a\rb\rc\r",
			[]Line{
				{Text: []byte("a"), Terminator: CR},
				{Text: []byte("b"), Terminator: CR},
				{Text: []byte("c"), Terminator: CR},
			},
		},
		{
			"bare CR, no trailing terminator",
			"a\rb",
			[]Line{
				{Text: []byte("a"), Terminator: CR},
				{Text: []byte("b"), Terminator: NoTerminator},
			},
		},
		{
			"mixed terminators in one stream",
			"unix\nwindows\r\nmac\rlast",
			[]Line{
				{Text: []byte("unix"), Terminator: LF},
				{Text: []byte("windows"), Terminator: CRLF},
				{Text: []byte("mac"), Terminator: CR},
				{Text: []byte("last"), Terminator: NoTerminator},
			},
		},
		{
			"consecutive blank lines (LF)",
			"\n\n",
			[]Line{
				{Text: []byte(""), Terminator: LF},
				{Text: []byte(""), Terminator: LF},
			},
		},
		{
			"input that is only a terminator",
			"\r\n",
			[]Line{{Text: []byte(""), Terminator: CRLF}},
		},
		{
			"bare CR immediately followed by another bare CR",
			"\r\r",
			[]Line{
				{Text: []byte(""), Terminator: CR},
				{Text: []byte(""), Terminator: CR},
			},
		},
		{
			"bare CR followed by a CRLF-terminated empty line",
			"\r\r\n",
			[]Line{
				{Text: []byte(""), Terminator: CR},
				{Text: []byte(""), Terminator: CRLF},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readAll(t, strings.NewReader(tt.input))
			assertLinesEqual(t, got, tt.want)
		})
	}
}

// TestReadLine_ArbitrarilyLongLine proves this reader has no maximum line
// length, unlike bufio.Scanner's default 64KiB token cap — a single line
// well beyond the internal bufio buffer size (64KiB, see NewReader) must
// still come back intact and correctly terminated.
func TestReadLine_ArbitrarilyLongLine(t *testing.T) {
	long := strings.Repeat("x", 5*64*1024+37) // > 5x the internal buffer
	input := long + "\n" + "second"

	got := readAll(t, strings.NewReader(input))
	if len(got) != 2 {
		t.Fatalf("got %d lines, want 2", len(got))
	}
	if string(got[0].Text) != long {
		t.Fatalf("first line length = %d, want %d (content mismatch)", len(got[0].Text), len(long))
	}
	if got[0].Terminator != LF {
		t.Fatalf("first line terminator = %v, want LF", got[0].Terminator)
	}
	if string(got[1].Text) != "second" || got[1].Terminator != NoTerminator {
		t.Fatalf("second line = %+v, want {second NoTerminator}", got[1])
	}
}

// TestReadLine_TextCopiedBeforeNextCall guards the documented aliasing
// contract: Line.Text is only valid until the next ReadLine call, so
// callers that need to keep it must copy — verify a copy survives.
func TestReadLine_TextCopiedBeforeNextCall(t *testing.T) {
	r := NewReader(strings.NewReader("first\nsecond\n"))

	l1, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() #1 error: %v", err)
	}
	first := append([]byte(nil), l1.Text...) // caller-side copy

	if _, err := r.ReadLine(); err != nil {
		t.Fatalf("ReadLine() #2 error: %v", err)
	}

	if string(first) != "first" {
		t.Fatalf("copied first line = %q, want %q (aliasing leaked across calls)", first, "first")
	}
}

// TestNewReader_ReusesAlreadyBufferedReader pins down the
// double-buffering fix from PROCESS.md §4 step 8: when the input is
// already a *bufio.Reader with at least DefaultBufSize of capacity
// (the shape internal/encoding.Sniff hands off), NewReader must reuse
// it directly rather than wrapping it in a second bufio.Reader — a
// white-box check (same package) since the wrapping decision has no
// externally-observable side effect other than performance.
func TestNewReader_ReusesAlreadyBufferedReader(t *testing.T) {
	br := bufio.NewReaderSize(strings.NewReader("a\nb\n"), DefaultBufSize)
	r := NewReader(br)
	if r.br != br {
		t.Fatal("NewReader wrapped an already-adequately-buffered *bufio.Reader instead of reusing it")
	}
}

// TestNewReader_WrapsUndersizedBufferedReader is
// TestNewReader_ReusesAlreadyBufferedReader's negative case: a
// *bufio.Reader smaller than DefaultBufSize (e.g. internal/encoding's
// own 8000-byte sniff buffer, before it was sized up to match) must
// still get wrapped, not reused as-is at a throughput-limiting size.
func TestNewReader_WrapsUndersizedBufferedReader(t *testing.T) {
	br := bufio.NewReaderSize(strings.NewReader("a\nb\n"), DefaultBufSize/2)
	r := NewReader(br)
	if r.br == br {
		t.Fatal("NewReader reused an undersized *bufio.Reader instead of wrapping it")
	}
}

func TestReader_Reset(t *testing.T) {
	r := NewReader(strings.NewReader("from-first\n"))
	got1 := readAllReuse(t, r)
	assertLinesEqual(t, got1, []Line{{Text: []byte("from-first"), Terminator: LF}})

	r.Reset(strings.NewReader("from-second\n"))
	got2 := readAllReuse(t, r)
	assertLinesEqual(t, got2, []Line{{Text: []byte("from-second"), Terminator: LF}})
}

// TestReader_CRLFAndLFFixturesAgreeOnContent is the concrete instance of
// the PROCESS.md §3.1 requirement: files that differ only in line-ending
// convention must yield identical line *content* once split. This is the
// property that actually matters for matching correctness — a pattern
// like "^line two$" must behave identically regardless of which fixture
// it runs against.
func TestReader_CRLFAndLFFixturesAgreeOnContent(t *testing.T) {
	crlfLines := readAllFile(t, "crlf.txt")
	lfLines := readAllFile(t, "lf.txt")

	if len(crlfLines) != len(lfLines) {
		t.Fatalf("crlf.txt has %d lines, lf.txt has %d", len(crlfLines), len(lfLines))
	}
	for i := range crlfLines {
		if string(crlfLines[i].Text) != string(lfLines[i].Text) {
			t.Errorf("line %d content differs: crlf.txt=%q lf.txt=%q",
				i, crlfLines[i].Text, lfLines[i].Text)
		}
		if crlfLines[i].Terminator != CRLF {
			t.Errorf("crlf.txt line %d terminator = %v, want CRLF", i, crlfLines[i].Terminator)
		}
		if lfLines[i].Terminator != LF {
			t.Errorf("lf.txt line %d terminator = %v, want LF", i, lfLines[i].Terminator)
		}
	}
}

func TestReader_NoTrailingNewlineFixture(t *testing.T) {
	lines := readAllFile(t, "no-trailing-newline.txt")
	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}
	last := lines[len(lines)-1]
	if last.Terminator != NoTerminator {
		t.Errorf("last line terminator = %v, want NoTerminator", last.Terminator)
	}
}

func TestReader_EmptyFixture(t *testing.T) {
	lines := readAllFile(t, "empty.txt")
	if len(lines) != 0 {
		t.Errorf("got %d lines from an empty file, want 0", len(lines))
	}
}

func TestReader_CRFixture(t *testing.T) {
	lines := readAllFile(t, "cr.txt")
	want := []Line{
		{Text: []byte("line one"), Terminator: CR},
		{Text: []byte("line two"), Terminator: CR},
		{Text: []byte("line three"), Terminator: CR},
	}
	assertLinesEqual(t, lines, want)
}

// --- test helpers -----------------------------------------------------

func readAll(t *testing.T, r io.Reader) []Line {
	t.Helper()
	return readAllReuse(t, NewReader(r))
}

// readAllReuse drains rd, copying each Line.Text since it aliases rd's
// internal buffer and would otherwise be invalidated by the next call.
func readAllReuse(t *testing.T, rd *Reader) []Line {
	t.Helper()
	var out []Line
	for {
		line, err := rd.ReadLine()
		if err != nil {
			if err == io.EOF {
				return out
			}
			t.Fatalf("ReadLine() error: %v", err)
		}
		out = append(out, Line{
			Text:       append([]byte(nil), line.Text...),
			Terminator: line.Terminator,
		})
	}
}

func readAllFile(t *testing.T, name string) []Line {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "test", "testdata", name))
	if err != nil {
		t.Fatalf("open fixture %s: %v", name, err)
	}
	defer f.Close()
	return readAll(t, f)
}

func assertLinesEqual(t *testing.T, got, want []Line) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d\n got: %+v\nwant: %+v", len(got), len(want), got, want)
	}
	for i := range got {
		if string(got[i].Text) != string(want[i].Text) || got[i].Terminator != want[i].Terminator {
			t.Errorf("line %d = %+v (text %q), want %+v (text %q)",
				i, got[i], got[i].Text, want[i], want[i].Text)
		}
	}
}
