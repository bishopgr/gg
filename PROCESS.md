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
  `SetConsoleMode` + `ENABLE_VIRTUAL_TERMINAL_PROCESSING`. Use
  `golang.org/x/sys/windows` directly or a maintained wrapper
  (`github.com/mattn/go-colorable`, `github.com/muesli/termenv`) — decide
  which, add to `internal/termenv`, and gate all color output through it.
- Detect "is stdout a terminal" correctly on Windows (a real console vs.
  redirected to a file/pipe) before defaulting `--color=auto` to on;
  `golang.org/x/term.IsTerminal` handles this cross-platform — use it
  instead of hand-rolling `isatty`.
- Windows Terminal / ConPTY vs. legacy conhost: test in both if possible;
  behavior differs for redirected output and ANSI passthrough.

### 3.4 Paths and globbing
- **PowerShell and cmd.exe do not expand globs before invoking the
  process** (unlike POSIX shells). If `gg` is to support `gg pattern *.go`,
  `gg` itself must expand the glob for arguments that look like patterns
  and didn't match a literal file — implement this in `internal/walker`,
  not by relying on shell behavior. Use `filepath.Glob` or a richer
  matcher (`github.com/bmatcuk/doublestar` for `**`) and unit-test it
  standalone from the Cobra layer.
- Normalize path separators for display: accept both `/` and `\` on
  input, but be deliberate about what gets printed (grep prints paths as
  given/discovered — on Windows that means backslashes from
  `filepath.Walk`/`fs.WalkDir`, which is correct and expected; don't
  force-convert to `/`).
- Case-insensitive filesystem: Windows paths are case-insensitive but
  case-preserving. This affects `--include`/`--exclude` glob matching
  (decide: match case-insensitively on Windows, case-sensitively on
  Unix — mirror how the native tools on each OS behave) and de-duplication
  of file arguments.
- Long paths / `\\?\` prefix: if recursive search may hit paths >260
  chars, decide whether to opt into Go's long-path handling (Go 1.24+ on
  Windows generally handles this via `GetFullPathNameW`/manifest — verify
  on the installed toolchain rather than assuming) and add a regression
  test with a deeply nested testdata directory if this matters for target
  users.
- Symlinks/junctions/reparse points: Windows has directory junctions and
  symlinks with different creation privileges. Decide the recursion
  policy (default: don't follow symlinks, add `--follow` later) up front
  so `internal/walker` has one clear contract.

### 3.5 Process/exit semantics
- Exit codes must be `0`/`1`/`2` regardless of OS — verify Cobra isn't
  swallowing this (don't let a Cobra-level error print + exit 1 mask a
  "no matches" vs "real error" distinction).
- `Ctrl+C` (SIGINT) handling on Windows goes through
  `os/signal` + `syscall.SIGINT`, which Go maps from `CTRL_C_EVENT` —
  verify graceful shutdown (flush buffered output) works under
  `go build` binaries run directly and via `go run` (behaves differently
  on Windows due to the intermediary process).

### 3.6 File reading specifics
- Opening files: no `O_NONBLOCK`-style POSIX assumptions; Windows file
  locking semantics differ (a file opened for read by another process
  without `FILE_SHARE_DELETE` etc. can behave differently under
  concurrent writes) — if grepping log files that are actively written,
  test that behavior explicitly rather than assuming POSIX semantics.
- `CON`, `NUL`, `PRN`, `COM1`... are reserved device names on Windows —
  decide how `internal/walker` handles a literal file argument that
  collides with a reserved name (should basically never come up from
  real directory walks, but a user could pass `gg pattern NUL`).

---

## 4. Build order (do these in sequence, commit/test at each step)

1. **Scaffold**: `go mod init`, Cobra root command, `--version` wired to
   build-time `ldflags`, empty `internal/grep.Run(opts) (exitCode int, err error)`.
2. **Matcher interface + regex/fixed-string impls**, unit tests only
   (no I/O yet): `Matcher.Find(line []byte) []MatchRange`.
3. **`lineio` reader**: streaming line splitter over `io.Reader` handling
   CRLF/LF/CR and no-trailing-newline, with table-driven tests using the
   Windows-correctness fixtures from §3.1 before touching any CLI code.
4. **Wire MVP**: stdin + single file, `-i -v -n -c -F -l -L -q`, exit codes.
   Add `test/e2e` invoking the built binary against `test/testdata`.
5. **Walker**: recursive dir traversal + include/exclude globs, tested
   standalone against a synthetic directory tree (`t.TempDir()`), *then*
   wired into the CLI.
6. **Context lines + `-o`/`-w`/`-x`/`-m`/`-e`/`-f`** in the printer/engine.
7. **Color + terminal detection** (`internal/termenv`), default
   `--color=auto` behavior verified on: Windows Terminal, redirected-to-
   file, redirected-to-pipe (`gg pattern file | cat` should not emit
   color codes).
8. **Performance pass**: buffered I/O sizing, avoid per-line allocations
   in the hot path (reuse scanner buffers), add `go test -bench` targets
   over a generated large fixture; profile with `pprof` before optimizing
   blindly.
9. **Cross-platform pass**: run the full suite on Linux/macOS (CI matrix,
   §6), fix anything Windows-specific that leaked into shared code paths
   (e.g., accidental `\r\n` assumptions).
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
| Symlink recursion default | follow / don't follow | §3.4 |
| Color library | `x/term` + hand-rolled vs. `go-colorable`/`termenv` | §3.3 |
| Long path support | rely on Go runtime vs. explicit `\\?\` handling | §3.4 |
