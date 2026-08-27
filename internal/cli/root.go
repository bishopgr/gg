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

	afterContext  int // -A
	beforeContext int // -B
	context       int // -C

	patternExprs []string // -e
	patternFiles []string // -f

	textBinary bool // -a / --text
	skipBinary bool // -I
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
		// A bare PATTERN positional argument is required ONLY when
		// neither -e nor -f was given — once either is present, every
		// positional argument is a FILE instead (verified directly
		// against real GNU grep: `grep -e pat somefile` searches
		// somefile, it does not also treat somefile as a second
		// pattern), so zero positional args is valid in that case
		// (search stdin, same as today's plain `gg PATTERN` with no
		// FILE). Checking cmd.Flags().Changed here — rather than a
		// static cobra.MinimumNArgs(1) — is what makes that
		// conditional requirement possible at all.
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("regexp") || cmd.Flags().Changed("file") {
				return nil
			}
			return cobra.MinimumNArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			colorMode, err := grep.ParseColorMode(f.color)
			if err != nil {
				return err
			}

			// Once -e/-f is used at all, PATTERN is no longer read from
			// the first positional argument — every positional argument
			// is a FILE instead, even one that "looks like" it was
			// meant as a pattern. Verified directly against real GNU
			// grep, not assumed; see the Args func above for the other
			// half of this.
			var patterns, files []string
			if cmd.Flags().Changed("regexp") || cmd.Flags().Changed("file") {
				patterns = f.patternExprs
				files = args
			} else {
				patterns = []string{args[0]}
				files = args[1:]
			}

			// -A/-B/-C precedence, verified directly against real GNU
			// grep rather than assumed: an explicitly-given -A or -B
			// always wins for its own direction, order-independent —
			// e.g. "-A 1 -C 3" and "-C 3 -A 1" both resolve to after=1,
			// before=3, not "last flag on the command line wins" the
			// way most flag libraries (including pflag) default to for
			// repeated/overlapping flags. Only the direction(s) not
			// explicitly given fall back to -C's value.
			after := f.afterContext
			if !cmd.Flags().Changed("after-context") {
				after = f.context
			}
			before := f.beforeContext
			if !cmd.Flags().Changed("before-context") {
				before = f.context
			}
			if after < 0 || before < 0 {
				return fmt.Errorf("invalid context length argument")
			}
			// GNU grep enables "--" group separators between
			// non-adjacent matches as soon as -A/-B/-C is given at all
			// — even -A0/-C0, which print zero actual context lines but
			// still separate non-contiguous matches. Verified directly:
			// this is genuinely different output from not passing any
			// context flag, not just an equivalent 0-value default.
			contextEnabled := cmd.Flags().Changed("after-context") ||
				cmd.Flags().Changed("before-context") ||
				cmd.Flags().Changed("context")

			// -a/-I precedence: unlike -A/-B/-C, this is a plain,
			// documented "the more restrictive flag wins if both are
			// given" rule rather than an attempt at exact command-line-
			// order last-one-wins semantics (verified only informally
			// against real GNU grep, which does track flag order here —
			// but reproducing that precisely would need inspecting raw
			// argument order ourselves, for a combination unlikely to
			// ever be given deliberately). -I is checked last so it
			// wins over -a when both are present.
			binaryMode := grep.BinaryDefault
			if f.textBinary {
				binaryMode = grep.BinaryText
			}
			if f.skipBinary {
				binaryMode = grep.BinarySkip
			}

			opts := grep.Options{
				Patterns:     patterns,
				PatternFiles: f.patternFiles,
				Files:        files,

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
				Binary:            binaryMode,

				Recursive:      f.recursive || f.dereferenceRecursive,
				FollowSymlinks: f.dereferenceRecursive,
				Include:        f.include,
				Exclude:        f.exclude,
				ExcludeDir:     f.excludeDir,

				ContextEnabled: contextEnabled,
				ContextBefore:  before,
				ContextAfter:   after,
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

	flagSet.IntVarP(&f.afterContext, "after-context", "A", 0, "print NUM lines of trailing context after matching lines")
	flagSet.IntVarP(&f.beforeContext, "before-context", "B", 0, "print NUM lines of leading context before matching lines")
	flagSet.IntVarP(&f.context, "context", "C", 0, "print NUM lines of leading and trailing context (like -A NUM -B NUM, unless -A/-B is also given)")

	flagSet.StringArrayVarP(&f.patternExprs, "regexp", "e", nil, "use PATTERN as a matching pattern (may be repeated); once given, every positional argument is a FILE instead")
	flagSet.StringArrayVarP(&f.patternFiles, "file", "f", nil, "read patterns from FILE, one per line (may be repeated); \"-\" means stdin")

	flagSet.BoolVarP(&f.textBinary, "text", "a", false, "treat binary files as text")
	// GNU grep's own -I has no separate long-flag form at all (it's
	// shorthand for --binary-files=without-match, which gg doesn't
	// implement — see README LIMITATIONS). Registering it with an empty
	// long name to match that exactly renders as a bare "-I, --" in
	// --help output, which looks broken rather than intentional — so
	// gg gives it "--skip-binary" as its own long form instead, a gg-
	// specific convenience rather than a claim of GNU compatibility.
	flagSet.BoolVarP(&f.skipBinary, "skip-binary", "I", false, "skip binary files entirely, as if they had no matches")

	cmd.MarkFlagsMutuallyExclusive("files-with-matches", "files-without-match")
	cmd.MarkFlagsMutuallyExclusive("no-filename", "with-filename")
}
