# PROCESS.md — Building `gg`, a cross-platform `grep` in Go

Scope: a `grep`-compatible CLI, **Windows-first**, later hardened for Linux/macOS.
Binary name: `gg` (matches this module's folder). Go toolchain: 1.26+.

This document is the working plan. Update it as decisions are made — treat
it as a living log, not a spec written once and ignored.

---

## 0. Guiding principles

1. **Correctness before speed.** Get GNU-grep-compatible behavior right on
   Windows first; optimize once there's a passing test suite to protect.
2. **Windows is not "Linux with different paths."** Line endings, console
   I/O, encoding, and shell globbing all differ structurally. Each gets its
   own explicit design decision below — no "should just work" assumptions.
3. **Thin `cmd/`, fat `internal/`.** Cobra commands only parse flags and
   call into `internal/` packages. `RunE` returns errors; no `os.Exit`
   buried in business logic — only `main.go` decides the process exit code.
4. **Streaming, not slurping.** Never read a whole file into memory to grep
   it (breaks on large files, defeats early-exit for `-l`/`-q`/`-m`).
5. **Test against real grep behavior**, not intuition. Use Git Bash / WSL
   grep or ripgrep as an oracle for edge cases (empty files, no trailing
   newline, binary files, multiple patterns).

---

## 1. Decide the feature surface (write this down before coding)

Pick a target flag set up front so the architecture (esp. the matcher and
printer interfaces) doesn't need reshaping later. Suggested MVP → stretch
ordering:

**MVP (v0.1):**
- Positional `PATTERN [FILE...]`, stdin when no files or `-`
- `-i` ignore case, `-v` invert match, `-n` line numbers, `-c` count
- `-F` fixed string (literal), default pattern = RE2 regex (Go `regexp`)
- `-l` / `-L` files-with(out)-matches, `-q` quiet (exit code only)
- Exit codes matching grep: `0` match found, `1` no match, `2` error

**v0.2 — multi-file & recursion:**
- Multiple files, `-r` / `-R` recursive directory walk
- `--include` / `--exclude` glob filters, `.gitignore`-style `--exclude-dir`
- Print `filename:` prefix automatically when >1 file or recursive

**v0.3 — match shaping:**
- `-w` word boundary, `-x` whole line, `-o` only-matching
- `-A NUM` / `-B NUM` / `-C NUM` context lines with `--` group separators
- `-m NUM` max matches per file (enables early exit)
- `-e PATTERN` (repeatable), `-f FILE` (patterns from file)

**v0.4 — output polish:**
- Color output (`--color=auto|always|never`)
- `-H` / `-h` force/suppress filename, `-z`/NUL-separated lines (stretch)
- `-E`/`-G`/`-P` engine selection (Go `regexp` is RE2 ~ ERE; document PCRE
  gaps — backreferences/lookaround are **not** supported, call this out
  in `--help` rather than silently mismatching)

**Explicit non-goals for v1:** true PCRE (backrefs/lookaround), `-P`
via cgo/onig, multi-byte locale collation beyond UTF-8/ASCII.

---

## 2. Project layout

```
gg/
  cmd/gg/main.go            # entrypoint: build info, calls cmd.Execute(), os.Exit
  internal/cli/             # Cobra command tree, flag definitions -> Options struct
  internal/grep/            # core engine: orchestrates matcher+walker+reader+printer
  internal/matcher/         # Matcher interface: regex, fixed-string, word/line-anchored
  internal/walker/          # file/dir traversal, include/exclude glob filtering
  internal/lineio/          # line-splitting reader: CRLF/LF/CR, no-trailing-newline
  internal/encoding/        # BOM detection/stripping, UTF-16->UTF-8 transcode, binary sniff
  internal/output/          # printer: plain/color, context buffering, filename prefixing
  internal/termenv/         # windows console mode (ANSI enable), isatty detection
  internal/testutil/        # golden-file helpers, oracle-comparison helpers
  test/testdata/            # fixture files: crlf.txt, lf.txt, no-trailing-newline.txt,
                             # utf8-bom.txt, utf16le.txt, binary.bin, empty.txt
  test/e2e/                 # black-box tests invoking the built binary
  PROCESS.md
  go.mod
```

Rationale: `internal/grep` is the composition root so `matcher`, `walker`,
`lineio`, and `output` stay independently unit-testable and swappable
(e.g., add a PCRE matcher later without touching the walker).

---

## 3. Windows-first correctness checklist

This is the section that differentiates "grep that happens to compile on
Windows" from "grep that behaves correctly on Windows." Do these **before**
chasing feature parity or performance.

### 3.1 Line endings
- Do not split on `\n` alone and call it done. Detect and handle `\r\n`,
  `\n`, and bare `\r` (old Mac / some tool output) per line, and preserve
  whether a trailing newline existed on the last line.
  **Decided:** `internal/lineio.Reader` tracks this per line as
  `Terminator` (`LF`/`CRLF`/`CR`/`NoTerminator`) purely as a fact about
  the source — it is not used to decide output formatting (see below),
  and gg does not replicate `diff`'s "\ No newline at end of file"-style
  warning; that's a `diff` behavior, not something grep itself does, and
  it isn't a goal here. Implemented and tested — see `internal/lineio`.
- `-o`/normal output: strip the line terminator before printing, then
  emit a terminator on the way out.
  **Decided:** gg always emits a bare `\n` after every printed line —
  matched lines, context lines, everything — regardless of the *source*
  line's terminator (`CRLF`/`CR`/`NoTerminator` all normalize to `\n` on
  output) and regardless of platform. Rationale: pipe-friendliness
  (composing with other tools) matters more than mirroring the input
  file's convention, this matches GNU grep's own real-world behavior,
  and it's simpler — one code path, no OS branch. This also means: no
  `\r\n` is ever written, even on Windows, and even for a source line
  that had no trailing newline at all. Not yet implemented — this is a
  decision for `internal/output` (§4 step 6) to apply; recorded here now
  so that package doesn't have to re-derive it.
