package output

import (
	"bytes"
	"testing"

	"gg/internal/matcher"
)

func render(opts Options, do func(p *Printer)) string {
	var buf bytes.Buffer
	p := New(&buf, opts)
	do(p)
	if err := p.Flush(); err != nil {
		panic(err)
	}
	return buf.String()
}

func TestPrintLine_Plain(t *testing.T) {
	got := render(Options{}, func(p *Printer) {
		p.PrintLine("file.txt", 1, []byte("hello world"), []matcher.MatchRange{{Start: 6, End: 11}})
	})
	want := "hello world\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintLine_WithFilename(t *testing.T) {
	got := render(Options{ShowFilename: true}, func(p *Printer) {
		p.PrintLine("file.txt", 1, []byte("hello"), nil)
	})
	want := "file.txt:hello\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintLine_WithLineNumber(t *testing.T) {
	got := render(Options{LineNumber: true}, func(p *Printer) {
		p.PrintLine("file.txt", 42, []byte("hello"), nil)
	})
	want := "42:hello\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintLine_WithFilenameAndLineNumber(t *testing.T) {
	got := render(Options{ShowFilename: true, LineNumber: true}, func(p *Printer) {
		p.PrintLine("file.txt", 7, []byte("hello"), nil)
	})
	want := "file.txt:7:hello\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintLine_ColorHighlightsRanges(t *testing.T) {
	got := render(Options{Color: true}, func(p *Printer) {
		p.PrintLine("f", 1, []byte("the cat sat"), []matcher.MatchRange{{Start: 4, End: 7}})
	})
	want := "the " + colorStart + "cat" + colorReset + " sat\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintLine_ColorHighlightsMultipleNonOverlappingRanges(t *testing.T) {
	got := render(Options{Color: true}, func(p *Printer) {
		p.PrintLine("f", 1, []byte("cat and dog"), []matcher.MatchRange{{Start: 0, End: 3}, {Start: 8, End: 11}})
	})
	want := colorStart + "cat" + colorReset + " and " + colorStart + "dog" + colorReset + "\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintLine_NoColorWhenDisabledEvenWithRanges(t *testing.T) {
	got := render(Options{Color: false}, func(p *Printer) {
		p.PrintLine("f", 1, []byte("the cat sat"), []matcher.MatchRange{{Start: 4, End: 7}})
	})
	want := "the cat sat\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintLine_InvertedMatchHasNoRangesToHighlight(t *testing.T) {
	// -v: the line is selected because nothing matched, so ranges is
	// empty — the whole line prints plainly even with color enabled.
	got := render(Options{Color: true}, func(p *Printer) {
		p.PrintLine("f", 1, []byte("no match here"), nil)
	})
	want := "no match here\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintLine_OnlyMatchingSingleRange(t *testing.T) {
	got := render(Options{OnlyMatching: true}, func(p *Printer) {
		p.PrintLine("f", 1, []byte("the cat sat"), []matcher.MatchRange{{Start: 4, End: 7}})
	})
	want := "cat\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintLine_OnlyMatchingMultipleRangesEachOnOwnLine(t *testing.T) {
	got := render(Options{OnlyMatching: true}, func(p *Printer) {
		p.PrintLine("f", 1, []byte("cat and dog"), []matcher.MatchRange{{Start: 0, End: 3}, {Start: 8, End: 11}})
	})
	want := "cat\ndog\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintLine_OnlyMatchingWithFilenameAndLineNumberPerMatch(t *testing.T) {
	got := render(Options{OnlyMatching: true, ShowFilename: true, LineNumber: true}, func(p *Printer) {
		p.PrintLine("f.txt", 3, []byte("cat and dog"), []matcher.MatchRange{{Start: 0, End: 3}, {Start: 8, End: 11}})
	})
	want := "f.txt:3:cat\nf.txt:3:dog\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintLine_OnlyMatchingWithNoRangesPrintsNothing(t *testing.T) {
	// -v combined with -o: the line was selected for not matching, so
	// there is nothing to print under -o's "only the matched part"
	// contract — matches GNU grep's own behavior for this combination.
	got := render(Options{OnlyMatching: true}, func(p *Printer) {
		p.PrintLine("f", 1, []byte("no match here"), nil)
	})
	if got != "" {
		t.Errorf("got %q, want empty output", got)
	}
}

func TestPrintLine_OnlyMatchingWithColor(t *testing.T) {
	got := render(Options{OnlyMatching: true, Color: true}, func(p *Printer) {
		p.PrintLine("f", 1, []byte("the cat sat"), []matcher.MatchRange{{Start: 4, End: 7}})
	})
	want := colorStart + "cat" + colorReset + "\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintCount_WithoutFilename(t *testing.T) {
	got := render(Options{}, func(p *Printer) { p.PrintCount("file.txt", 5) })
	if got != "5\n" {
		t.Errorf("got %q, want %q", got, "5\n")
	}
}

func TestPrintCount_WithFilename(t *testing.T) {
	got := render(Options{ShowFilename: true}, func(p *Printer) { p.PrintCount("file.txt", 5) })
	if got != "file.txt:5\n" {
		t.Errorf("got %q, want %q", got, "file.txt:5\n")
	}
}

func TestPrintFilename_IgnoresShowFilenameSetting(t *testing.T) {
	// -l/-L's entire point is printing the name; ShowFilename=false
	// (as if -h were given) must not suppress it.
	got := render(Options{ShowFilename: false}, func(p *Printer) { p.PrintFilename("file.txt") })
	if got != "file.txt\n" {
		t.Errorf("got %q, want %q", got, "file.txt\n")
	}
}

func TestPrintBinaryMatch(t *testing.T) {
	got := render(Options{}, func(p *Printer) { p.PrintBinaryMatch("data.bin") })
	if got != "Binary file data.bin matches\n" {
		t.Errorf("got %q, want %q", got, "Binary file data.bin matches\n")
	}
}

func TestFlush_IsRequiredForOutputToAppear(t *testing.T) {
	var buf bytes.Buffer
	p := New(&buf, Options{})
	p.PrintLine("f", 1, []byte("hello"), nil)
	if buf.Len() != 0 {
		t.Fatalf("expected buffered output before Flush, got %d bytes already written", buf.Len())
	}
	if err := p.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if buf.String() != "hello\n" {
		t.Errorf("after Flush, got %q, want %q", buf.String(), "hello\n")
	}
}
