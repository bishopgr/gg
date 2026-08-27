package grep

import (
	"context"
	"errors"
	"fmt"
	"io"

	"gg/internal/encoding"
	"gg/internal/lineio"
	"gg/internal/matcher"
	"gg/internal/output"
	"gg/internal/source"
	"gg/internal/walker"
)

// stdinDisplayName is GNU grep's own pseudo-filename for standard input,
// used for messages and, when filenames are shown, the line prefix.
const stdinDisplayName = "(standard input)"

// mode selects which of grep's several mutually-exclusive output shapes
// is active, in the precedence order real grep applies when more than
// one of -q/-l/-L/-c is given.
type mode int

const (
	modeNormal mode = iota
	modeQuiet
	modeListMatching
	modeListNonMatching
	modeCount
)

func selectMode(opts Options) mode {
	switch {
	case opts.Quiet:
		return modeQuiet
	case opts.FilesWithMatches:
		return modeListMatching
	case opts.FilesWithoutMatch:
		return modeListNonMatching
	case opts.Count:
		return modeCount
	default:
		return modeNormal
	}
}

// errStop is an internal sentinel a walker.VisitFunc returns to abort
// the remaining walk early once quiet mode has already found its first
// match — it never escapes engine.searchPath.
var errStop = errors.New("stop: match already found in quiet mode")

// engine holds the state threaded through a single Run call's search.
type engine struct {
	ctx     context.Context
	matcher matcher.Matcher
	opts    Options
	mode    mode
	printer *output.Printer
	stderr  io.Writer

	// context is non-nil only when Options.ContextEnabled is set, and is
	// intentionally shared (not recreated) across every file searched in
	// this run — see contextTracker's doc comment for why that matters.
	context *contextTracker

	hadMatch bool
	hadError bool
	stopped  bool // true once quiet mode's early exit has triggered
}

func (e *engine) searchStdin() {
	rc, err := source.Open("-")
	if err != nil {
		e.reportError(stdinDisplayName, err)
		return
	}
	defer rc.Close()
	if e.context != nil {
		e.context.startFile()
	}
	e.searchReader(stdinDisplayName, rc)
}

func (e *engine) searchPath(root string) {
	walkerOpts := walker.Options{
		Recursive:      e.opts.Recursive,
		FollowSymlinks: e.opts.FollowSymlinks,
		Include:        e.opts.Include,
		Exclude:        e.opts.Exclude,
		ExcludeDir:     e.opts.ExcludeDir,
	}

	err := walker.Walk(root, walkerOpts, func(path string, err error) error {
		if e.ctx.Err() != nil {
			return e.ctx.Err()
		}
		if err != nil {
			e.reportError(path, err)
			return nil // report and continue with the rest of the walk
		}

		e.searchFile(path)
		if e.stopped {
			return errStop
		}
		return nil
	})

	switch {
	case errors.Is(err, errStop):
		// Quiet mode found its match; not a real error.
	case err != nil && e.ctx.Err() != nil:
		e.hadError = true
	case err != nil:
		e.reportError(root, err)
	}
}

func (e *engine) searchFile(path string) {
	rc, err := source.Open(path)
	if err != nil {
		e.reportError(path, err)
		return
	}
	defer rc.Close()
	if e.context != nil {
		e.context.startFile()
	}
	e.searchReader(path, rc)
}