- Build a `test/testdata/crlf.txt` and `lf.txt` with identical content and
  assert identical match output modulo the terminator question above.
  **Done at the `lineio` level**: `crlf.txt`/`lf.txt`/
  `no-trailing-newline.txt`/`cr.txt`/`empty.txt` exist under
  `test/testdata/` and `internal/lineio`'s tests assert `crlf.txt` and
  `lf.txt` produce byte-identical line *content*. The full "identical
  match *output*" assertion (through matcher + output formatting) is
  still pending on `internal/grep`/`internal/output` (§4 steps 4 and 6).

### 3.2 Encoding & BOM
- UTF-8 is the baseline. Explicitly decide behavior for a UTF-8 BOM
  (strip it before matching so `^pattern` doesn't silently fail to anchor
  on line 1 — this is a classic bug).
- UTF-16 LE/BE files exist on Windows (PowerShell `Out-File` defaults to
  UTF-16LE historically in Windows PowerShell 5.1, UTF-8 in PowerShell 7+).
  Decide: detect UTF-16 BOM and transcode via `golang.org/x/text/encoding`,
  or explicitly document "UTF-8 only, treat UTF-16 as binary" like most
  greps do by default. Either is fine — **pick one and write a test that
  proves the chosen behavior**, don't leave it implicit.
- Binary detection: replicate grep's heuristic (presence of a NUL byte in
  the first chunk ⇒ treat as binary, skip with a `Binary file X matches`
  message unless `-a`/`--text` or `-I` given).

