# gg — manual test session output

Ad hoc exercising of the real, compiled `gg.exe` on Windows (PowerShell
7), run after the CLI was wired up end-to-end. This is not the
automated test suite (see `go test ./...` and `test/e2e/`) — it's a
manual dogfooding pass, and it found (and fixed) one real bug. See
[FINDINGS](#findings) at the bottom.

**Environment:**

```
gg version 9c916ca-dirty (commit=9c916ca, built=2026-08-26T18:30:59Z)
go version go1.26.2 windows/amd64
```

All commands below were run from the repository root (`C:\Dev\Go\gg`)
in PowerShell 7, invoking `.\bin\gg.exe` directly.

---

## 1. `--version`

```powershell
PS> .\bin\gg.exe --version
gg version 9c916ca-dirty (commit=9c916ca, built=2026-08-26T18:30:59Z)
exit=0
```

## 2. The requested pipe test: `$(gg.exe --version) | gg "version"`

`$(...)` runs `gg.exe --version` and captures its stdout as a string;
PowerShell then pipes that string to the second `gg.exe` invocation as
its standard input. `gg "version"` has no `FILE` argument, so it reads
that piped text from stdin (the documented `-`/no-argument default) and
searches it for the pattern `version`.

```powershell
PS> $(.\bin\gg.exe --version) | .\bin\gg.exe "version"
gg version 9c916ca-dirty (commit=9c916ca, built=2026-08-26T18:30:59Z)
exit=0
```

Works exactly as expected: the version string contains the literal word
"version," so it's echoed back, with exit code `0` (match found). This
also incidentally proves the stdin-default and pipe-composability both
work correctly through a real external-process pipeline, not just
in-process tests.

## 3. Basic search in a source file

```powershell
PS> .\bin\gg.exe "func Run" .\internal\grep\grep.go
func Run(ctx context.Context, opts Options) (int, error) {
exit=0
```

## 4. Case-insensitive search (`-i`)

```powershell
PS> .\bin\gg.exe -i "PACKAGE grep" .\internal\grep\grep.go
// Package grep is the composition root for the search engine: it wires
package grep
exit=0
```

## 5. Recursive search for stray `TODO` markers (`-r`)

```powershell
PS> .\bin\gg.exe -r "TODO" .\internal
exit=1
```

No output, exit `1` — no leftover `TODO` placeholder comments in
`internal/`. Sanity-checked by widening the search to the whole repo
(`.`), which correctly turns up the ones that *do* exist — Git's own
sample hook scripts, and this README's own example lines that
deliberately mention `TODO` as demo text:

```powershell
PS> .\bin\gg.exe -r "TODO" .
.git\hooks\sendemail-validate.sample:# Replace the TODO placeholders with appropriate checks according to your
.git\hooks\sendemail-validate.sample:	# TODO: Replace with appropriate checks (e.g. spell checking).
.git\hooks\sendemail-validate.sample:	# TODO: Replace with appropriate checks for this patch
.git\hooks\sendemail-validate.sample:	# TODO: Replace with appropriate checks for the whole series
README.md:gg -rn TODO .\src                    # recursive, with line numbers
README.md:gg -rl "TODO" .\src                  # just the filenames that contain it
README.md:gg --include=*.go -r TODO .          # only search .go files
exit=0
```

## 6. Recursive per-file count (`-rc`)

```powershell
PS> .\bin\gg.exe -rc "func " .\internal\matcher
internal\matcher\fixed.go:3
internal\matcher\fixed_test.go:3
internal\matcher\fuzz_test.go:1
internal\matcher\matcher.go:2
internal\matcher\matcher_test.go:4
internal\matcher\regex.go:2
internal\matcher\regex_test.go:3
exit=0
```

## 7. Only-matching extraction (`-o`)

Extracting every `internal/xxx` package reference out of README.md:

```powershell
PS> .\bin\gg.exe -o "internal/[a-z]+" README.md
internal/cli
internal/cli
internal/source
internal/buildinfo
internal/matcher
...(48 lines total)...
internal/walker
internal/walker
exit=0
```

## 8. Invert match (`-v`)

```powershell
PS> Get-Content go.mod
module gg

go 1.26.2

require github.com/spf13/cobra v1.10.2

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)

PS> .\bin\gg.exe -v "require" go.mod
module gg

go 1.26.2


	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
exit=0
```

Both lines containing `require` are correctly dropped; blank/other
lines pass through unchanged.

## 9. Files-with-matches (`-l`)

```powershell
PS> .\bin\gg.exe -rl "context.Context" .\internal\grep
internal\grep\engine.go
internal\grep\grep.go
exit=0
```

## 10. Files-without-match (`-L`) — **found and fixed a real bug here**

```powershell
PS> .\bin\gg.exe -rL "package grep" .\internal\grep
exit=0        <-- WRONG, before the fix (see FINDINGS)
```

Every file in `internal/grep` declares `package grep`, so `-L` correctly
printed *nothing* — but it exited `0` (as if it had found something to
report) instead of `1`. Fixed; re-verified after the fix:

```powershell
PS> .\bin\gg.exe -rL "package grep" .\internal\grep
exit=1        <-- correct
```

And the case where `-L` genuinely has something to select still works:

```powershell
PS> .\bin\gg.exe -rL "package output" .\internal\grep
internal\grep\color.go
internal\grep\color_test.go
internal\grep\engine.go
internal\grep\errors.go
internal\grep\errors_test.go
internal\grep\grep.go
internal\grep\grep_test.go
internal\grep\run_test.go
exit=0
```

## 11. Word-regexp (`-w`)

```powershell
PS> "the category contains a cat" | .\bin\gg.exe -w "cat"
the category contains a cat
exit=0
```

The line matches because "cat" appears standalone later in the
sentence, not because of "category". Confirmed by removing the
standalone occurrence:

```powershell
PS> "the category is nice" | .\bin\gg.exe -w "cat"
exit=1
```

No match — `-w` correctly refuses to match "cat" as a substring of
"category".

## 12. Chained pipe through two `gg` invocations

```powershell
PS> Get-Content README.md | .\bin\gg.exe -i "windows" | .\bin\gg.exe -c "."
26
exit=0
```

First `gg` filters README.md's lines for (case-insensitive) "windows";
the second counts how many of *those* lines are non-empty (`.` matches
any character). Confirms `gg`'s stdout composes cleanly as another
process's stdin, chained twice.

## 13. Missing-file error

```powershell
PS> .\bin\gg.exe pattern .\does-not-exist.txt
gg: .\does-not-exist.txt: No such file or directory
exit=2
```

Portable message text (not Windows' native "The system cannot find the
file specified.") — this is `internal/grep/errors.go`'s
`fs.ErrNotExist`-based translation doing its job.

## 14. Color (`--color=always`) — verified at the raw byte level

Piping through `Format-Hex` after a `Select-Object` turned out to be an
unreliable way to inspect bytes (PowerShell re-encodes the string object
in between, which visibly mangled an unrelated em-dash elsewhere in the
same line — a PowerShell pipeline artifact, not a `gg` bug). Redirecting
straight to a file and reading raw bytes is the trustworthy version:

```powershell
PS> .\bin\gg.exe --color=always "cross-platform" README.md > $env:TEMP\gg-color-test.txt
PS> [System.IO.File]::ReadAllBytes(...) | ForEach-Object { $_.ToString("X2") } | ... -join " "
23 20 67 67 28 31 29 20 E2 80 94 20 1B 5B 31 3B 33 31 6D 63 72 6F 73 73 2D 70 6C 61 74 66 6F 72 6D 1B 5B 30 6D 20 67 72 ...

PS> [System.Text.Encoding]::UTF8.GetString($bytes)
# gg(1) — cross-platform grep
  this defers to a maintained, cross-platform implementation.
```

Byte-exact confirmation: `1B 5B 31 3B 33 31 6D` is `\x1b[1;31m` (the
highlight-start code) immediately before "cross-platform", `1B 5B 30 6D`
is `\x1b[0m` (reset) immediately after, and `E2 80 94` is the correct
3-byte UTF-8 encoding of "—" — `gg`'s own output was correct UTF-8 with
correct ANSI the whole time; the first attempt's mangled em-dash was
introduced by the terminal-display pipeline, not by `gg`.

## 15. `--include` filter

```powershell
PS> .\bin\gg.exe --include=*.ps1 -r "go build" .
scripts\build.ps1:go build -ldflags $ldflags -o $Output ./cmd/gg
exit=0
```

## 16. `--exclude-dir` filter

```powershell
PS> .\bin\gg.exe -r --exclude-dir=testdata "line" .\test
test\e2e\e2e_test.go:...(8 matching lines from e2e_test.go's own source, referencing "line")...
exit=0
```

`test\testdata\*.txt` (which contain literal `"line one"` etc. and would
otherwise dominate the results) are correctly excluded; only
`test\e2e\e2e_test.go`'s own source lines mentioning "line" remain.

## 17. Quiet mode (`-q`)

```powershell
PS> .\bin\gg.exe -q "package grep" .\internal\grep\grep.go
exit=0
```

No output at all, correct exit code.

## 18. Recursive `-o` across a whole directory tree

```powershell
PS> .\bin\gg.exe -ro "^package \w+" .\internal
internal\buildinfo\buildinfo.go:package buildinfo
internal\cli\root_test.go:package cli
internal\cli\root.go:package cli
...(45 lines total, one per .go file under internal/)...
internal\walker\walker.go:package walker
```

Every `.go` file under `internal/` accounted for, correct package name
extracted from each.

## 19. Recursive per-file test-function count

```powershell
PS> .\bin\gg.exe -rc "^func Test" .\internal\grep
internal\grep\color.go:0
internal\grep\color_test.go:4
internal\grep\engine.go:0
internal\grep\errors.go:0
internal\grep\errors_test.go:4
internal\grep\grep.go:0
internal\grep\grep_test.go:2
internal\grep\run_test.go:31
```

## 20. Multi-flag combination, no match expected

```powershell
PS> .\bin\gg.exe -rin "FIXME|XXX" .\internal
exit=1
```

`-r` (recursive) + `-i` (case-insensitive) + `-n` (line numbers)
combined correctly; no such markers exist in the codebase, so no
output and exit `1`.

---

## FINDINGS

### Bug found and fixed: `-L`/`-l` exit code didn't match their own selection criterion

**Symptom**: `gg -rL "package grep" .\internal\grep` printed nothing
(correct — every file in that directory matches, so `-L`, which lists
files *without* a match, has nothing to list) but exited `0` as if it
had found something to report. It should exit `1` (nothing selected).

**Root cause**: `internal/grep/engine.go`'s per-file search loop set the
overall "did we find anything" flag (`hadMatch`, which drives the final
exit code) from the raw "did any line in this file match the pattern"
result, unconditionally — regardless of output mode. That's correct for
normal search and for `-l` (files*with* a match — "matched" *is* its
selection criterion), but backwards for `-L`: its selection criterion is
the *opposite*, "this file did **not** match," so `hadMatch` needs to be
set from `!matched`, not `matched`.

**Why automated tests didn't catch it**: the existing
`TestRun_FilesWithMatches`/`TestRun_FilesWithoutMatch` tests asserted
stdout content but discarded the returned exit code (`_, stdout, _, err
:= runCapture(...)`) — so a wrong-but-plausible exit code had nothing
checking it. Manual exercising with a directory where the pattern
matched *every* file (not the tests' "some match, some don't" fixture)
is what surfaced it, since that's the specific case where "-L found
nothing to report" and "no lines matched anywhere" happen to coincide
in every *other* test case — the two conditions are indistinguishable
unless every file matches.

**Fix**: `internal/grep/engine.go` now sets `hadMatch` per-mode, using
each mode's actual selection criterion (`matched` for `-l`/count/normal,
`!matched` for `-L`), decided in the same `switch` that does the
printing rather than in a separate blanket check beforehand.

**Regression coverage added**:
- `TestRun_FilesWithoutMatch_ExitCodeReflectsSelectionNotRawLineMatching`
  — every file matches; `-L` must print nothing and exit `ExitNoMatch`.
- `TestRun_FilesWithMatches_ExitCodeReflectsSelectionNotRawLineMatching`
  — no file matches; `-l` must print nothing and exit `ExitNoMatch`.
- The two pre-existing tests now also assert the exit code, closing the
  actual gap that let this slip through.

All ~109 automated tests (`go test ./...`) plus the manual scenarios
above pass after the fix.

### Non-bug: PowerShell pipeline byte-mangling during a `Format-Hex` inspection

Piping `gg`'s output through `Select-Object -First 1 | Format-Hex`
displayed a mangled multi-byte character elsewhere in the same line.
Re-verified by redirecting `gg`'s raw stdout straight to a file and
reading its bytes directly (`[System.IO.File]::ReadAllBytes`) —
byte-perfect UTF-8 and correct ANSI codes. The mangling was introduced
by PowerShell decoding the raw bytes into a .NET string and `Format-Hex`
re-encoding that string, not by `gg`. Noted here so this specific
inspection technique isn't mistaken for a reliable byte-level check in
future sessions — redirect to a file and read bytes directly instead.

## Summary

20 manual scenarios exercised across matching flags (`-i -F -w -v`),
output-shaping flags (`-n -o -c -l -L -q`), traversal (`-r`,
`--include`, `--exclude-dir`), color, stdin/pipe composition (including
the specifically requested `$(gg --version) | gg "version"` case and a
two-stage `gg | gg` chain), and error handling. One real bug was found,
root-caused, fixed, and regression-tested; one false alarm (a
PowerShell display-pipeline artifact) was investigated and ruled out
rather than assumed. Every exit code observed (`0`, `1`, `2`) matched
expectations after the fix.
