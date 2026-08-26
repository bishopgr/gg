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
	e.searchReader(path, rc)
}

// searchReader runs the actual line-by-line search for one already-open
// stream, applying e.mode's output shape. displayName is what gets
// printed/reported for this stream (a real path, or stdinDisplayName).
func (e *engine) searchReader(displayName string, r io.Reader) {
	sniff, sr := encoding.Sniff(r)
	lr := lineio.NewReader(sr)

	matched := false
	count := 0
	binaryReported := false
	lineNo := 0

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

		ranges := e.matcher.Find(line.Text)
		isMatch := len(ranges) > 0
		if e.opts.InvertMatch {
			isMatch = !isMatch
			ranges = nil // nothing to highlight: the line was selected for NOT matching
		}
		if !isMatch {
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
			// Tally only; the relevant one of these prints once, after
			// the loop, based on the final matched/count values.
		default: // modeNormal
			if sniff.Binary {
				if !binaryReported {
					e.printer.PrintBinaryMatch(displayName)
					binaryReported = true
				}
				continue
			}
			e.printer.PrintLine(displayName, lineNo, line.Text, ranges)
		}
	}

	// hadMatch (which drives the overall exit code) must track each
	// mode's own "selected something" criterion, not raw line-matching
	// unconditionally — -L's whole point is selecting files that did
	// NOT match, so its success condition is !matched, not matched.
	// (Found via manual exercising, not from first-principles design:
	// -L on a directory where every file matched was printing nothing,
	// correctly, but still exiting 0 as if it had — see OUTPUT.md.)
	switch e.mode {
	case modeListMatching:
		if matched {
			e.hadMatch = true
			e.printer.PrintFilename(displayName)
		}
	case modeListNonMatching:
		if !matched {
			e.hadMatch = true
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
