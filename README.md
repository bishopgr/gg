# gg(1) — cross-platform grep, work in progress

> Status snapshot as of the current build. This README is written in a
> man-page shape but doubles as a **progress report**: every section below
> states plainly what is implemented, tested, and wired up versus what is
> still a stub. For the full design/build plan see [PROCESS.md](PROCESS.md).

## NAME

`gg` — search files for lines matching a pattern (grep-compatible, Windows-first)

## SYNOPSIS

```
gg PATTERN [FILE...]
gg --version
gg --help
```

## DESCRIPTION

`gg` searches `FILE`(s) for lines matching `PATTERN` and prints them. When
no `FILE` is given, or `FILE` is `-`, `gg` reads standard input.

**This is currently true only as a statement of intent.** The command
line and matching engine are built and unit-tested independently, but
they are not yet wired together — see [STATUS](#status) below.

## STATUS

Legend: ✅ done + tested · 🚧 built, not wired in · ⬜ not started

| Area | State | Notes |
|---|---|---|
| CLI scaffold (Cobra root command, `--version`, `--help`, usage errors) | ✅ | `internal/cli` |
| Exit code contract (`0`/`1`/`2`) | ✅ | Now permanently regression-tested (was manual-only since scaffolding) via a swappable `runGrep` seam in `internal/cli` |
| Ctrl+C / graceful shutdown context threading | ✅ | `signal.NotifyContext` in `main.go` → Cobra `ExecuteContext` → `grep.Run(ctx, opts)`; `Run` checks `ctx.Err()` eagerly. Nothing to gracefully flush yet — no output layer |
| File opening (Windows-appropriate sharing semantics, "-" for stdin) | ✅ | `internal/source` — fixes a real gap: plain `os.Open` on Windows blocks concurrent rename/delete (e.g. log rotation); fixed via `FILE_SHARE_DELETE`, **not yet reachable from the CLI** |
| Build-time version metadata (`-ldflags`) | ✅ | `internal/buildinfo` + `scripts/build.ps1`, falls back to `dev`/`none` without git |
| Matching engine (`Matcher` interface, fixed-string + regex impls) | ✅ | `internal/matcher` — table + fuzz tested, **not yet reachable from the CLI** |
| CLI flags for matching (`-i -v -F -w -x -e -f ...`) | 🚧 | `matcher.Options` supports `-i`/`-F`/`-w`/`-x` already; no Cobra flags expose them yet |
| Actual search execution (`gg PATTERN FILE` producing output) | ⬜ | `grep.Run` is a stub that always returns `(ExitError, "not yet implemented")` |
| Line-splitting reader (CRLF/LF/CR, no-trailing-newline) | ✅ | `internal/lineio` — table + fuzz tested, **not yet reachable from the CLI** |
| Encoding/BOM detection, UTF-16 transcode, binary sniff | ✅ | `internal/encoding` — table + fuzz tested, **detection only, no skip/`--text` policy yet, not wired into the CLI** |
| Recursive directory walk, glob expansion, `--include`/`--exclude`/`--exclude-dir` | ✅ | `internal/walker` — table + fuzz tested, **not yet reachable from the CLI** |
| Terminal detection, Windows ANSI enabling | ✅ | `internal/termenv` — tested for non-terminal cases; real-console behavior needs manual verification (see LIMITATIONS) |
| Output formatting (filename prefix, line numbers, context, color) | ⬜ | `internal/output` |

Running `gg` today will parse arguments and print usage/version correctly,
but any actual search (`gg foo somefile.txt`) currently fails with:

```
gg: gg: search engine not yet implemented
```

exit code `2`. This is expected and intentional at this stage — see
[PROCESS.md §4](PROCESS.md#4-build-order-do-these-in-sequence-committest-at-each-step)
for the build order this project is following.

## OPTIONS

Only flags Cobra actually recognizes today are marked implemented; the
rest describe what `matcher.Options` already supports internally and
what `PROCESS.md` commits to for upcoming steps.

| Flag | Meaning | State |
|---|---|---|
| `-h`, `--help` | show usage | ✅ implemented |
| `-v`, `--version` | print version, commit, build date | ✅ implemented (note: `-v` will be reassigned to `--invert-match` once flags land, matching GNU grep; version will stay `--version`-only) |
| `-i` | ignore case | 🚧 engine-ready (`matcher.Options.IgnoreCase`), no flag yet |
| `-F` | fixed string (literal), not regex | 🚧 engine-ready (`matcher.Options.Fixed`), no flag yet |
| `-w` | match whole words only | 🚧 engine-ready (`matcher.Options.WordRegexp`), no flag yet |
| `-x` | match whole lines only | 🚧 engine-ready (`matcher.Options.LineRegexp`), no flag yet |
| `-v` (invert) | select non-matching lines | ⬜ planned, engine-level (not a `matcher.Matcher` concern) |
| `-n` | prefix output with line numbers | ⬜ planned |
| `-c` | print only a count of matches | ⬜ planned |
| `-l`, `-L` | print only file names with(out) matches | ⬜ planned |
| `-q` | quiet, exit-status only | ⬜ planned |
| `-r`, `-R` | recurse into directories | ⬜ planned |
| `-o` | print only the matched part of the line | ⬜ planned |
| `-A`, `-B`, `-C NUM` | print lines of context | ⬜ planned |
| `-e PATTERN`, `-f FILE` | additional/file-sourced patterns | ⬜ planned |
| `--color=auto\|always\|never` | highlight matches | ⬜ planned |

Full phased rollout: [PROCESS.md §1](PROCESS.md#1-decide-the-feature-surface-write-this-down-before-coding).

## EXIT STATUS

Defined now in `internal/grep` and already exercised end-to-end through
the CLI, ahead of the engine itself, so the contract is locked in before
more code depends on it:

| Code | Meaning |
|---|---|
| `0` | at least one match was found (also: `--version`/`--help` succeeded) |
| `1` | ran successfully, found no matches |
| `2` | usage error, invalid pattern, missing file, or (today) "not yet implemented" |

## ARCHITECTURE

```
cmd/gg/main.go        entrypoint — only place that calls os.Exit          [done]
internal/cli/         Cobra command tree, flag parsing only               [done: scaffold]
internal/grep/         composition root: Run(opts) (exitCode int, err error) [stub]
internal/matcher/      pattern matching: fixed-string + regex, no I/O     [done]
internal/buildinfo/    Version/Commit/Date via -ldflags                   [done]
internal/lineio/       CRLF/LF/CR line splitting, streaming, no length cap [done]
internal/encoding/     BOM detect/strip, UTF-16->UTF-8 transcode, binary sniff [done: detection]
internal/walker/       file/dir traversal, glob expansion, include/exclude [done]
internal/output/       printer: plain/color, context, filename prefixing [not started]
internal/termenv/      Windows console-mode / isatty handling            [done]
internal/source/       open a file or stdin, Windows-safe sharing flags  [done]
```

`internal/grep` is the seam: `internal/cli` depends on it, it will depend
on `matcher`/`lineio`/`walker`/`output`, and nothing above it leaks
Cobra or terminal concerns downward. See PROCESS.md §2 for the full
rationale.

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

### `internal/lineio` design (implemented)

- A single `Reader` type wraps `bufio.Reader` and recognizes `\n`,
  `\r\n`, and bare `\r` as line terminators, returning each line's raw
  `Text` plus which `Terminator` followed it (`LF`/`CRLF`/`CR`/
  `NoTerminator`) — no terminator is ever silently assumed.
- **No maximum line length**, unlike `bufio.Scanner`'s default 64KiB
  token cap — verified with a test line >5x the internal buffer size.
  Deliberate tradeoff: correctness/simplicity (byte-by-byte scanning)
  over throughput for now, called out in the package doc as the
  specific thing to revisit in the PROCESS.md §4 step-8 performance pass.
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
  ~1M generated executions with zero panics.
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
- `internal/cli` and `internal/grep` have tests for the exit-code
  contract (`--version`/`--help`/no-args, and — via a swappable
  `runGrep` seam — all three of `ExitMatch`/`ExitNoMatch`/`ExitError`
  from a fake engine), argument-to-`Options` wiring, and context
  propagation/cancellation for the Ctrl+C plumbing.
- `go test -race` currently fails **locally** on this machine with
  `0xc0000139` — that's an incompatible `gcc` (Strawberry Perl's, found
  first on `PATH`) rather than a code issue; GitHub Actions'
  `windows-latest` runners have a working MinGW toolchain and will run
  `-race` cleanly in CI (see PROCESS.md §6).

## LIMITATIONS (current, expected to shrink)

- Does not yet search anything — `gg PATTERN FILE` exits `2`.
- `internal/walker`, `internal/termenv`, and `internal/source` exist and
  are tested, but nothing calls them yet — no `-r`/`-R`/`--include`/
  `--exclude`/`--color` flags exist on the CLI, glob expansion isn't
  applied to CLI arguments yet, and `grep.Run` doesn't open any files.
- Graceful shutdown on Ctrl+C is wired at the plumbing level
  (`context.Context` flows from `main.go` all the way to `grep.Run`,
  which checks `ctx.Err()`), but there's no actual long-running work or
  buffered output yet for cancellation to meaningfully interrupt —
  the real test of this is deferred until the output layer exists.
- No color output (no output layer yet at all — see above).
- No PCRE (backreferences/lookaround); RE2 syntax only.
- Encoding/BOM/binary *detection* exists (`internal/encoding.Sniff`) but
  isn't wired to anything yet — no code calls it before opening a file.
  There is also no *policy* built on top of it yet: nothing skips binary
  files or prints `Binary file X matches`, and there's no `-a`/`--text`/
  `-I` flag to control that once it exists.
- No-BOM UTF-16 is undetectable by design (no reliable signature) and
  will be read as (mostly) NUL-heavy garbage text — an accepted,
  documented gap, not a bug to chase.
- Output line-ending is **decided but not yet enforced**: gg will always
  write a bare `\n` after every printed line, regardless of the source
  file's terminator or platform (PROCESS.md §3.1) — but there is no
  output layer yet to actually do that. `internal/lineio.Terminator`
  records the source fact only and explicitly does not drive this.
- `internal/termenv`'s behavior against a genuine interactive console
  (legacy conhost vs. Windows Terminal/ConPTY) has not been verified —
  only "is this a regular file/pipe" (correctly `false`) is covered by
  automated tests. Needs manual verification in both console types.
- `internal/walker`'s symlink-following logic has not been specifically
  tested against a Windows directory junction (a different underlying
  mechanism from `os.Symlink`-created symlinks) — believed to behave
  the same way based on how modern Go's `os` package reports reparse
  points, but not proven with a test here.

## SEE ALSO

- [PROCESS.md](PROCESS.md) — full design and build plan this project follows
- `grep(1)`, `regexp/syntax` (Go's RE2 syntax reference:
  <https://github.com/google/re2/wiki/Syntax>)
