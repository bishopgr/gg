// Package output formats matched lines for printing. It is pure
// formatting policy, the same "report/apply facts, don't decide them"
// contract as every other package here: it does not decide whether a
// line matched (internal/matcher), which files to search
// (internal/walker), or whether color *should* be used (that's for
// internal/grep, the composition root, to resolve from a --color flag
// value plus internal/termenv's terminal detection).
package output

import (
	"bufio"
	"fmt"
	"io"

	"gg/internal/matcher"
)

// ANSI SGR codes for match highlighting: bold red, matching GNU grep's
// own default GREP_COLORS ("ms=01;31"). Deliberately not configurable
// (no GREP_COLORS-equivalent) for v1 — a fixed, sensible default beats
// building a color-configuration format nobody asked for yet.
const (
	colorStart = "\x1b[1;31m"
	colorReset = "\x1b[0m"
)

// Options controls formatting. The zero value is a valid, minimal
// configuration: no filename/line-number prefix, full lines, no color.
type Options struct {
	// ShowFilename prefixes each printed line with "filename:".
	ShowFilename bool
	// LineNumber prefixes each printed line with "N:" (-n).
	LineNumber bool
	// OnlyMatching prints just the matched substring(s), one per output
	// line, instead of the whole source line (-o).
	OnlyMatching bool
	// Color highlights matched substrings using ANSI escape codes.
	// Resolving *whether* to enable this from --color/terminal state is
	// the caller's job (internal/grep) — see the package doc.
	Color bool
}

// Printer writes formatted match output to an underlying writer. All
// writes are buffered; call Flush before the process exits (a deferred
// Flush right after New covers every return path, including an early
// quiet-mode exit).
type Printer struct {
	w    *bufio.Writer
	opts Options
}

// New returns a Printer that writes to w according to opts.
func New(w io.Writer, opts Options) *Printer {
	return &Printer{w: bufio.NewWriter(w), opts: opts}
}

// Flush writes any buffered output to the underlying writer. It must be
// called before the process exits or output can be silently lost.
func (p *Printer) Flush() error {
	return p.w.Flush()
}

// PrintLine writes one selected line's output: optional filename/line-
// number prefix, then either the whole line (with ranges highlighted,
// if enabled) or — under OnlyMatching — just the matched substring(s),
// one per output line. ranges is empty for an inverted (-v) match,
// where the line was selected precisely because nothing matched; under
// OnlyMatching that means nothing is printed for this line at all,
// mirroring GNU grep's own behavior for -v -o.
//
// Every printed line ends with a bare '\n', regardless of platform or
// the source line's own terminator (PROCESS.md §3.1 — decided ahead of
// this package existing, applied here).
func (p *Printer) PrintLine(filename string, lineNo int, text []byte, ranges []matcher.MatchRange) {
	if p.opts.OnlyMatching {
		for _, r := range ranges {
			p.writePrefix(filename, lineNo)
			p.writeMatch(text[r.Start:r.End])
			p.w.WriteByte('\n')
		}
		return
	}

	p.writePrefix(filename, lineNo)
	p.writeHighlighted(text, ranges)
	p.w.WriteByte('\n')
}

// PrintCount writes a per-file match count (-c): "filename:N\n" if
// filenames are being shown, otherwise just "N\n".
func (p *Printer) PrintCount(filename string, count int) {
	if p.opts.ShowFilename {
		fmt.Fprintf(p.w, "%s:%d\n", filename, count)
		return
	}
	fmt.Fprintf(p.w, "%d\n", count)
}

// PrintFilename writes filename alone (-l/-L): the entire point of
// those modes is printing the name, so this ignores ShowFilename/-h —
// suppressing it would make -l useless.
func (p *Printer) PrintFilename(filename string) {
	fmt.Fprintf(p.w, "%s\n", filename)
}

// PrintBinaryMatch writes GNU grep's conventional
// "Binary file FILENAME matches" message, used in place of printing
// individual lines when a file is detected binary and still matched.
func (p *Printer) PrintBinaryMatch(filename string) {
	fmt.Fprintf(p.w, "Binary file %s matches\n", filename)
}

func (p *Printer) writePrefix(filename string, lineNo int) {
	if p.opts.ShowFilename {
		p.w.WriteString(filename)
		p.w.WriteByte(':')
	}
	if p.opts.LineNumber {
		fmt.Fprintf(p.w, "%d:", lineNo)
	}
}

// writeHighlighted writes text with each range in ranges wrapped in
// color codes when enabled. ranges must be ascending and non-overlapping
// (matcher.Matcher's documented contract) — writeHighlighted trusts it
// rather than re-validating on every line for performance.
func (p *Printer) writeHighlighted(text []byte, ranges []matcher.MatchRange) {
	if !p.opts.Color || len(ranges) == 0 {
		p.w.Write(text)
		return
	}

	pos := 0
	for _, r := range ranges {
		p.w.Write(text[pos:r.Start])
		p.writeMatch(text[r.Start:r.End])
		pos = r.End
	}
	p.w.Write(text[pos:])
}

func (p *Printer) writeMatch(matched []byte) {
	if !p.opts.Color {
		p.w.Write(matched)
		return
	}
	p.w.WriteString(colorStart)
	p.w.Write(matched)
	p.w.WriteString(colorReset)
}