// searchReader runs the actual line-by-line search for one already-open
// stream, applying e.mode's output shape. displayName is what gets
// printed/reported for this stream (a real path, or stdinDisplayName).
func (e *engine) searchReader(displayName string, r io.Reader) {
	sniff, sr := encoding.Sniff(r)
	lr := lineio.NewReader(sr)

	// -I: a binary-detected file is treated as if it produced no
	// matches at all, without reading any further — verified directly
	// against real GNU grep: -c reports 0 for it, -l never lists it,
	// -L always does, and -v makes no observable difference (the file
	// is simply never searched). Skipping the loop entirely below,
	// rather than special-casing each mode's output here, is what makes
	// all of those fall out of the ordinary end-of-function bookkeeping
	// for free, from matched/count staying at their zero values.
	skipBinary := sniff.Binary && e.opts.Binary == BinarySkip
	// -a/--text: treat a binary-detected file exactly like text for
	// matching, context lines, and output — verified directly against
	// real GNU grep, including that -C/-v work normally against it (raw
	// NUL bytes and all).
	asText := !sniff.Binary || e.opts.Binary == BinaryText

	matched := false
	count := 0
	binaryReported := false
	lineNo := 0

	if !skipBinary {
		for {
			if e.ctx.Err() != nil {
				e.hadError = true
				return
			}

			line, err := lr.ReadLine()
			if err != nil {
				if err == io.EOF {
					break
				}
				e.reportError(displayName, err)
				return
			}
			lineNo++

			// rawRanges is what the pattern literally matched on this
			// line, independent of -v. It's kept separate from the
			// (possibly nil'd-out) ranges used for the *selected*
			// line's own highlighting because a line excluded by -v can
			// still surface later as a context line (see the -v+-C
			// interaction test) and must highlight its own literal
			// matches when it does — verified directly against real
			// grep, which does exactly this.
			rawRanges := e.matcher.Find(line.Text)
			isMatch := len(rawRanges) > 0
			ranges := rawRanges
			if e.opts.InvertMatch {
				isMatch = !isMatch
				ranges = nil // nothing to highlight: the line was selected for NOT matching
			}

			if !isMatch {
				if e.context != nil && e.mode == modeNormal && asText {
					if e.context.Skipped(lineNo, line.Text, rawRanges) {
						e.printer.PrintContextLine(displayName, lineNo, line.Text, rawRanges)
					}
				}
				continue
			}

			matched = true
			count++

			switch e.mode {
			case modeQuiet:
				e.hadMatch = true
				e.stopped = true
				return
			case modeListMatching, modeListNonMatching, modeCount:
				// Tally only; the relevant one of these prints once,
				// after the loop, based on the final matched/count
				// values.
			default: // modeNormal
				if !asText {
					if !binaryReported {
						e.printer.PrintBinaryMatch(displayName)
						binaryReported = true
					}
					continue
				}
				if e.context != nil {
					needSeparator, leading := e.context.Selected(lineNo)
					if needSeparator {
						e.printer.PrintContextSeparator()
					}
					for _, l := range leading {
						e.printer.PrintContextLine(displayName, l.lineNo, l.text, l.ranges)
					}
				}
				e.printer.PrintLine(displayName, lineNo, line.Text, ranges)
			}
		}
	}

	// hadMatch (which drives the overall exit code) always tracks raw
	// per-file line-matching, for every mode including -L — verified
	// directly against real GNU grep, not assumed: -l/-L's exit status
	// is identical to plain grep's own (0 iff the pattern matched *some*
	// line in *some* file), completely independent of what -l/-L itself
	// chooses to print. Concretely: `grep -L pat onlyfile` where onlyfile
	// does NOT match still exits 1 ("no match"), even though -L prints
	// onlyfile's name; conversely `grep -L pat a b` where only `a`
	// matches (so -L prints only `b`) exits 0, because *something*
	// matched, even though the file -L printed did not.
	//
	// An earlier version of this code got this backwards for -L
	// specifically (`if !matched` instead of `if matched`), reasoning
	// that -L's own "selection criterion" (which files get printed) and
	// its exit-code criterion must be the same thing — they are not.
	// That version was "confirmed" only by checking gg's own output
	// looked plausible for a single hand-picked scenario (every file
	// matching, so -L prints nothing) without cross-checking against
	// real grep for that exact case, which — as it turns out — also
	// exits 0 there (since every file *did* match), not the 1 that
	// version asserted. See run_test.go's TestRun_FilesWithoutMatch_*
	// tests for the full oracle-verified matrix this was corrected
	// against.
	switch e.mode {
	case modeListMatching:
		if matched {
			e.hadMatch = true
			e.printer.PrintFilename(displayName)
		}
	case modeListNonMatching:
		// Printing criterion (list files that did NOT match) is
		// deliberately independent of the exit-code criterion above.
		if matched {
			e.hadMatch = true
		} else {
			e.printer.PrintFilename(displayName)
		}
	case modeCount:
		if matched {
			e.hadMatch = true
		}
		e.printer.PrintCount(displayName, count)
	default: // modeNormal (modeQuiet already returned earlier in the loop)
		if matched {
			e.hadMatch = true
		}
	}
}

func (e *engine) reportError(path string, err error) {
	e.hadError = true
	fmt.Fprintf(e.stderr, "gg: %s: %s\n", path, describeError(err))
}
