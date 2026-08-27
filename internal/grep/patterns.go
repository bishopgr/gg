package grep

import (
	"fmt"
	"io"

	"gg/internal/lineio"
	"gg/internal/source"
)

// resolvePatterns combines literal patterns (-e, or the positional
// PATTERN when neither -e nor -f was given) with patterns read from
// pattern files (-f), in that order: every literal pattern first, then
// every -f file's lines, in the order the files were given and the
// order lines appear within each file.
//
// This ordering is a deliberately simple, documented convention rather
// than an attempt at exact command-line interleaving between -e and -f
// occurrences: regex alternation is order-independent for *whether* a
// line matches, so there is no observable-behavior reason (and real GNU
// grep gives no stronger guarantee either) to do more here.
//
// Each line of a pattern file becomes its own pattern verbatim,
// including a blank line — which becomes a literal empty pattern that
// matches every line, exactly like `-e ""` does. Verified directly
// against real GNU grep, not assumed: blank lines are not skipped.
//
// A pattern file that can't be opened or read is a fatal, whole-command
// error (like an invalid regex), not a per-search-file diagnostic: with
// no readable pattern list, there is nothing meaningful left to search
// with.
func resolvePatterns(literal []string, files []string) ([]string, error) {
	if len(files) == 0 {
		return literal, nil
	}

	patterns := append([]string{}, literal...)
	for _, path := range files {
		filePatterns, err := readPatternFile(path)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, filePatterns...)
	}
	return patterns, nil
}

// readPatternFile reads path (or, for "-", standard input — the same
// convention every FILE argument already follows) one line at a time
// via internal/lineio, so pattern files get the exact same CRLF/LF/CR
// and no-trailing-newline correctness as files being searched, rather
// than a second, less careful line-splitting implementation.
func readPatternFile(path string) ([]string, error) {
	displayName := path
	if path == "-" {
		displayName = stdinDisplayName
	}

	rc, err := source.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", displayName, describeError(err))
	}
	defer rc.Close()

	lr := lineio.NewReader(rc)
	var patterns []string
	for {
		line, err := lr.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("%s: %s", displayName, describeError(err))
		}
		patterns = append(patterns, string(line.Text))
	}
	return patterns, nil
}