### 3.3 Console / terminal behavior
- Legacy `cmd.exe` and old Windows PowerShell consoles historically did
  not interpret ANSI color codes; Windows 10+ consoles support virtual
  terminal sequences but the mode must be enabled via
  `SetConsoleMode` + `ENABLE_VIRTUAL_TERMINAL_PROCESSING`.
  **Decided:** `golang.org/x/sys/windows` directly (`GetConsoleMode`/
  `SetConsoleMode`), not a wrapper library — it's a handful of lines
  against a stable, documented Win32 API, and go-colorable/termenv's
  main value (ANSI-to-Win32-API translation for pre-Windows-10 consoles
  that can't interpret VT sequences at all) isn't a target for this
  project. Implemented as `internal/termenv.EnableANSI`, split by build
  tag (`ansi_windows.go` / `ansi_other.go`; non-Windows is a no-op
  reporting success, since those terminals already handle ANSI
  natively).
- Detect "is stdout a terminal" correctly on Windows (a real console vs.
  redirected to a file/pipe) before defaulting `--color=auto` to on.
  **Decided:** `golang.org/x/term.IsTerminal`, wrapped as
  `internal/termenv.IsTerminal(*os.File) bool`, instead of hand-rolling
  `isatty`. Detection only — `internal/termenv` does not decide
  `--color` policy; that's for the future output layer (§4 step 7) to
  combine with the flag value.
- Windows Terminal / ConPTY vs. legacy conhost: test in both if possible;
  behavior differs for redirected output and ANSI passthrough.
  **Not fully verified**: unit tests cover regular files and pipes
  (both correctly report non-terminal), but a genuine interactive
  console can't be constructed inside an automated `go test` run in
  this environment — manual verification on both conhost and Windows
  Terminal is still an open item, tracked in README's LIMITATIONS.

### 3.4 Paths and globbing
- **PowerShell and cmd.exe do not expand globs before invoking the
  process** (unlike POSIX shells). If `gg` is to support `gg pattern *.go`,
  `gg` itself must expand the glob for arguments that look like patterns
  and didn't match a literal file — implement this in `internal/walker`,
  not by relying on shell behavior.
  **Decided:** `filepath.Glob` (no `**` support via doublestar for v1 —
  not a stated goal yet). Implemented as `walker.ExpandGlobs`: a literal
  existing path always wins over glob interpretation (so a real file
  named `a[1].txt` isn't glob-mangled), and a pattern with zero matches
  passes through unchanged so a later "no such file" error surfaces
  naturally — mirroring a POSIX shell's own non-nullglob behavior.
  Unit-tested standalone from the Cobra layer, per plan.
- Normalize path separators for display: accept both `/` and `\` on
  input, but be deliberate about what gets printed (grep prints paths as
  given/discovered — on Windows that means backslashes from
  `filepath.Walk`/`fs.WalkDir`, which is correct and expected; don't
  force-convert to `/`).
  **Decided:** do nothing — `walker.Walk` builds every path via
  `filepath.Join`, which already uses the OS-native separator and
  preserves the root's own relative/absolute form. Verified with a test
  asserting the native separator is actually present in a result.
- Case-insensitive filesystem: Windows paths are case-insensitive but
  case-preserving. This affects `--include`/`--exclude` glob matching
  (decide: match case-insensitively on Windows, case-sensitively on
  Unix — mirror how the native tools on each OS behave) and de-duplication
  of file arguments.
  **Decided:** case-insensitive on Windows (`runtime.GOOS == "windows"`),
  case-sensitive elsewhere, implemented in `walker.matchOneCaseAware`
  with the case-folding logic separated from the OS check so both
  branches are directly unit-tested on one CI runner rather than relying
  on platform-specific build tags for what's otherwise identical logic.
  File-argument de-duplication is not yet implemented — deferred until
  something actually needs it (no observed bug motivating it yet).
- Long paths / `\\?\` prefix: if recursive search may hit paths >260
  chars, decide whether to opt into Go's long-path handling (Go 1.24+ on
  Windows generally handles this via `GetFullPathNameW`/manifest — verify
  on the installed toolchain rather than assuming) and add a regression
  test with a deeply nested testdata directory if this matters for target
  users.
  **Verified, not a decision**: Go 1.26 on this Windows toolchain handles
  paths >260 characters transparently with zero special handling — no
  `\\?\` prefix, no manifest change, nothing. `TestWalk_LongPath` builds
  a >300-character nested path and confirms it's walked correctly. If a
  future Go upgrade or a different toolchain ever regresses this, that
  test will catch it.
- Symlinks/junctions/reparse points: Windows has directory junctions and
  symlinks with different creation privileges. Decide the recursion
  policy (default: don't follow symlinks, add `--follow` later) up front
  so `internal/walker` has one clear contract.
  **Decided:** default `FollowSymlinks: false` — any symlink
  *encountered while recursing* (to a file or a directory) is skipped
  entirely, neither read nor descended into. A root passed directly to
  `Walk` is always dereferenced regardless of this setting (matching the
  "explicit arguments are always followed" convention, e.g. `cat
  somelink`). `FollowSymlinks: true` is implemented now (not deferred to
  a later `--follow` flag's implementation — only the CLI flag itself is
  deferred) with cycle detection via `filepath.EvalSymlinks`-canonicalized
  visited-directory tracking, tested with an actual symlink cycle under
  a 5-second hang guard.
  **Caveat, not fully verified**: Windows directory junctions are
  reparse points, not symlinks, and whether Go's `os` package reports
  them with `fs.ModeSymlink` (so this same logic applies to them too) is
  believed true on modern Go but was not specifically tested here — no
  junction was created in a test (that needs `mklink /J` or raw
  syscalls, not `os.Symlink`). Flagged rather than assumed correct.

### 3.5 Process/exit semantics
- Exit codes must be `0`/`1`/`2` regardless of OS — verify Cobra isn't
  swallowing this (don't let a Cobra-level error print + exit 1 mask a
  "no matches" vs "real error" distinction).
  **Verified, and now regression-tested**: this contract was only
  smoke-tested manually back at scaffolding time (step 1) — which is
  exactly how the `--version`/`--help` incorrectly-exiting-2 bug was
  found and fixed then. It had zero automated coverage since. Closed
  now: `internal/cli` gained a `runGrep` seam (a swappable package var,
  restored after each test) so `TestExecute_PropagatesExitCodeFromRunGrep`
  can drive all three codes without a real search engine existing yet,
  plus the original `--version`/`--help`/no-args cases are now
  permanent tests instead of one-off manual checks.
- `Ctrl+C` (SIGINT) handling on Windows goes through
  `os/signal` + `syscall.SIGINT`, which Go maps from `CTRL_C_EVENT` —
  verify graceful shutdown (flush buffered output) works under
  `go build` binaries run directly and via `go run` (behaves differently
  on Windows due to the intermediary process).
  **Decided and wired, ahead of there being real work to cancel**:
  `main.go` derives a `context.Context` from
  `signal.NotifyContext(context.Background(), os.Interrupt)` — the same
  API and the same `os.Interrupt` value handles both Unix SIGINT and
  Windows' `CTRL_C_EVENT` translation, no OS branch needed. This is
  threaded through `cli.Execute(ctx, args)` → Cobra's
  `ExecuteContext`/`cmd.Context()` → `grep.Run(ctx, opts)`, and `Run`
  already checks `ctx.Err()` eagerly (tested) even though it has nothing
  to flush yet — the point is that the walker/file-reading loop landing
  in step 4+ doesn't need a breaking signature change to gain
  cancellation. **Not yet verified**: actually flushing buffered output
  gracefully mid-search under `go build` vs. `go run` on Windows, since
  there's no buffered output (or search) to flush yet — deferred until
  the output layer exists.

### 3.6 File reading specifics
- Opening files: no `O_NONBLOCK`-style POSIX assumptions; Windows file
  locking semantics differ (a file opened for read by another process
  without `FILE_SHARE_DELETE` etc. can behave differently under
  concurrent writes) — if grepping log files that are actively written,
  test that behavior explicitly rather than assuming POSIX semantics.
  **Found a real gap, and fixed it**: empirically verified (not
  assumed) against this exact Go version — plain `os.Open` on Windows
  sets `FILE_SHARE_READ|FILE_SHARE_WRITE` but *not*
  `FILE_SHARE_DELETE` (confirmed in `src/syscall/syscall_windows.go`).
  Concretely: with plain `os.Open`, a concurrent write while gg holds a
  file open succeeds, but a concurrent rename or delete — exactly what
  typical log rotation does — fails with "The process cannot access the
  file because it is being used by another process." POSIX has no such
  restriction. Fixed in `internal/source.Open`: on Windows, files are
  opened via `golang.org/x/sys/windows.CreateFile` directly with
  `FILE_SHARE_DELETE` added, matching POSIX's more permissive
  unlink/rename-while-open semantics. Regression-tested directly
  (`TestOpen_AllowsDeleteWhileOpen`, `TestOpen_AllowsRenameWhileOpen`),
  including that the still-open handle keeps reading the original
  content after the concurrent delete
  (`TestOpen_StillReadableAfterConcurrentDelete`).
- `CON`, `NUL`, `PRN`, `COM1`... are reserved device names on Windows —
  decide how `internal/walker` handles a literal file argument that
  collides with a reserved name (should basically never come up from
  real directory walks, but a user could pass `gg pattern NUL`).
  **Decided, verified empirically, no special-casing needed**:
  `os.Stat("NUL")` succeeds (reports a non-directory character device,
  size 0) and `os.Open("NUL")` + read yields immediate EOF with no
  error — the same harmless outcome Unix grep gets from `/dev/null`.
  Neither `internal/walker` nor `internal/source` special-cases reserved
  names at all; the OS already does the right thing. Verified with
  `TestOpen_NULDevice` (Windows-only). `CON`/`PRN`/`COMn` were
  deliberately **not** live-tested — `CON` in particular would open the
  real console and could block waiting for keyboard input inside an
  automated test with no console attached, which is not a risk worth
  taking just to confirm a rarely-hit path; the same "let the OS handle
  it, don't special-case" reasoning is expected to apply but is flagged
  as reasoned-not-tested rather than claimed verified.

---

## 4. Build order (do these in sequence, commit/test at each step)

Status legend: ✅ done · 🚧 partially done · ⬜ not started.

1. ✅ **Scaffold**: `go mod init`, Cobra root command, `--version` wired to
   build-time `ldflags`, empty `internal/grep.Run(opts) (exitCode int, err error)`.
2. ✅ **Matcher interface + regex/fixed-string impls**, unit tests only
   (no I/O yet): `Matcher.Find(line []byte) []MatchRange`.
3. ✅ **`lineio` reader**: streaming line splitter over `io.Reader` handling
   CRLF/LF/CR and no-trailing-newline, with table-driven tests using the
   Windows-correctness fixtures from §3.1 before touching any CLI code.
4. ✅ **Wire MVP**: stdin + single file, `-i -v -n -c -F -l -L -q`, exit codes.
   Added `test/e2e` invoking the built binary against `test/testdata`.
   Done together with steps 5 and 7 (below) in one wiring pass, once
   §3's foundational packages (`matcher`/`lineio`/`encoding`/`walker`/
   `source`/`termenv`) already existed and were independently tested —
   the sequential order this list originally proposed turned out less
   important than having those pieces solid first.
5. ✅ **Walker**: recursive dir traversal + include/exclude globs, tested
   standalone against a synthetic directory tree (`t.TempDir()`), *then*
   wired into the CLI.
6. 🚧 **`-o`/`-w`/`-x`, context lines (`-A`/`-B`/`-C`), and multi-pattern
   (`-e`/`-f`) done and wired. `-m` not started** — see README
   LIMITATIONS. Context lines are implemented as a streaming
   `contextTracker` (`internal/grep/context.go`): a capped ring buffer
   of recent non-selected lines plus a trailing-context countdown, so no
   whole-file buffering is needed regardless of window size. Every
   behavioral rule — merge-without-separator when windows overlap,
   `-A0`/`-C0` still enabling `--` separators despite printing zero
   context text, `-A`/`-B` winning over `-C` for their own direction
   regardless of flag order, the separator firing between different
   files' output (not just within one file), `-o` keeping the separator
   but dropping context text, `-v` letting an excluded-because-it-
   matched line resurface as highlighted context — was verified directly
   against real GNU grep (Git for Windows' `grep.exe`), not assumed from
   the man page, and is regression-tested at all three levels
   (`internal/output`, `internal/grep`, `internal/cli`) plus `test/e2e`.
   `-e`/`-f` widened `matcher.Options.Pattern` (a single string) into
   `Patterns []string`, combined into one alternation and compiled as a
   single regexp rather than running multiple matchers per line;
   `internal/grep.resolvePatterns` handles `-f` file reading (via
   `internal/source` + `internal/lineio`, the same Windows-correct
   opening and line-splitting used for searched files, not a second
   implementation) and combination order. `internal/cli` resolves the
   "positional `PATTERN` vs. `-e`/`-f`" precedence — verified directly
   against real GNU grep: once either is given, every positional
   argument becomes a `FILE`, and zero positional arguments becomes
   valid (a bare `PATTERN` is no longer required). Also verified and
   replicated: a blank line in a `-f` file becomes a literal
   empty-pattern (matches every line, like `-e ""`), and supplying zero
   patterns total matches *nothing* (not "matches everything," which is
   what an actually-empty regex would do) — `matcher.New`'s `neverMatcher`
   exists specifically for that case.
7. ✅ **Color + terminal detection** (`internal/termenv`), wired via
   `internal/grep.resolveColor`. `--color=auto` verified for
   redirected-to-file/pipe (correctly off); **real-console verification
   (Windows Terminal vs. legacy conhost) is still an open manual-testing
   item** — see README LIMITATIONS, same caveat as §3.3.
8. ✅ **Performance pass**: buffered I/O sizing, avoid per-line allocations
   in the hot path (reuse scanner buffers), add `go test -bench` targets
   over a generated large fixture; profile with `pprof` before optimizing
   blindly. Benchmarks live alongside the packages they measure
   (`internal/lineio/bench_test.go`, `internal/matcher/bench_test.go`,
   `internal/grep/bench_test.go`) so they keep running as regression
   guards, not one-off scratch scripts. See
   [BENCHMARK.md](BENCHMARK.md) for a full before/after comparison
   against the pre-performance-pass branch (`benchstat`-backed, not
   eyeballed), including the two rejected intermediate designs.
   - **`internal/lineio.Reader.ReadLine` — the actual hot path, found by
     profiling, not guessed.** The original implementation scanned
     byte-by-byte via `bufio.Reader.ReadByte` into a `bytes.Buffer`
     (explicitly flagged as the thing to revisit, back when it was
     written — see the git history of `internal/lineio`'s package doc).
     `go tool pprof` on `BenchmarkReadLine_LF` confirmed it: `ReadByte`
     plus the matching byte-by-byte `bytes.Buffer.WriteByte`
     accumulation accounted for ~65% of `ReadLine`'s CPU time. Rewritten
     to bulk-scan whatever's already sitting in bufio's internal buffer
     — via `bytes.IndexByte`, one bounded pass per terminator instead of
     a per-byte loop — and to return a slice aliasing that buffer
     directly (zero-copy) whenever a line doesn't span more than one
     buffered chunk, which is the overwhelming common case. Measured
     effect (`go test -benchmem`, this machine, 50,000-line synthetic
     fixture): **~211 MB/s → ~3.6 GB/s on LF input (~17×), ~227 MB/s →
     ~3.35 GB/s on CRLF (~15×)**, with the `Reset`-and-reuse path
     (`BenchmarkReadLine_Reuse`, the shape `internal/grep` actually
     uses across a multi-file walk) down to a single small allocation
     per `b.N` iteration — effectively zero per-line allocation in
     steady state.
   - **A real correctness-of-performance bug was found and fixed
     mid-optimization, not just the headline win**: an intermediate
     version used two independent, unbounded `bytes.IndexByte` scans
     (one for `'\r'`, one for `'\n'`) and picked whichever came first.
     This is *actively worse* than the original byte-by-byte code for
     pure-LF input specifically — `IndexByte` searching for a `'\r'`
     that never appears anywhere in a stream can't stop early, so it
     re-scans the entire remaining buffered chunk (tens of KB) on
     *every single line*, which is quadratic in the number of lines per
     buffer-full. Caught by benchmarking before shipping it, not by
     inspection — `BenchmarkReadLine_LF` regressed to ~105 MB/s, *worse*
     than the byte-by-byte baseline. Fixed by finding `'\n'` first (the
     common terminator, so normally a short scan) and bounding the
     `'\r'` search to only the region before it — but that swap
     reintroduced the identical failure mode in the opposite direction
     for bare-CR-only content (classic Mac OS9 files with no `'\n'`
     anywhere at all), so the scan window is additionally capped to a
     fixed 4 KiB (`scanWindow` in `internal/lineio/reader.go`) regardless
     of which terminator, if either, is actually present. Both failure
     modes now have permanent regression benchmarks
     (`BenchmarkReadLine_LF` and the dedicated `BenchmarkReadLine_BareCR`
     added specifically to pin this down), not just the fixed-and-moved-on
     code. **Accepted, documented tradeoff**: bare-CR-only input still
     benchmarks slower than LF/CRLF (~820 MB/s vs. ~3.3–3.6 GB/s) — two
     bounded `IndexByte` scans per 4 KiB window instead of one, since
     `'\n'` genuinely never being found means the `'\r'` search can't be
     narrowed by it. Not fixed further: bare CR is already the
     documented rare/legacy case elsewhere in this file (§3.1), and
     correctness (all existing table/fixture/fuzz tests, including
     `cr.txt`) is unaffected either way.
   - **Eliminated accidental double-buffering** between
     `internal/encoding.Sniff` and `internal/lineio.NewReader`: `Sniff`
     previously returned a `*bufio.Reader` sized only for its own
     8000-byte sniff peek, which `lineio.NewReader` then wrapped in a
     *second*, independent 64 KiB `bufio.Reader` — capping every
     downstream fill at whatever the inner 8000-byte buffer's `Read`
     returned per call instead of one larger read. Fixed on both ends:
     `encoding.Sniff`'s buffer capacity is now sized to match
     `lineio.DefaultBufSize` (64 KiB) — `sniffLen` itself, the amount
     actually peeked for BOM/binary detection, is unchanged — and
     `lineio.NewReader` recognizes an already-adequately-buffered
     `*bufio.Reader` input and reuses it directly instead of wrapping it
     again. Both sides are regression-tested directly, not just
     inferred from throughput numbers:
     `TestNewReader_ReusesAlreadyBufferedReader` /
     `TestNewReader_WrapsUndersizedBufferedReader` in `internal/lineio`,
     and `TestSniffThenLineio_NoDoubleBuffering` in `internal/encoding`
     (the actual real composition point) confirm reuse by comparing
     `Buffered()` counts, not just checking the code compiles.
   - **`internal/matcher` was already close to optimal** — profiled, not
     assumed. Both `fixedMatcher.Find` and `regexMatcher.Find` allocate
     **zero** bytes on the no-match case (`BenchmarkFixedMatcher_Find_NoMatch`,
     `BenchmarkRegexMatcher_Find_NoMatch`), which matters because most
     lines in a real search don't match — that's the actual hot path,
     not the match case. The match case's small allocations (one
     `[]MatchRange` per matching line, ~16–270 B depending on engine)
     are unavoidable output data, not waste, and weren't touched.
   - **End-to-end benchmarks** (`internal/grep/bench_test.go`) exercise
     the full `source → encoding.Sniff → lineio → matcher → output`
     pipeline against a generated 100,000-line synthetic log fixture:
     `BenchmarkRun_Regex_SparseMatches` (~520 MB/s), `BenchmarkRun_
     Fixed_SparseMatches` (~930 MB/s), and `BenchmarkRun_Quiet_
     FirstLineMatches` (effectively O(1) in file size via the existing
     `-q` early-exit — not a new optimization, but now a permanent
     benchmark proving that early exit stays cheap rather than
     silently regressing). `pprof` on the regex benchmark shows the
     remaining time split between real disk I/O (`ReadFile` via
     `internal/source`) and Go's own `regexp` engine (`allMatches`/
     `backtrack`, ~57% cumulative) — neither is this project's code to
     optimize further without replacing the RE2 engine, which is out of
     scope.
   - **Discovered, fixed, and unrelated to the performance work itself**:
     verifying `internal/lineio`'s existing CRLF/LF fixture-comparison
     test on this machine surfaced that `core.autocrlf=true` (a common
     Windows Git default) was silently rewriting `test/testdata/lf.txt`'s
     `LF` bytes to `CRLF` on checkout, and — worse — that `crlf.txt`'s
     *actual git blob* had lost its `CRLF` bytes entirely at some
     previous commit (autocrlf normalizes to `LF` for storage on
     commit too), meaning `crlf.txt` only ever "worked" by accident, by
     being re-mangled back to `CRLF` on checkout on machines with the
     same setting — and would silently degrade to plain `LF` content,
     defeating the fixture's entire purpose, on any checkout with
     `autocrlf=false`/`input` (the git/CI default on Linux and macOS
     runners, PROCESS.md §6). Fixed with `test/testdata/.gitattributes`
     (`* -text`, disabling any of git's line-ending normalization for
     that directory regardless of a contributor's global config) and by
     restoring `crlf.txt`'s actual `CRLF` bytes in the working tree and
     re-committing them under the new attribute. Flagged here rather
     than folded silently into the performance-pass diff, per this
     document's own rule about not hiding what was found.
9. ⬜ **Cross-platform pass**: run the full suite on Linux/macOS (CI matrix,
   §6), fix anything Windows-specific that leaked into shared code paths
   (e.g., accidental `\r\n` assumptions). Not started; this repo has no
   CI configured yet at all.
10. **Packaging** (§7).

Do not skip ahead to performance or Linux support before step 4 passes
its e2e tests on Windows — the point of "Windows first" is that the
line-ending/encoding/console decisions in §3 are foundational and
expensive to retrofit.

---

## 5. Testing strategy

- **Unit tests** per package (`matcher`, `lineio`, `walker`, `output`) —
  no CLI or filesystem-walking dependencies where avoidable; use
  `strings.Reader`/`t.TempDir()`.
- **Golden-file tests**: for a matrix of (pattern, flags, fixture file),
  store expected stdout+exit-code, compare byte-for-byte. Cover every
  fixture in §3.1/§3.2 (`crlf.txt`, `lf.txt`, `utf8-bom.txt`,
  `no-trailing-newline.txt`, `binary.bin`, `empty.txt`).
- **Oracle comparison** (manual/CI-optional): where Git Bash or WSL grep
  is available, script a comparison of `gg` vs `grep` output for a corpus
  of patterns/flags to catch behavioral drift; don't hard-depend CI on
  this since it won't run on a plain `windows-latest` runner without WSL.
- **E2E tests** invoke the compiled binary (`exec.Command`) for real
  process/exit-code/stdio-redirection behavior — this is the only layer
  that actually validates §3.3/§3.5.
- **Fuzz** the matcher and `lineio` splitter (`go test -fuzz`) — regex
  compilation and line-splitting on adversarial byte sequences are prone
  to panics if not careful with slicing.

---

## 6. CI (GitHub Actions matrix)

- Jobs: `windows-latest`, `ubuntu-latest`, `macos-latest` × current Go.
- `go vet`, `staticcheck`/`golangci-lint`, `go test ./... -race`.
- Windows job additionally runs the e2e suite with both a real console
  (if feasible) and piped/redirected stdio to exercise §3.3.
- Fail the build on any golden-file mismatch — these are the contract.

---

## 7. Packaging & distribution

- `goreleaser` for GOOS/GOARCH matrix builds (`windows/amd64`,
  `windows/arm64`, `linux/amd64`, `darwin/arm64`, etc.), embedding
  version/commit via `-ldflags`.
- Windows distribution: publish a `winget` manifest and/or a `scoop`
  bucket entry so `gg` installs like a native tool; ship both an `.exe`
  zip and (optionally) an MSI later — start with the zip, it's
  near-zero-cost via goreleaser.
- Decide the binary/command name collision story: `gg` may collide with
  other tools users have on PATH — confirm before locking the name into
  packaging manifests.

---

## 8. Open decisions log

Fill in as choices are made; each row should reference the section above.

| Decision | Choice | Section |
|---|---|---|
| Regex engine | Go `regexp` (RE2) only for v1? | §1 |
| UTF-16 handling | **Decided:** detect via BOM (LE/BE) and transcode to UTF-8 via `golang.org/x/text/encoding/unicode`, streaming (no whole-file read). No-BOM UTF-16 is undetectable by design (no reliable signature) and falls through to the plain NUL-byte binary heuristic — this is a known, accepted gap, not an oversight. `internal/encoding.Sniff`. | §3.2 |
| Binary-detection heuristic | **Decided:** NUL byte anywhere in an 8000-byte prefix (mirrors Git's `buffer_is_binary`), skipped entirely (forced `Binary: false`) when a UTF-16 BOM was positively identified, since UTF-16 ASCII text is legitimately full of zero bytes. Detection only — the skip/`--text` *policy* is deferred to the engine wiring step. `internal/encoding.Sniff` | §3.2 |
| Trailing newline emission | **Decided:** always `\n` on output, regardless of source terminator or platform — never `\r\n`, never omitted, no `diff`-style "no newline" warning. Not yet implemented (belongs to `internal/output`, §4 step 6); recorded ahead of that package existing. | §3.1 |
| Symlink recursion default | **Decided:** don't follow (default `FollowSymlinks: false`); explicit roots always dereferenced regardless. `internal/walker.Walk` | §3.4 |
| Color library | **Decided:** `golang.org/x/term.IsTerminal` for detection + `golang.org/x/sys/windows` `SetConsoleMode` directly for enabling ANSI on Windows — no wrapper library. `internal/termenv` | §3.3 |
| Long path support | **Verified:** Go 1.26 on this Windows toolchain handles paths >260 chars transparently, no `\\?\` prefix or manifest needed. `TestWalk_LongPath` | §3.4 |
| Exit-code contract test coverage | **Decided:** add a swappable `runGrep` package var in `internal/cli` purely for testability, so the 0/1/2 contract has permanent regression tests instead of the one-off manual verification that originally caught the `--version`/`--help` exit-2 bug. `internal/cli.runGrep` | §3.5 |
| Ctrl+C / graceful shutdown | **Decided:** `signal.NotifyContext(context.Background(), os.Interrupt)` in `main.go`, threaded via Cobra's `ExecuteContext`/`cmd.Context()` into `grep.Run(ctx, opts)`. Wired ahead of there being real work to cancel, specifically to avoid a breaking signature change later. `cmd/gg/main.go`, `internal/grep.Run` | §3.5 |
| Windows file-sharing semantics | **Decided (and a real bug found+fixed):** plain `os.Open` on Windows omits `FILE_SHARE_DELETE` (confirmed against Go 1.26's own source and empirically), breaking concurrent rename/delete — e.g. log rotation — while gg holds a file open. `internal/source.Open` opens via `x/sys/windows.CreateFile` with `FILE_SHARE_DELETE` added on Windows. `internal/source` | §3.6 |
| Reserved device names (`NUL`, `CON`, ...) | **Decided:** no special-casing — verified `os.Stat`/`os.Open` on `NUL` already behave harmlessly (empty read, no error), matching Unix `/dev/null` semantics. `CON`/`PRN`/`COMn` reasoned-but-not-live-tested (opening `CON` risks blocking on keyboard input with no console attached). | §3.6 |
| `lineio.ReadLine` scanning strategy | **Decided, and measured, not guessed:** bulk `bytes.IndexByte` scan of whatever's already buffered (with a fixed 4 KiB `scanWindow` cap to keep the cost bounded regardless of which terminator byte, if any, is actually present), zero-copy alias of bufio's own buffer for the single-chunk common case. ~15–17× throughput over the original byte-by-byte scan; two intermediate designs were tried and rejected specifically because benchmarking (not inspection) caught them regressing pure-LF or pure-bare-CR input to quadratic behavior. `internal/lineio.Reader.ReadLine` | §4 step 8 |
| `Sniff`/`lineio` buffer sizing | **Decided:** `encoding.Sniff`'s buffer capacity matches `lineio.DefaultBufSize` (64 KiB) so `lineio.NewReader` can detect and reuse it directly instead of double-buffering; `sniffLen` (the 8000-byte peek used for BOM/binary detection) is unchanged. `internal/encoding.Sniff`, `internal/lineio.NewReader` | §4 step 8 |
| `test/testdata` line-ending fixtures | **Bug found and fixed, not just a decision:** `core.autocrlf=true` (common on Windows) was silently rewriting `lf.txt` on checkout, and `crlf.txt`'s committed blob had already lost its actual `CRLF` bytes to the same setting at some prior commit — meaning `crlf.txt` only "worked" by accident on machines that happened to share that setting, and would silently degrade to plain `LF` on any checkout without it (e.g. the Linux/macOS CI runners in §6). Fixed with `test/testdata/.gitattributes` (`* -text`) plus restoring `crlf.txt`'s real bytes. | §3.1, §4 step 8 |
| `-A`/`-B`/`-C` precedence when combined | **Verified against real GNU grep, not assumed:** an explicitly-given `-A`/`-B` always wins for its own direction over `-C`, order-independent (`-A 1 -C 3` and `-C 3 -A 1` both resolve to `after=1, before=3`) — not the "last flag on the command line wins" behavior pflag defaults to for overlapping scalar flags. Resolved in `internal/cli`'s `RunE` via `cmd.Flags().Changed(...)`, so `internal/grep.Options` always receives already-resolved `ContextBefore`/`ContextAfter` ints. | §1, §4 step 6 |
| `-A0`/`-B0`/`-C0` still enabling group separators | **Verified against real GNU grep, not assumed:** explicitly passing e.g. `-A0` still turns on `--` separators between non-adjacent matches, even though zero context lines print — genuinely different, observable output from omitting `-A`/`-B`/`-C` entirely (which never separates). `internal/grep.Options.ContextEnabled` carries this as a distinct bool from the resolved before/after values specifically to preserve the distinction. | §4 step 6 |
| Context separator scope: per-file or per-run | **Verified against real GNU grep, not assumed:** the `--` separator fires between two *different files*' output too, not just between two groups within one file — `internal/grep`'s `contextTracker` is deliberately one instance shared across an entire run (reset per-file via `startFile`, except its "has anything ever been printed" flag) rather than recreated per file. | §4 step 6 |
| Multi-pattern combination strategy | **Decided:** join every `-e`/`-f` pattern into one `p1\|p2\|...` alternation (escaping each with `regexp.QuoteMeta` first under `-F`) and compile it as a single regexp, rather than running `len(Patterns)` independent `Matcher`s per line. Simpler — no need to merge match ranges from multiple matchers into `Matcher`'s required ascending/non-overlapping order — and lets one `-x`/`-w`/`-i` wrap apply to the whole alternation at once, matching real grep. Each pattern is still validated individually first (`regexp.Compile`) purely for precise per-pattern error messages. `internal/matcher.New` | §1, §4 step 6 |
| `-e`/`-f` vs. positional `PATTERN` precedence | **Verified against real GNU grep, not assumed:** once *either* `-e` or `-f` is given, the positional `PATTERN` slot disappears entirely — every remaining positional argument becomes a `FILE`, even one that "looks like" a pattern; and zero positional arguments becomes valid (a bare `PATTERN` is no longer required at all). Resolved in `internal/cli`'s custom `Args` func and `RunE`, mirroring how `-A`/`-B`/`-C` precedence is resolved there rather than in `internal/grep`. `internal/cli/root.go` | §1, §4 step 6 |
| `-e`/`-f` combination order | **Decided (not an attempt at exact GNU parity):** every `-e`/positional pattern first, in the order given, then every `-f` file's lines, files in the order given and lines in file order — not true command-line interleaving between `-e` and `-f` occurrences, since regex alternation doesn't make that distinction externally observable anyway (which pattern in an OR "wins" for whether a line matches is order-independent). `internal/grep.resolvePatterns` | §4 step 6 |
| Zero total patterns from `-e`/`-f` | **Verified against real GNU grep, not assumed:** supplying zero patterns between `-e`/`-f` (e.g. an empty `-f` file and no `-e` at all) makes the search match *nothing* — the opposite of what compiling an actually-empty regex pattern would do (that zero-width-matches every line). `matcher.New` returns a dedicated `neverMatcher` for this case rather than special-casing it deeper in the engine. `internal/matcher.New` | §3.2 (empty-pattern precedent), §4 step 6 |
| Blank lines in `-f` pattern files | **Verified against real GNU grep, not assumed:** a blank line is not skipped — it becomes a literal empty-string pattern, which (like `-e ""`) matches every line. `internal/grep.readPatternFile` deliberately does not filter blank lines. | §4 step 6 |
| Missing/unreadable `-f` file | **Decided, consistent with the invalid-pattern precedent:** a `-f` file that can't be opened or read is a **fatal**, once-returned error (`internal/grep.Run` returns it directly), not a per-search-file diagnostic written to stderr and continued past — with no readable pattern list, there is nothing left to search with. `internal/grep.readPatternFile` | §4 step 6 |
| `-f` file reading implementation | **Decided:** read via `internal/source.Open` + `internal/lineio.NewReader` — the same Windows-correct file opening (including `"-"` → stdin) and CRLF/LF/CR-safe line splitting already used for files being searched — rather than a second, independent line-reading implementation (e.g. `bufio.Scanner`). `internal/grep.readPatternFile` | §4 step 6 |
