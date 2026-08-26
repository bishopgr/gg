package grep

import "gg/internal/matcher"

// contextLine is one buffered "before" candidate line, already copied
// out of lineio's reused read buffer. lineio.Line.Text "is only valid
// until the next call to ReadLine" (see internal/lineio's doc comment),
// so anything held across multiple ReadLine calls — like these —  must
// be copied, not aliased.
type contextLine struct {
	lineNo int
	text   []byte
	ranges []matcher.MatchRange
}

// contextTracker implements GNU grep's own streaming -A/-B/-C algorithm:
// buffer up to `before` of the most recent non-selected lines, and once
// a line is selected (a match, or — under -v — a non-match), emit
// however many of those buffered lines are needed to bridge back to
// whatever was already printed, then the selected line itself, then
// keep emitting up to `after` further lines as trailing context
// regardless of whether they themselves get selected.
//
// Every behavioral rule encoded here (the exact separator/merge
// conditions, cross-file separator behavior, -o's "keep the separator
// but drop the context text" split) was verified directly against real
// GNU grep (Git for Windows' grep.exe) rather than assumed from the
// man page — see the PROCESS.md/README commit introducing this file for
// the transcript.
//
// A single contextTracker is reused across every root a search.go loop
// visits (see engine.startFile), not recreated per file: "already
// printed something in a previous file, so the next file's first group
// still needs a separator" is real, observed GNU grep behavior, and
// falls out naturally from everOutput never being reset, rather than
// needing dedicated cross-file bookkeeping.
type contextTracker struct {
	before int
	after  int

	buf          []contextLine // ring buffer, oldest-first, len() capped at `before`
	pendingAfter int           // remaining trailing-context lines still owed
	lastLine     int           // last line number output (real or, under -o, "virtual") in the *current* file; 0 = nothing yet this file
	everOutput   bool          // true once anything has ever been output across the whole run — drives the cross-file separator; deliberately never reset by startFile
}

// newContextTracker returns a tracker for the given before/after window
// sizes. Both may be zero — see Options.ContextEnabled's doc comment for
// why "enabled with 0/0" (separators only) is still a distinct, valid
// configuration from "disabled entirely".
func newContextTracker(before, after int) *contextTracker {
	return &contextTracker{before: before, after: after}
}

// startFile resets the per-file state (buffered lines, the trailing-
// context countdown, and the last-output-line marker) ahead of
// searching a new file. everOutput is deliberately NOT reset — it's the
// one piece of state that must survive across files for the cross-file
// separator behavior described on contextTracker.
func (c *contextTracker) startFile() {
	c.buf = c.buf[:0]
	c.pendingAfter = 0
	c.lastLine = 0
}

// Selected reports what must be printed immediately before a selected
// (matching, or, under -v, non-matching) line at lineNo: whether a "--"
// group separator is needed first, followed by whichever previously-
// buffered lines still need to be emitted as leading context. It always
// also resets the trailing-context countdown for the group this line
// starts.
func (c *contextTracker) Selected(lineNo int) (needSeparator bool, leading []contextLine) {
	// groupStart is how far back this selected line's own leading
	// context window reaches. If that reaches into (or immediately
	// after) whatever was already output for this file, the two groups
	// touch or overlap and must merge with no separator — matching real
	// grep's behavior for e.g. -C2 with matches 4 lines apart.
	groupStart := lineNo - c.before
	contiguous := c.lastLine != 0 && groupStart <= c.lastLine+1

	if contiguous {
		// Only the buffered lines newer than what's already been
		// output are new; the rest were already printed as a previous
		// group's trailing context.
		for _, l := range c.buf {
			if l.lineNo > c.lastLine {
				leading = append(leading, l)
			}
		}
	} else {
		needSeparator = c.everOutput
		// Not contiguous with anything already output — either a real
		// gap, or the first group in this file (or the whole run). The
		// entire buffer (capped at `before` entries by push) becomes
		// leading context.
		leading = append(leading, c.buf...)
	}

	c.buf = c.buf[:0]
	c.pendingAfter = c.after
	c.lastLine = lineNo
	c.everOutput = true
	return needSeparator, leading
}

// Skipped records one non-selected line at lineNo. It reports whether
// the line falls inside a still-open trailing-context window from a
// prior Selected call (in which case the caller should print it as
// context); otherwise the line is buffered as a leading-context
// candidate for a future Selected call, subject to the `before` cap.
func (c *contextTracker) Skipped(lineNo int, text []byte, ranges []matcher.MatchRange) (asContext bool) {
	if c.pendingAfter > 0 {
		c.pendingAfter--
		c.lastLine = lineNo
		c.everOutput = true
		return true
	}
	if c.before > 0 {
		c.buf = append(c.buf, contextLine{
			lineNo: lineNo,
			text:   append([]byte(nil), text...),
			ranges: ranges,
		})
		if len(c.buf) > c.before {
			c.buf = c.buf[1:]
		}
	}
	return false
}
