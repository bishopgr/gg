// Package cli wires Cobra commands to the grep engine. This package owns
// flag parsing and process-facing concerns (usage text, --version)
// only; all search logic lives in internal/grep.
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"gg/internal/buildinfo"
	"gg/internal/grep"
)

// unsetExitCode marks that RunE never ran (e.g. Cobra handled --version,
// --help, or a usage error before reaching it), so Execute must pick the
// right fallback based on whether an error occurred.
const unsetExitCode = -1

// runGrep is the seam RunE calls through. It's a package-level var
// (rather than grep.Run called directly) purely so tests can verify the
// exit-code plumbing — including PROCESS.md §3.5's "must be 0/1/2
// regardless of OS, and Cobra must not swallow that distinction"
// requirement — for every code, without needing a real search engine
// wired up yet. Production code never reassigns it.
var runGrep = grep.Run

// Execute builds and runs the root command for the given args (typically
// os.Args[1:]) and returns the process exit code to use. Only main.go
// calls os.Exit; no command logic below this does.
//
// ctx is threaded through to grep.Run via Cobra's ExecuteContext/
// cmd.Context() so Ctrl+C (PROCESS.md §3.5) can eventually trigger a
// graceful stop once there's real work to cancel — see main.go, which
// derives ctx from signal.NotifyContext.
func Execute(ctx context.Context, args []string) int {
	exitCode := unsetExitCode
	root := newRootCmd(&exitCode)
	root.SetArgs(args)

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "gg:", err)
		if exitCode == unsetExitCode {
			return grep.ExitError // Cobra-level usage error; RunE never ran
		}
		return exitCode
	}
	if exitCode == unsetExitCode {
		return grep.ExitMatch // e.g. --version/--help handled without RunE
	}
	return exitCode
}

// flags holds the destinations pflag writes into; RunE reads them into
// a grep.Options. Grouping them in one struct (rather than a pile of
// separate local vars in newRootCmd) keeps the flag-registration function
// and the "turn flags into Options" translation each readable on their own.
type flags struct {
	ignoreCase   bool
	fixedStrings bool
	invertMatch  bool
	wordRegexp   bool
	lineRegexp   bool

	lineNumber        bool
	onlyMatching      bool
	count             bool
	filesWithMatches  bool
	filesWithoutMatch bool
	quiet             bool
	noFilename        bool
	withFilename      bool
	color             string

	recursive            bool
	dereferenceRecursive bool
	include              []string
	exclude              []string
	excludeDir           []string
}

func newRootCmd(exitCode *int) *cobra.Command {
	var f flags

	cmd := &cobra.Command{
		Use:   "gg [OPTION]... PATTERN [FILE...]",
		Short: "gg is a cross-platform grep implementation",
		Long: "gg searches FILE(s) for lines matching PATTERN and prints them.\n" +
			"When no FILE is given, or FILE is \"-\", gg reads standard input.",
		Version:       buildinfo.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			colorMode, err := grep.ParseColorMode(f.color)
			if err != nil {
				return err
			}

			opts := grep.Options{
				Pattern: args[0],
				Files:   args[1:],

				Fixed:       f.fixedStrings,
				IgnoreCase:  f.ignoreCase,
				InvertMatch: f.invertMatch,
				WordRegexp:  f.wordRegexp,
				LineRegexp:  f.lineRegexp,

				LineNumber:        f.lineNumber,
				OnlyMatching:      f.onlyMatching,
				Count:             f.count,
				FilesWithMatches:  f.filesWithMatches,
				FilesWithoutMatch: f.filesWithoutMatch,
				Quiet:             f.quiet,
				NoFilename:        f.noFilename,
				WithFilename:      f.withFilename,
				Color:             colorMode,

				Recursive:      f.recursive || f.dereferenceRecursive,
				FollowSymlinks: f.dereferenceRecursive,
				Include:        f.include,
				Exclude:        f.exclude,
				ExcludeDir:     f.excludeDir,
			}

			code, err := runGrep(cmd.Context(), opts)
			*exitCode = code
			return err
		},
	}

	registerFlags(cmd, &f)

	cmd.SetVersionTemplate("gg version {{.Version}}\n")

	return cmd
}

