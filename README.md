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
| Exit code contract (`0`/`1`/`2`) | ✅ | Verified for `--version`, `--help`, no-args, and the engine-stub error path |
| Build-time version metadata (`-ldflags`) | ✅ | `internal/buildinfo` + `scripts/build.ps1`, falls back to `dev`/`none` without git |
| Matching engine (`Matcher` interface, fixed-string + regex impls) | ✅ | `internal/matcher` — table + fuzz tested, **not yet reachable from the CLI** |
| CLI flags for matching (`-i -v -F -w -x -e -f ...`) | 🚧 | `matcher.Options` supports `-i`/`-F`/`-w`/`-x` already; no Cobra flags expose them yet |
| Actual search execution (`gg PATTERN FILE` producing output) | ⬜ | `grep.Run` is a stub that always returns `(ExitError, "not yet implemented")` |
| Line-splitting reader (CRLF/LF/CR, no-trailing-newline) | ✅ | `internal/lineio` — table + fuzz tested, **not yet reachable from the CLI** |
| Encoding/BOM handling, binary-file detection | ⬜ | PROCESS.md §3.2 — separate concern, deliberately not in `lineio` |
| Recursive directory walk, `--include`/`--exclude` | ⬜ | `internal/walker` |
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
internal/walker/       file/dir traversal, include/exclude globs         [not started]
internal/output/       printer: plain/color, context, filename prefixing [not started]
internal/termenv/      Windows console-mode / isatty handling            [not started]
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
- `go test -race` currently fails **locally** on this machine with
  `0xc0000139` — that's an incompatible `gcc` (Strawberry Perl's, found
  first on `PATH`) rather than a code issue; GitHub Actions'
  `windows-latest` runners have a working MinGW toolchain and will run
  `-race` cleanly in CI (see PROCESS.md §6).

## LIMITATIONS (current, expected to shrink)

- Does not yet search anything — `gg PATTERN FILE` exits `2`.
- No recursive directory search, no globbing, no color output.
- No PCRE (backreferences/lookaround); RE2 syntax only.
- No encoding detection yet (UTF-8 BOM stripping, UTF-16, binary-file
  detection — PROCESS.md §3.2). `lineio.Reader` treats all input as raw
  bytes; a UTF-8 BOM would currently show up as leading garbage on the
  first line's `Text`, and a UTF-16 file would read as (mostly) blank
  lines full of NUL bytes. Both are the next step, not yet started.

## SEE ALSO

- [PROCESS.md](PROCESS.md) — full design and build plan this project follows
- `grep(1)`, `regexp/syntax` (Go's RE2 syntax reference:
  <https://github.com/google/re2/wiki/Syntax>)
