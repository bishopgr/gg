# gg(1) — cross-platform grep

> This README is written in a man-page shape but doubles as a
> **progress report**: every section states plainly what is
> implemented and tested versus what's still planned. For the full
> design/build plan see [PROCESS.md](PROCESS.md).

## NAME

`gg` — search files for lines matching a pattern (grep-compatible, Windows-first)

## SYNOPSIS

```
gg [OPTION]... PATTERN [FILE...]
gg --version
gg --help
```

**`gg` is now a working, usable grep implementation** — matching,
multi-file/stdin/recursive search, `--include`/`--exclude`, output
shaping (`-n -c -o -l -L -q -v`), and color are all wired end-to-end.
See [OPTIONS](#options) for exactly what's implemented vs. still
planned, and [LIMITATIONS](#limitations-current-expected-to-shrink) for
explicit, honest gaps (context lines, `-e`/`-f`, PCRE, etc.).

## DESCRIPTION

`gg` searches `FILE`(s) for lines matching `PATTERN` and prints them.
When no `FILE` is given, or `FILE` is `-`, `gg` reads standard input.

```powershell
gg error app.log                    # search one file
gg -i "warn|error" app.log           # case-insensitive, alternation
gg -rn TODO .\src                    # recursive, with line numbers
gg -rl "TODO" .\src                  # just the filenames that contain it
gg -c error app.log                  # just a count
gg -v skip app.log                   # invert: lines NOT matching
gg -o "\d+" app.log                  # only the matched substrings
Get-Content app.log | gg error       # read from stdin
gg --include=*.go -r TODO .          # only search .go files
```

See [EXAMPLES](#examples) below for a walkthrough with real, verified
output, from single-file searches up through combining several flags at
once.

## EXAMPLES

Every command and output block below was actually run against `gg.exe`
built from this repo (`go build -o bin/gg.exe ./cmd/gg`) — copy-pasted,
not invented. They assume a working directory shaped like this:

```
.
├── app.log
├── src
│   ├── main.go
│   └── util
│       └── util.go
└── vendor
    └── thirdparty.go
```

`app.log`:

```
2024-01-01T10:00:00 INFO  starting service
2024-01-01T10:00:01 WARN  cache miss for key=42
2024-01-01T10:00:02 ERROR failed to connect to db: timeout
2024-01-01T10:00:03 INFO  retrying connection
2024-01-01T10:00:04 ERROR failed to connect to db: timeout
2024-01-01T10:00:05 INFO  connected
```

`src\main.go` and `src\util\util.go` each have a `// TODO: ...` comment
line; `vendor\thirdparty.go` has one too, to demonstrate excluding it.

### Simple

Search one file for a literal-looking pattern:

```powershell
PS> gg ERROR app.log
2024-01-01T10:00:02 ERROR failed to connect to db: timeout
2024-01-01T10:00:04 ERROR failed to connect to db: timeout
```

Case-insensitive search (`-i`):

```powershell
PS> gg -i warn app.log
2024-01-01T10:00:01 WARN  cache miss for key=42
```

Count matching lines instead of printing them (`-c`):

```powershell
PS> gg -c ERROR app.log
2
```

Read from standard input instead of a file:

```powershell
PS> Get-Content app.log | gg -ci error
2
```

### Medium

Invert the match to see everything that *isn't* an error (`-v`):

```powershell
PS> gg -v INFO app.log
2024-01-01T10:00:01 WARN  cache miss for key=42
2024-01-01T10:00:02 ERROR failed to connect to db: timeout
2024-01-01T10:00:04 ERROR failed to connect to db: timeout
```

Prefix matches with their line number, and require a whole-word match
(`-n -w`, so this wouldn't match e.g. `ERRORS`):

```powershell
PS> gg -n -w ERROR app.log
3:2024-01-01T10:00:02 ERROR failed to connect to db: timeout
5:2024-01-01T10:00:04 ERROR failed to connect to db: timeout
```

Extract just the matched substring with a regex, instead of whole lines
(`-o`) — here, pulling out every date:

```powershell
PS> gg -o "\d{4}-\d{2}-\d{2}" app.log
2024-01-01
2024-01-01
2024-01-01
2024-01-01
2024-01-01
2024-01-01
```

Recurse into a directory tree, with line numbers (`-r -n`, combined as
`-rn`):

```powershell
PS> gg -rn TODO .
src\main.go:3:// TODO: add graceful shutdown
src\util\util.go:3:// TODO: replace with a real implementation
vendor\thirdparty.go:2:// TODO: this should never show up in results
```

Same search, but only print the filenames that matched (`-rl`):

```powershell
PS> gg -rl TODO .
src\main.go
src\util\util.go
vendor\thirdparty.go
```

### Complex

Skip a whole directory while recursing (`--exclude-dir`) — useful for
keeping `vendor`, `node_modules`, `.git`, etc. out of results:

```powershell
PS> gg -rn --exclude-dir=vendor TODO .
src\main.go:3:// TODO: add graceful shutdown
src\util\util.go:3:// TODO: replace with a real implementation
```

Combine several flags at once — recursive, case-insensitive, only `.go`
files, excluding `vendor`, with line numbers — to find every TODO in
first-party Go source and nowhere else:

```powershell
PS> gg -rni --include=*.go --exclude-dir=vendor "todo" .
src\main.go:3:// TODO: add graceful shutdown
src\util\util.go:3:// TODO: replace with a real implementation
```

Show the lines immediately surrounding each match (`-C`, context on both
sides) — here the two matches are close enough that their context
windows overlap, so they merge into one continuous block with no `--`
separator, and shared lines aren't duplicated:

```powershell
PS> gg -C 1 -n ERROR app.log
2-2024-01-01T10:00:01 WARN  cache miss for key=42
3:2024-01-01T10:00:02 ERROR failed to connect to db: timeout
4-2024-01-01T10:00:03 INFO  retrying connection
5:2024-01-01T10:00:04 ERROR failed to connect to db: timeout
6-2024-01-01T10:00:05 INFO  connected
```

Non-adjacent matches, by contrast, get a `--` separator between their
groups — here with `-A0`, which prints zero actual context lines but
still separates the two matches (this is deliberate, GNU-grep-compatible
behavior; a bare `-A`/`-B`/`-C` with no context flag at all never prints
separators — see OPTIONS):

```powershell
PS> gg -A0 -n ERROR app.log
3:2024-01-01T10:00:02 ERROR failed to connect to db: timeout
--
5:2024-01-01T10:00:04 ERROR failed to connect to db: timeout
```

Search for several patterns at once, kept in a file instead of the
command line (`-f`) — useful for a reusable list like log severities to
flag:

```powershell
PS> Get-Content levels.txt
ERROR
WARN
PS> gg -n -f levels.txt app.log
2:2024-01-01T10:00:01 WARN  cache miss for key=42
3:2024-01-01T10:00:02 ERROR failed to connect to db: timeout
5:2024-01-01T10:00:04 ERROR failed to connect to db: timeout
```

The same search inline with repeated `-e` instead of a file — once `-e`
(or `-f`) is used at all, every positional argument becomes a `FILE`,
never a second pattern, so this needs no other special syntax:

```powershell
PS> gg -e ERROR -e WARN -n app.log
2:2024-01-01T10:00:01 WARN  cache miss for key=42
3:2024-01-01T10:00:02 ERROR failed to connect to db: timeout
5:2024-01-01T10:00:04 ERROR failed to connect to db: timeout
```

Anchor a whole-line match (`-x`), e.g. to assert a file's first line is
*exactly* `package main` and nothing else on that line:

```powershell
PS> gg -x "package main" src\main.go
package main
```

Use `gg` purely for its exit code in a script or CI check (`-q`
suppresses all output and stops at the first match) — `0` means found,
`1` means not found:

```powershell
PS> gg -q ERROR app.log; $LASTEXITCODE
0
PS> gg -q nomatch app.log; $LASTEXITCODE
1
```

Force color even when output isn't a terminal (e.g. piping to a file
you'll view later with a color-aware viewer) — GNU-style `--color`
resolution means bare `--color`, `--color=always`, and `--color=auto`
piped to a real terminal all highlight the match, here shown against
`-n`:

```powershell
PS> gg --color=always -n ERROR app.log
3:2024-01-01T10:00:02 ERROR failed to connect to db: timeout
5:2024-01-01T10:00:04 ERROR failed to connect to db: timeout
```

(In an actual terminal, `ERROR` renders bold red in that output —
Markdown can't render the ANSI escape codes gg emits, but they're
really there; see `internal/output`.)

## STATUS

Legend: ✅ done + tested · 🚧 built, not wired in · ⬜ not started

| Area | State | Notes |
|---|---|---|
| CLI scaffold (Cobra root command, `--version`, `--help`, usage errors) | ✅ | `internal/cli` |
| **End-to-end search execution** (`gg PATTERN FILE` producing real output) | ✅ | `grep.Run` — see OPTIONS below for exact flag coverage |
| Exit code contract (`0`/`1`/`2`) | ✅ | Permanently regression-tested via a swappable `runGrep` seam in `internal/cli`, plus real-process e2e tests in `test/e2e` |
| Ctrl+C / graceful shutdown context threading | ✅ | `signal.NotifyContext` in `main.go` → Cobra `ExecuteContext` → `grep.Run(ctx, opts)`; checked eagerly and per-line |
| File opening (Windows-appropriate sharing semantics, "-" for stdin) | ✅ | `internal/source`, wired into `grep.Run` |
| Build-time version metadata (`-ldflags`) | ✅ | `internal/buildinfo` + `scripts/build.ps1` |
| Matching engine (regex + fixed-string, `-i`/`-F`/`-w`/`-x`/`-v`) | ✅ | `internal/matcher`, wired into `grep.Run` |
| Line-splitting (CRLF/LF/CR, no-trailing-newline) | ✅ | `internal/lineio`, wired into `grep.Run` |
| Encoding/BOM detection, UTF-16 transcode, binary-match message | ✅ | `internal/encoding`, wired into `grep.Run`; no `-a`/`--text`/`-I` flag yet (see LIMITATIONS) |
| Recursive directory walk, glob expansion, `--include`/`--exclude`/`--exclude-dir` | ✅ | `internal/walker`, wired into `grep.Run` |
| Terminal detection, Windows ANSI enabling, `--color` | ✅ | `internal/termenv` + `grep.resolveColor`, wired into `grep.Run` |
| Output formatting (filename prefix, line numbers, `-o`, `-c`, `-l`/`-L`, color) | ✅ | `internal/output`, wired into `grep.Run` |
| Context lines (`-A`/`-B`/`-C`) | ✅ | `internal/grep`'s `contextTracker`, wired into `grep.Run`; every group/separator/precedence rule verified directly against real GNU grep — see OPTIONS below |
| Multi-pattern (`-e`/`-f`) | ✅ | `internal/grep`'s `resolvePatterns` + `matcher`'s alternation-joining, wired into `grep.Run`; precedence/combination rules verified directly against real GNU grep — see OPTIONS below |
| `-m`, `-z`, PCRE | ⬜ | Explicit non-goals for now — see LIMITATIONS |
| Black-box e2e tests against the real compiled binary | ✅ | `test/e2e` |

## OPTIONS

| Flag | Meaning | State |
|---|---|---|
| `--help` | show usage (long-flag-only — see note below) | ✅ |
| `--version` | print version, commit, build date (long-flag-only) | ✅ |
| `-i`, `--ignore-case` | ignore case distinctions | ✅ |
| `-F`, `--fixed-strings` | treat `PATTERN` as a literal string, not regex | ✅ |
| `-v`, `--invert-match` | select non-matching lines | ✅ |
| `-w`, `--word-regexp` | match only whole words | ✅ |
| `-x`, `--line-regexp` | match only whole lines | ✅ |
| `-n`, `--line-number` | prefix each printed line with its line number | ✅ |
| `-o`, `--only-matching` | print only the matched part of each line | ✅ |
| `-c`, `--count` | print only a per-file count of matching lines | ✅ |
| `-l`, `--files-with-matches` | print only names of files with a match | ✅ |
| `-L`, `--files-without-match` | print only names of files with no match | ✅ |
| `-q`, `--quiet` | no output at all; exit status only (stops at first match) | ✅ |
| `-h`, `--no-filename` | never print filenames | ✅ |
| `-H`, `--with-filename` | always print filenames | ✅ |
| `-r`, `--recursive` | recurse into directories (does not follow symlinks) | ✅ |
| `-R`, `--dereference-recursive` | like `-r`, but follow all symlinks | ✅ |
| `--include=GLOB` | only search files matching `GLOB` when recursing | ✅ |
| `--exclude=GLOB` | skip files matching `GLOB` when recursing | ✅ |
| `--exclude-dir=GLOB` | don't descend into directories matching `GLOB` | ✅ |
| `--color[=auto\|always\|never]` | highlight matches; bare `--color` means `always` | ✅ |
| `-A NUM`, `--after-context=NUM` | print `NUM` lines of trailing context after each match | ✅ |
| `-B NUM`, `--before-context=NUM` | print `NUM` lines of leading context before each match | ✅ |
| `-C NUM`, `--context=NUM` | print `NUM` lines of context on both sides (like `-A NUM -B NUM`, unless `-A`/`-B` is *also* given — see note below) | ✅ |
| `-e PATTERN`, `--regexp=PATTERN` | add `PATTERN` to the list of patterns to match (may be repeated) | ✅ |
| `-f FILE`, `--file=FILE` | add patterns from `FILE`, one per line (may be repeated; `-` means stdin) | ✅ |
| `-m NUM` | stop after `NUM` matches | ⬜ planned |
| `-a`, `--text`, `-I` | force text mode / skip binary files entirely | ⬜ planned (binary files currently always print the `Binary file X matches` message, never raw lines, never skipped) |
| `-z` | NUL-separated input/output | ⬜ planned |
| `-P` (PCRE) | Perl-compatible regex | ⬜ non-goal — RE2 syntax only (PROCESS.md §1) |

**Why `--help`/`--version` are long-flag-only**: GNU grep has never used
`-h` for help or `-v` for version — `-h` means `--no-filename` and `-v`
means `--invert-match`. `gg` matches that convention rather than
Cobra's defaults; see the doc comment on `registerFlags` in
`internal/cli/root.go` for exactly how (it needed two different
workarounds for two asymmetric Cobra behaviors, verified against
Cobra's source, not assumed).

**`-A`/`-B`/`-C` precedence**: an explicitly-given `-A` or `-B` always
wins for its own direction over `-C`, regardless of the order the flags
appear — `gg -A 1 -C 3` and `gg -C 3 -A 1` both resolve to `after=1,
before=3`, not "whichever flag comes last wins." Verified directly
against real GNU grep rather than assumed, since it's genuinely
counter-intuitive for a typical flag-parsing library's default
last-one-wins behavior (see `internal/cli/root.go`'s `RunE` for exactly
how). Also verified and replicated: explicitly passing `-A0`/`-B0`/`-C0`
still turns on `--` group separators between non-adjacent matches, even
though zero actual context lines get printed — that's genuinely
different, observable output from not passing any context flag at all
(`internal/grep`'s `Options.ContextEnabled` exists specifically to carry
that distinction). GNU grep's `-NUM` shorthand for `-C NUM` (e.g. `grep
-3 pattern file`) is **not** implemented — see LIMITATIONS.

**`-e`/`-f` precedence**: once *either* is given, the positional
`PATTERN` argument disappears entirely — every remaining positional
argument becomes a `FILE` instead, even one that "looks like" it was
meant as a pattern (`gg -e foo bar file` searches `bar` and `file` as
files, it does not also match against `bar`). Verified directly against
real GNU grep, not assumed. Patterns from `-e` and `-f` combine in a
simple, documented order — every `-e` pattern first (in the order
given), then every `-f` file's lines (files in the order given, lines in
file order) — rather than attempting exact command-line interleaving
between the two flags, since regex alternation doesn't make that
distinction observable anyway. A blank line in a `-f` file becomes a
literal empty-string pattern (matches every line), exactly like `-e ""`
— verified, not skipped. Supplying **zero** patterns total (e.g. an
empty `-f` file and no `-e` at all) makes the search match *nothing*,
not *everything* — also verified directly against real GNU grep, and the
opposite of what compiling an actually-empty regex would do.

Full phased rollout this table tracks:
[PROCESS.md §1](PROCESS.md#1-decide-the-feature-surface-write-this-down-before-coding).

## EXIT STATUS

| Code | Meaning |
|---|---|
| `0` | at least one match was found (also: `--version`/`--help` succeeded) |
| `1` | ran successfully, found no matches |
| `2` | usage error, invalid pattern, or a per-file problem (missing file, permission denied, a directory given without `-r`) |

Per-file problems (missing file, `Is a directory`, etc.) are reported
directly to stderr as they're found, one line per problem, exactly like
GNU grep — search continues with the remaining files, and the exit code
still ends up `2`. A single whole-command-fatal error (e.g. an invalid
regex pattern) is reported once, via the CLI's generic `gg: <err>`
handler, instead — see the doc comment on `grep.Run` for the reasoning
behind that split.

## ARCHITECTURE

```
cmd/gg/main.go        entrypoint — only place that calls os.Exit          [done]
internal/cli/         Cobra command tree, flag parsing only               [done]
internal/grep/         composition root: Run(ctx, opts) (exitCode int, err error) [done: MVP+]
internal/matcher/      pattern matching: fixed-string + regex, no I/O     [done]
internal/buildinfo/    Version/Commit/Date via -ldflags                   [done]
internal/lineio/       CRLF/LF/CR line splitting, streaming, no length cap [done]
internal/encoding/     BOM detect/strip, UTF-16->UTF-8 transcode, binary sniff [done]
internal/walker/       file/dir traversal, glob expansion, include/exclude [done]
internal/output/       printer: filename/line-number/-o/color formatting  [done]
internal/termenv/      Windows console-mode / isatty handling            [done]
internal/source/       open a file or stdin, Windows-safe sharing flags  [done]
test/e2e/               black-box tests against the compiled binary       [done]
```

`internal/grep` is the seam: `internal/cli` depends on it, and it now
depends on every other package listed above — `matcher`, `lineio`,
`encoding`, `walker`, `source`, `output`, `termenv` — while nothing
above it leaks Cobra or terminal concerns downward. See PROCESS.md §2
for the full rationale.

### `internal/grep` design (implemented) — the composition root

- `Run(ctx, opts) (exitCode int, err error)` wires everything together
  per file: `source.Open` → `encoding.Sniff` → `lineio.NewReader` →
  `matcher.Find` per line → `output.Printer`. `walker.Walk` drives that
  per-file loop for directory roots; a bare file or `"-"` (stdin) skips
  straight to it.
- **Two kinds of failure, deliberately distinguished** (see the `Run`
  doc comment for the full reasoning): a single whole-command-fatal
  error (bad pattern, cancelled context) is *returned*, for
  `internal/cli`'s generic `gg: <err>` handler to print once; a
  per-file problem hit while searching multiple files (missing file,
  permission denied, a directory without `-r`) is written directly to
  `opts.Stderr` as it's found and *never* returned, so it isn't also
  wrapped in a redundant generic line — exactly how GNU grep prints one
  diagnostic per bad file and keeps going. Either way, `hadError` forces
  the final exit code to `ExitError`, even if other files matched.
- **Mode precedence** (`-q` > `-l`/`-L` > `-c` > normal) is resolved
  once via `selectMode`, not scattered `if` checks through the search
  loop.
- **`-q` actually stops searching**, not just suppresses output: the
  moment quiet mode finds its first match, the walk aborts (via an
  internal `errStop` sentinel) and no further files — including ones
  that would have errored — are even opened. Regression-tested
  (`TestRun_Quiet_StopsSearchingAfterFirstMatch`) by pairing a matching
  file with a missing one and confirming the missing one is never
  reached.
- **Filename-prefix policy** matches GNU grep's actual convention, not
  a simplified guess: shown when there's more than one root *or* when
  recursing (a recursive search that happens to find exactly one file
  still gets a prefix in real grep), overridable by `-H`/`-h`.
- **Color policy resolution lives here, not in `internal/termenv`**:
  `resolveColor` combines the `--color` value with
  `termenv.IsTerminal`/`termenv.EnableANSI` on `opts.Stdout` — continuing
  the "low-level packages report facts, the composition root decides
  policy" rule every package here follows.
- **Binary files** print GNU grep's conventional
  `Binary file FILENAME matches` message instead of raw (likely
  garbage) line content — there is no `-a`/`--text`/`-I` flag yet to
  override this, which is an explicit, stated gap (see LIMITATIONS),
  not a silent one.
- **Context lines (`-A`/`-B`/`-C`)** are handled by a dedicated
  `contextTracker` (`internal/grep/context.go`) implementing GNU grep's
  own streaming algorithm: a capped ring buffer of the most recent
  non-selected lines (leading-context candidates) plus a trailing-
  context countdown, so no whole-file buffering is needed regardless of
  `-A`/`-B` size. It is deliberately **one instance shared across every
  file** a run searches (reset per-file via `startFile`, but its
  "has anything ever been printed" flag is not) because real GNU grep's
  `--` group separator fires between two different files' output too,
  not just between two groups within one file — verified directly
  against it, not assumed, and regression-tested
  (`TestRun_Context_SeparatorBetweenDifferentFiles`). Every other
  behavioral rule here — merge-without-separator when context windows
  overlap, `-A0`/`-C0` still enabling separators despite printing zero
  context text, `-o` keeping the separator but dropping context line
  text, `-v` letting an excluded-because-it-matched line resurface as a
  highlighted context line — was independently verified against real
  GNU grep (Git for Windows' `grep.exe`) rather than derived from the
  man page; see the commit introducing this feature for the full
  transcript.
- **Multi-pattern (`-e`/`-f`)**: `resolvePatterns`
  (`internal/grep/patterns.go`) is a small pre-processing step `Run`
  calls before `matcher.New`, combining literal patterns (`-e`, or the
  positional `PATTERN`) with every `-f` file's lines, in that order.
  Pattern files are read via `internal/source.Open` +
  `internal/lineio.NewReader` — the exact same Windows-correct opening
  and CRLF/LF/CR-safe line splitting used for files being *searched* —
  rather than a second, less careful line-splitting implementation, and
  `-f -` means stdin for the same reason every other `FILE` argument
  does. A pattern file that can't be opened or read is a **fatal**,
  once-returned error (like an invalid regex), not a per-search-file
  diagnostic — with no readable pattern list there's nothing left to
  search with. `internal/cli` resolves the "positional `PATTERN` vs.
  `-e`/`-f`" precedence (once either flag is given, *every* positional
  argument becomes a `FILE`, verified directly against real GNU grep)
  before `Options` is even constructed, so this package only ever sees
  an already-decided, unambiguous pattern list.

### `internal/output` design (implemented)

- `Printer` is pure formatting policy: filename/line-number prefixing,
  `-o`'s one-match-per-output-line behavior, ANSI highlighting, and the
  `-c`/`-l`/`-L` message shapes — it never decides *whether* a line
  matched or *whether* color should be used, matching every other
  package's "report/apply facts, don't decide them" contract.
  `internal/grep` is the only caller and the only place those policy
  decisions get made.
- Buffered (`bufio.Writer`) for performance, with an explicit `Flush`
  the caller must call — `grep.Run` defers it once, right after
  construction, so every return path (including `-q`'s early exit) is
  covered.
- Every printed line ends with a bare `'\n'`, unconditionally — the
  PROCESS.md §3.1 decision, finally with code enforcing it. Verified at
  the real-process level too (`TestE2E_OutputAlwaysEndsLinesWithBareLF`
  greps gg's own output for a stray `\r`).
- `-v`'s inverted matches carry no highlight ranges (there's nothing to
  highlight in a line selected for *not* matching), and `-o` combined
  with `-v` prints nothing for those lines — both match GNU grep's own
  behavior for this combination and are explicitly tested, not just
  incidentally correct.
- `PrintContextLine`/`PrintContextSeparator` extend the same contract
  for `-A`/`-B`/`-C`: a `-` prefix separator instead of `:` for context
  lines, still capable of highlighting a context line's own literal
  matches (relevant under `-v`), and both silently no-op the context
  *text* under `-o` while still emitting the group separator — decisions
  `internal/grep`'s `contextTracker` makes, not this package; `Printer`
  just renders what it's told.

### `internal/matcher` design (implemented)

- One `Matcher` interface, two implementations selected by `matcher.New`:
  - **`fixedMatcher`** — plain `bytes.Index` scan, used for bare `-F`
    (no `-i`/`-w`/`-x`). No regexp compilation overhead.
  - **`regexMatcher`** — Go `regexp` (RE2 syntax), used for the default
    regex mode, and also for `-F` combined with `-i`/`-w`/`-x` (via
    `regexp.QuoteMeta`), trading a bit of performance for correct
    Unicode case folding and `\b`/`^$` boundary semantics "for free."
- `-x` (whole-line) takes precedence over `-w` (whole-word) when both
  are set, matching GNU grep.
- Empty-pattern behavior is pinned down and cross-validated between both
  implementations (zero-width matches at every rune boundary).
- **RE2, not PCRE**: no backreferences, no lookaround. This is a stated
  non-goal for v1 (PROCESS.md §1), not an oversight.
- **Multi-pattern (`-e`/`-f`)**: `Options.Patterns` is a slice, not a
  single string. Multiple entries are combined into one alternation
  (each escaped with `regexp.QuoteMeta` first when `-F` is set, so every
  pattern in the list is still literal, not just the first) and compiled
  as a **single** regexp, rather than running one matcher per pattern
  per line — this both avoids needing to merge match ranges from
  several independent matchers into `Matcher`'s required ascending/
  non-overlapping order, and lets one `-x`/`-w`/`-i` wrap apply to the
  whole alternation at once, exactly like real grep does. Each pattern
  is validated with its own `regexp.Compile` call first, purely so a bad
  pattern among several is reported by *itself* (`compileError`) rather
  than as an unreadable fragment of the final joined expression. An
  **empty** `Patterns` slice is a deliberate, valid input — `New`
  returns a `neverMatcher` that matches nothing, rather than compiling
  an empty-string pattern (which would zero-width-match every line
  instead) — verified directly against real GNU grep's own behavior for
  `-e`/`-f` supplying zero patterns between them, not assumed.

### `internal/lineio` design (implemented)

- A single `Reader` type wraps `bufio.Reader` and recognizes `\n`,
  `\r\n`, and bare `\r` as line terminators, returning each line's raw
  `Text` plus which `Terminator` followed it (`LF`/`CRLF`/`CR`/
  `NoTerminator`) — no terminator is ever silently assumed.
- **No maximum line length**, unlike `bufio.Scanner`'s default 64KiB
  token cap — verified with a test line >5x the internal buffer size.
- **Performance pass done** (PROCESS.md §4 step 8): rewritten from an
  initial byte-by-byte `bufio.Reader.ReadByte` scan (correctness-first,
  deliberately simple) to bulk-scanning whatever's already buffered via
  `bytes.IndexByte`, returning a slice that aliases bufio's own buffer
  directly (zero-copy) whenever a line fits in one buffered chunk.
  Measured ~211 MB/s → ~3.6 GB/s on LF input, ~227 MB/s → ~3.35 GB/s on
  CRLF (`internal/lineio/bench_test.go`). A real quadratic-blowup bug
  (an intermediate version re-scanned an entire buffered chunk per line
  when one terminator byte never appeared) was caught by benchmarking
  before shipping — see PROCESS.md §4 step 8 for the full story,
  including the accepted, still-slower-but-no-longer-quadratic
  bare-CR-only tradeoff (`BenchmarkReadLine_BareCR`).
- **`Reset(io.Reader)`** lets one `Reader` (and its internal buffers) be
  reused across many files — set up now so the future `walker`-driven
  multi-file loop doesn't allocate a fresh reader per file.
- Fixtures proving the actual concern in PROCESS.md §3.1 — that CRLF and
  LF files must produce *identical line content* — live in
  `test/testdata/` (`crlf.txt`, `lf.txt`, `no-trailing-newline.txt`,
  `cr.txt`, `empty.txt`) and are asserted byte-exact via `[]byte` (not
  string-literal source, which could get silently re-encoded).
- Round-trip fuzzed (`FuzzReader_RoundTrip`, ~1.3M executions): for
  arbitrary bytes — including invalid UTF-8 and embedded NULs —
  concatenating every line's `Text` + `Terminator.Bytes()` must
  reconstruct the original input exactly.

### `internal/encoding` design (implemented — detection only)

- `Sniff(io.Reader) (Result, io.Reader)` peeks at most 8000 leading
  bytes (never slurps the file) to detect a byte-order mark and decide
  whether the stream looks binary, returning a `Result` plus a `Reader`
  picking up exactly where the original content does.
- **UTF-8 BOM**: stripped, not transcoded (the bytes after it are
  already UTF-8) — this is the classic bug PROCESS.md §3.2 calls out:
  without stripping it, `^pattern` silently fails to anchor on line 1.
- **UTF-16 LE/BE BOM**: detected and transcoded to UTF-8 streaming via
  `golang.org/x/text/encoding/unicode` + `transform.NewReader` — chosen
  over "treat UTF-16 as binary" specifically because this project is
  Windows-first, and Windows PowerShell 5.1's default `Out-File`
  encoding is UTF-16LE. No-BOM UTF-16 remains undetectable (no reliable
  signature exists) and is an accepted, documented gap.
- **Binary detection**: a NUL byte anywhere in the (BOM-stripped) 8000-byte
  prefix marks a stream binary — mirrors Git's `buffer_is_binary`
  heuristic. Critically, this check is **skipped entirely** (forced
  `false`) when a UTF-16 BOM was positively identified: UTF-16-encoded
  ASCII text is legitimately full of zero bytes (every character has a
  zero high or low byte), so applying the naive heuristic there would
  flag nearly all UTF-16 text as binary. This was caught and fixed
  during test-writing, not assumed correct up front — see
  `TestSniff_UTF16LE_TranscodesAndIsNotFlaggedBinary`.
- **Detection only, no policy**: `Sniff` reports `Result.Binary`; it does
  not skip the file or print `Binary file X matches` — that decision
  needs to know about `-a`/`--text`/`-I`, which belong to the not-yet-built
  `grep` engine, not this package.
- Proven to compose with `lineio` as intended
  (`TestSniffThenLineio`): a UTF-16LE file with CRLF line endings splits
  into the same lines a plain UTF-8/LF file would.
- **No accidental double-buffering with `lineio`** (PROCESS.md §4 step
  8): `Sniff`'s internal buffer is sized to match
  `lineio.DefaultBufSize` (64 KiB) — larger than the 8000-byte
  `sniffLen` it actually peeks — specifically so `lineio.NewReader`
  recognizes and reuses it directly instead of wrapping it in a second
  buffer, which previously capped every downstream read at whatever the
  smaller inner buffer returned per call. Verified at the real
  composition point, not just inferred from throughput numbers
  (`TestSniffThenLineio_NoDoubleBuffering`).
- Fuzzed (`FuzzSniff`, ~2.2M executions): for the non-UTF-16 path,
  asserts byte-exact reconstruction after BOM-stripping; for the UTF-16
  path, asserts only "never panics, decode errors are the worst case"
  since arbitrary fuzz bytes aren't guaranteed valid UTF-16.

### `internal/termenv` design (implemented)

- `IsTerminal(*os.File) bool` wraps `golang.org/x/term.IsTerminal` rather
  than hand-rolling `isatty` — Windows terminal detection isn't a single
  ioctl the way Unix's is (conhost vs. ConPTY vs. redirected handle), so
  this defers to a maintained, cross-platform implementation.
- `EnableANSI(*os.File) bool` attempts to turn on
  `ENABLE_VIRTUAL_TERMINAL_PROCESSING` via `golang.org/x/sys/windows`'
  `GetConsoleMode`/`SetConsoleMode` directly — chosen over a wrapper
  library (`go-colorable`, `termenv`) since it's a handful of lines
  against a stable Win32 API, and those libraries' main value (ANSI
  translation for pre-Windows-10 consoles with no VT support at all)
  isn't a target for this project. Split by build tag: `ansi_windows.go`
  does the real work; `ansi_other.go` is an unconditional-`true` no-op,
  since non-Windows terminals already handle ANSI natively.
- **Detection only, no policy**, same pattern as `matcher`/`lineio`/
  `encoding`: neither function decides whether color should actually be
  used — combining these with a `--color=auto|always|never` flag value
  belongs to the future output layer (§4 step 7).
- **Known test gap, stated rather than hidden**: unit tests prove
  `IsTerminal`/`EnableANSI` correctly report "not a terminal" for regular
  files and pipes — the only two cases constructible inside an
  automated `go test` run. Behavior against a genuine interactive
  console (legacy conhost vs. Windows Terminal/ConPTY) has **not** been
  verified and needs manual testing; tracked in LIMITATIONS below.

### `internal/walker` design (implemented)

- `ExpandGlobs([]string) ([]string, error)` exists because PowerShell
  and cmd.exe — unlike POSIX shells — never expand globs before
  invoking a process: `gg pattern *.go` reaches `gg` with the literal
  three characters `*.go` as an argument. Precedence: a literal existing
  path always wins over glob interpretation (a real file named
  `a[1].txt` isn't glob-mangled); a pattern with zero matches passes
  through unchanged so a "no such file" error surfaces naturally later,
  mirroring a POSIX shell's own non-nullglob behavior.
- `Walk(root string, opts Options, visit VisitFunc) error` — single-root,
  callback-based, deterministic (lexical) order, same "report facts,
  not policy" contract as every other package here: it never decides to
  print a filename prefix or a binary-file warning, just calls `visit`.
- **Matches grep's actual "Is a directory" behavior**, not an invented
  shallow-scan mode: a directory root without `Recursive: true` reports
  `ErrIsDir` instead of silently listing one level — real grep doesn't
  have a shallow mode either, so this doesn't invent one.
- **Symlink policy**: default `FollowSymlinks: false` skips any symlink
  *encountered while recursing* entirely (neither read nor descended);
  an explicit root argument is always dereferenced regardless (matching
  `cat somelink`-style "explicit args are always followed" convention).
  `FollowSymlinks: true` is implemented now, with cycle detection via
  `filepath.EvalSymlinks`-canonicalized visited-directory tracking,
  tested against a real symlink cycle under a 5-second hang guard — not
  deferred to the future `--follow` *flag*, only that flag's CLI wiring is.
- **Case-aware include/exclude matching**: case-insensitive on Windows,
  case-sensitive elsewhere (`runtime.GOOS`-gated), mirroring each OS's
  own native tools. The case-folding logic is factored out from the OS
  check specifically so both branches are unit-tested directly on one
  CI runner, rather than needing platform-specific build tags to test
  what's otherwise identical logic.
- **Bad glob patterns fail fast**: `--include`/`--exclude`/
  `--exclude-dir` patterns are validated against `filepath.Match` up
  front, before any traversal starts — `filepath.Match`'s `ErrBadPattern`
  is easy to swallow accidentally deep in recursive matching code, so
  it's checked once, eagerly, instead.
- **Long paths verified, not engineered around**: Go 1.26 on this
  Windows toolchain walks a >300-character path with zero special
  handling (no `\\?\` prefix, no manifest change) — proven with a
  regression test rather than assumed.
- **Caveat, not fully verified**: Windows directory junctions are
  reparse points, not symlinks; whether Go's `os` package reports them
  with `fs.ModeSymlink` (so the same skip/follow logic applies) is
  believed true on modern Go but wasn't specifically tested — creating a
  junction needs `mklink /J` or raw syscalls, not `os.Symlink`.
- Fuzzed (`FuzzMatchOneCaseAware`, `FuzzExpandGlobs`): ~900K executions
  combined against arbitrary glob patterns and names, zero panics.

### `internal/source` design (implemented) — a real Windows bug found and fixed

- `Open(path string) (ReadCloser, error)` opens a named file, or standard
  input when `path == "-"` (the Close on the stdin wrapper is always a
  safe no-op — it never closes the process's real `os.Stdin`).
- **This is the one package so far where the correctness checklist
  surfaced an actual bug, not just a decision to document.** Verified
  against Go 1.26's own `src/syscall/syscall_windows.go` *and*
  empirically with a throwaway scratch program: plain `os.Open` on
  Windows opens with `FILE_SHARE_READ|FILE_SHARE_WRITE` but **not**
  `FILE_SHARE_DELETE`. Concretely: a concurrent *write* to a file gg has
  open for reading succeeds, but a concurrent *rename or delete* —
  exactly what typical log rotation does — fails with "The process
  cannot access the file because it is being used by another process."
  POSIX has no such restriction.
- **Fixed**, not just documented: on Windows, `internal/source` opens
  files via `golang.org/x/sys/windows.CreateFile` directly, adding
  `FILE_SHARE_DELETE`. Regression-tested directly — `TestOpen_
  AllowsDeleteWhileOpen` and `TestOpen_AllowsRenameWhileOpen` both fail
  against plain `os.Open` and pass against this fix; `TestOpen_
  StillReadableAfterConcurrentDelete` proves the fix is actually useful
  (the still-open handle keeps reading the original content after
  the file is unlinked, matching POSIX unlink-while-open semantics).
- **Reserved device names** (`NUL`, `CON`, `PRN`, `COM1`, ...): decided
  against any special-casing, verified for `NUL` specifically —
  `os.Stat("NUL")` reports a non-directory character device and
  `os.Open("NUL")` + read yields immediate EOF with no error, the same
  harmless outcome Unix grep gets from `/dev/null`. `CON` was
  deliberately **not** live-tested (opening it could block on keyboard
  input with no console attached to an automated test) — the same
  reasoning is expected to hold but is flagged as unverified, not
  claimed.

## BUILDING

```powershell
# quick local build, no version metadata
go build -o bin/gg.exe ./cmd/gg

# release-style build with git-derived version/commit/date
./scripts/build.ps1
```

Requires Go 1.26+. No CGO, no external system dependencies.

## TESTING

```powershell
go vet ./...
go test ./...
```

- `internal/matcher` has table-driven unit tests for both implementations,
  a cross-validation test asserting `fixedMatcher` and
  `regexMatcher(QuoteMeta(pattern))` agree wherever their behavior is
  supposed to overlap, and a fuzz target (`FuzzNew`) exercised for
  ~1M generated executions with zero panics. Multi-pattern (`-e`/`-f`)
  support adds its own cases: OR'd-together matching, `-F`/`-x`/`-w`
  each applying to every pattern in the list (not just the first), an
  invalid pattern among several being attributed to *that* pattern in
  the error message, and the empty-`Patterns`-never-matches case.
- `internal/lineio` has table-driven tests covering every terminator
  combination (including adjacent/degenerate cases like `"\r\r\n"`), the
  `test/testdata/` fixtures above, an arbitrarily-long-line test, and a
  byte-perfect round-trip fuzz target (~1.3M executions, zero mismatches).
- `internal/encoding` has table-driven tests for BOM detection, UTF-8 BOM
  stripping, UTF-16 LE/BE transcoding (including non-ASCII content,
  generated via the real `x/text` encoder so fixtures can't be
  hand-assembled wrong), the binary/UTF-16 interaction edge case above,
  an integration test proving composition with `lineio`, and a fuzz
  target (~2.2M executions, zero crashes).
- `internal/termenv` has unit tests for the two non-terminal cases
  constructible in an automated test (regular file, pipe); real-console
  behavior is a manual-testing gap, stated explicitly rather than
  claimed covered.
- `internal/walker` has unit tests for recursion, `ErrIsDir`,
  include/exclude/exclude-dir filtering, native path-separator
  preservation, symlink default-skip and follow-with-cycle-detection
  (under a 5s hang guard), a >300-character long-path regression, and
  fuzz targets for glob-pattern matching and expansion (~900K executions
  combined, zero panics).
- `internal/source` has regression tests for the Windows file-sharing
  fix specifically (concurrent write/delete/rename while open, plus
  proving the still-open handle keeps reading after a concurrent
  delete), the `"-"` → stdin mapping, and (Windows-only) the `NUL`
  reserved-device-name case.
- `internal/output` has tests for every formatting combination
  (filename/line-number prefixes, color on/off, `-o`'s one-match-per-line
  behavior including with color and with filename/line-number, `-v`'s
  no-ranges-to-highlight case, `-c`/`-l`/`-L`/binary-match message
  shapes) and confirms buffering requires an explicit `Flush`, plus
  `PrintContextLine`/`PrintContextSeparator` coverage: the `-` vs `:`
  prefix distinction, highlighting a context line's own ranges, and
  `-o` suppressing context text while still allowing the separator.
- `internal/grep` has ~70 tests covering the actual end-to-end wiring
  against real temp files: basic match/no-match, every matching flag
  (`-i`/`-F`/`-w`/`-x`/`-v`), every output-shape flag (`-n`/`-o`/`-c`/
  `-l`/`-L`/`-q` including the quiet-mode early-stop-before-reaching-a-
  broken-file case), filename-prefix policy (`-H`/`-h`/single-vs-multi-file),
  recursive search with `--include`/`--exclude`, glob expansion, stdin
  (both as the default and as an explicit `"-"` mixed with real files),
  `--color` resolution against both a forced and a non-file writer, the
  binary-file-match-message path, and Go `context.Context` cancellation
  — plus the fatal-vs-per-file error split (`internal/grep`'s central
  design decision). `-A`/`-B`/`-C` context lines specifically have their
  own dedicated table (`context_test.go`, ~15 cases, every one of them
  cross-checked against real GNU grep's actual output rather than
  assumed): before-only, after-only, before+after merging when windows
  overlap, the `-` vs `:` prefix split, `-A0`/`-C0` still enabling
  separators, `-v` surfacing an excluded match as highlighted context,
  `-o` dropping context text but keeping the separator, and that
  `-c`/`-q`/`-l` remain completely unaffected by context flags.
  `-e`/`-f` multi-pattern support similarly has its own dedicated table
  (`patterns_test.go`, covering `resolvePatterns` directly) plus
  end-to-end `Run` tests: literal-then-file combination order, blank
  lines becoming empty patterns, CRLF pattern files, a missing pattern
  file being a fatal returned error (not a per-file diagnostic), and the
  empty-pattern-list-matches-nothing case, end to end.
- `internal/cli` has tests for the exit-code contract (`--version`/
  `--help`/no-args, and — via a swappable `runGrep` seam — all three of
  `ExitMatch`/`ExitNoMatch`/`ExitError` from a fake engine),
  argument-to-`Options` wiring for every flag, the `-h`/`-v` shorthand
  reassignment (proven by checking the fake engine actually gets called
  instead of Cobra short-circuiting into help/version), `--color`'s
  GNU-style optional-value parsing (bare/`=always`/`=never`/`=auto`),
  the `-l`/`-L` and `-h`/`-H` mutual-exclusion checks, and `-e`/`-f`'s
  precedence rule specifically: a positional argument becoming a `FILE`
  (never a second pattern) once `-e`/`-f` is given, and zero positional
  arguments being valid in that case even though a bare `PATTERN` is
  still mandatory without them.
- `test/e2e` builds the real `gg` binary once (`TestMain`) and drives it
  as a subprocess for the scenarios only a real process boundary can
  validate: actual exit codes, actual stdio redirection, that `-h`/`-v`
  truly don't trigger Cobra's help/version machinery, that recursive
  search really does emit backslash-separated paths on Windows, that
  real stdout output never contains a stray `\r` byte, `-A`/`-B`/`-C`
  context lines (including the precedence and explicit-zero cases), and
  `-e`/`-f` multi-pattern behavior (including the positional-arg-becomes-
  a-FILE precedence rule and a missing pattern file's diagnostic).
- **Benchmarks** (PROCESS.md §4 step 8): `go test -bench . -benchmem
  ./internal/lineio/... ./internal/matcher/... ./internal/grep/...`
  covers the line-splitting hot path (LF/CRLF/bare-CR, plus the
  `Reset`-and-reuse shape a multi-file walk actually uses), both
  matcher implementations' match/no-match cases, and an end-to-end
  `source → encoding.Sniff → lineio → matcher → output` run against a
  generated 100,000-line fixture — kept as permanent regression guards
  next to the code they measure, not one-off scratch scripts.
- `go test -race` currently fails **locally** on this machine with
  `0xc0000139` — that's an incompatible `gcc` (Strawberry Perl's, found
  first on `PATH`) rather than a code issue; GitHub Actions'
  `windows-latest` runners have a working MinGW toolchain and will run
  `-race` cleanly in CI (see PROCESS.md §6).

## LIMITATIONS (current, expected to shrink)

Everything below is a genuine, deliberate gap — not a hidden one.

- **No `-NUM` shorthand for `-C NUM`** (e.g. GNU grep's `grep -3 pattern
  file`) — would need pre-processing raw args before Cobra/pflag parses
  them (pflag has no concept of a bare-numeric flag), and risks
  misinterpreting a pattern argument that itself starts with a digit
  after a dash; deferred rather than hacked in, unlike `-A`/`-B`/`-C`
  themselves, which are fully implemented (see OPTIONS).
- **No `-m NUM`** (stop after N matches per file) or **`-z`**
  (NUL-separated records).
- **No PCRE** (`-P`): Go's `regexp` is RE2 — no backreferences, no
  lookaround. Stated non-goal for v1 (PROCESS.md §1), not an oversight.
- **Binary files always print the `Binary file X matches` message**,
  never raw line content, with no way to override it yet — there's no
  `-a`/`--text`/`-I` flag. This is a real, if narrow, behavior gap: you
  cannot currently force gg to search a binary file as text.
- **No-BOM UTF-16 is undetectable by design** (no reliable signature)
  and reads as mostly NUL-heavy garbage text — an accepted, documented
  gap (PROCESS.md §3.2), not a bug to chase.
- **`--color`'s `auto` mode has an untested real-console path**:
  automated tests can only prove the "definitely not a terminal"
  branch (regular files, pipes, the buffers integration tests use).
  Whether `EnableANSI` actually renders correctly on both legacy
  conhost and Windows Terminal/ConPTY has not been manually verified —
  tracked as an open item, not assumed fine because the code compiles.
- **Windows directory junctions**: `internal/walker`'s symlink-following
  logic is believed, but not proven with a test, to treat junctions the
  same as `os.Symlink`-created symlinks (creating a junction needs
  `mklink /J` or raw syscalls, not `os.Symlink`).
- **No de-duplication of file arguments** — passing the same file twice
  searches it twice. Deferred until something actually needs it.
- **Filename-argument globbing only covers `internal/walker.ExpandGlobs`'s
  `filepath.Glob` syntax** — no `**` (recursive glob) support via
  something like `doublestar`. Use `-r`/`--include` instead for
  recursive filtering.

## SEE ALSO

- [PROCESS.md](PROCESS.md) — full design and build plan this project follows
- [BENCHMARK.md](BENCHMARK.md) — before/after measurements for the
  PROCESS.md §4 step 8 performance pass
- `grep(1)`, `regexp/syntax` (Go's RE2 syntax reference:
  <https://github.com/google/re2/wiki/Syntax>)