// registerFlags defines every flag and, deliberately, does so *before*
// Cobra lazily adds its default --help/-h and --version/-v flags inside
// Execute(). The goal — matching GNU grep's own convention, where -h
// means "no filename" and -v means "invert match," and --help/--version
// are long-flag-only — needs two different workarounds for two
// asymmetric Cobra behaviors (verified against cobra v1.10.2's source,
// not assumed):
//
//   - InitDefaultVersionFlag checks ShorthandLookup("v") and falls back
//     to long-only automatically if it's taken — registering our own -v
//     here is enough, no extra step needed.
//   - InitDefaultHelpFlag has no such check: it unconditionally tries to
//     register -h and *panics* ("unable to redefine 'h' shorthand") if
//     that shorthand is already in use. Its actual gate is
//     `Flags().Lookup("help") == nil` — checked by flag *name*, not
//     shorthand — so pre-registering a flag literally named "help"
//     ourselves (long-only, below) makes Cobra skip its own
//     registration entirely. Cobra's help-triggering logic
//     (command.go's execute()) reads the flag back by that same name,
//     so --help keeps working normally; only the never-requested -h
//     shorthand is gone, freed up for --no-filename.
func registerFlags(cmd *cobra.Command, f *flags) {
	flagSet := cmd.Flags()

	flagSet.Bool("help", false, "help for gg")

	flagSet.BoolVarP(&f.ignoreCase, "ignore-case", "i", false, "ignore case distinctions in PATTERN")
	flagSet.BoolVarP(&f.fixedStrings, "fixed-strings", "F", false, "treat PATTERN as a literal string, not a regular expression")
	flagSet.BoolVarP(&f.invertMatch, "invert-match", "v", false, "select non-matching lines")
	flagSet.BoolVarP(&f.wordRegexp, "word-regexp", "w", false, "match only whole words")
	flagSet.BoolVarP(&f.lineRegexp, "line-regexp", "x", false, "match only whole lines")

	flagSet.BoolVarP(&f.lineNumber, "line-number", "n", false, "prefix each printed line with its line number")
	flagSet.BoolVarP(&f.onlyMatching, "only-matching", "o", false, "print only the matched part of each line")
	flagSet.BoolVarP(&f.count, "count", "c", false, "print only a count of matching lines per file")
	flagSet.BoolVarP(&f.filesWithMatches, "files-with-matches", "l", false, "print only names of files containing a match")
	flagSet.BoolVarP(&f.filesWithoutMatch, "files-without-match", "L", false, "print only names of files containing no match")
	flagSet.BoolVarP(&f.quiet, "quiet", "q", false, "suppress all normal output; exit status only")
	flagSet.BoolVarP(&f.noFilename, "no-filename", "h", false, "never print filenames with matches")
	flagSet.BoolVarP(&f.withFilename, "with-filename", "H", false, "always print filenames with matches")

	flagSet.StringVar(&f.color, "color", "auto", "highlight matches: 'always', 'never', or 'auto' (default); bare --color means 'always'")
	flagSet.Lookup("color").NoOptDefVal = "always" // GNU grep convention: --color with no value means always

	flagSet.BoolVarP(&f.recursive, "recursive", "r", false, "search directories recursively (does not follow symlinks)")
	flagSet.BoolVarP(&f.dereferenceRecursive, "dereference-recursive", "R", false, "like -r, but follow all symlinks")
	flagSet.StringArrayVar(&f.include, "include", nil, "search only files matching GLOB (may be repeated)")
	flagSet.StringArrayVar(&f.exclude, "exclude", nil, "skip files matching GLOB (may be repeated)")
	flagSet.StringArrayVar(&f.excludeDir, "exclude-dir", nil, "skip directories matching GLOB (may be repeated)")

	cmd.MarkFlagsMutuallyExclusive("files-with-matches", "files-without-match")
	cmd.MarkFlagsMutuallyExclusive("no-filename", "with-filename")
}
