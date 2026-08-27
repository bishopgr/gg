# gg — a cross-platform grep, built for Windows first

`gg` is a `grep`-compatible command-line search tool written in Go. It
searches files (or standard input) for lines matching a pattern and prints
them, with the flags and output conventions GNU grep users already know —
`-i`, `-r`, `-n`, `-c`, `-o`, `-v`, `--include`/`--exclude`, color
highlighting, context lines, and more.

It's built **Windows-first**: correct line-ending handling (CRLF/LF/CR),
UTF-16/BOM-aware file reading, PowerShell-safe glob expansion, and
file-sharing semantics that don't choke on concurrent writes or log
rotation — while remaining a normal, portable Go binary on Linux and macOS.

> **Project status:** `gg` is a working, everyday-usable grep replacement.
> Matching, multi-file/stdin/recursive search, output shaping, context
> lines, multi-pattern search, and color are all implemented. A handful of
> less common GNU grep features aren't there yet — see
> [Not yet supported](#not-yet-supported) for the exact list.

## Features

- Drop-in familiar flags: `-i -F -v -w -x -n -o -c -l -L -q -h -H`
- Recursive directory search (`-r`/`-R`) with `--include`, `--exclude`,
  and `--exclude-dir` glob filtering
- Context lines around matches (`-A`, `-B`, `-C`), including GNU grep's
  `--` group-separator behavior
- Multiple patterns at once, via repeated `-e` or a pattern file (`-f`)
- Color-highlighted matches (`--color=auto|always|never`), with proper
  Windows terminal (ANSI/VT) detection
- Reads from files, stdin, or piped input transparently
- Encoding-aware: strips UTF-8 BOMs, transcodes UTF-16 (common from
  PowerShell's `Out-File`), and detects binary files (with `-a`/`-I` to
  override the default behavior)
- Correct exit codes (`0`/`1`/`2`) for scripting and CI use
- No CGO, no external runtime dependencies — a single static binary

## Installation

Requires [Go](https://go.dev) 1.26 or newer.

```powershell
git clone <this-repo-url>
cd gg
go build -o bin/gg.exe ./cmd/gg
```

For a build with embedded version/commit/date metadata (what `gg
--version` reports), use the provided build script instead:

```powershell
./scripts/build.ps1
```

Prebuilt binaries for Windows, Linux, and macOS may also be available from
this project's Releases page, if you're not building from source.

## Usage

```
gg [OPTION]... PATTERN [FILE...]
gg --version
gg --help
```

`gg` searches `FILE`(s) for lines matching `PATTERN` and prints them. When
no `FILE` is given, or `FILE` is `-`, `gg` reads standard input.

```powershell
gg error app.log                    # search one file
gg -i "warn|error" app.log          # case-insensitive, alternation
gg -rn TODO .\src                   # recursive, with line numbers
gg -rl "TODO" .\src                 # just the filenames that contain it
gg -c error app.log                 # just a count
gg -v skip app.log                  # invert: lines NOT matching
gg -o "\d+" app.log                 # only the matched substrings
Get-Content app.log | gg error      # read from stdin
gg --include=*.go -r TODO .         # only search .go files
```

## Examples

The commands and output below were run against a real `gg.exe` build, in
a working directory shaped like this:

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

### Basics

Search one file for a pattern:

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

Read from standard input:

```powershell
PS> Get-Content app.log | gg -ci error
2
```

### Filtering and formatting output

Invert the match, to see everything that *isn't* an info line (`-v`):

```powershell
PS> gg -v INFO app.log
2024-01-01T10:00:01 WARN  cache miss for key=42
2024-01-01T10:00:02 ERROR failed to connect to db: timeout
2024-01-01T10:00:04 ERROR failed to connect to db: timeout
```

Show line numbers, and require a whole-word match so `ERRORS` wouldn't
also match (`-n -w`):

```powershell
PS> gg -n -w ERROR app.log
3:2024-01-01T10:00:02 ERROR failed to connect to db: timeout
5:2024-01-01T10:00:04 ERROR failed to connect to db: timeout
```

Extract just the matched substring instead of whole lines (`-o`) — here,
pulling out every date:

```powershell
PS> gg -o "\d{4}-\d{2}-\d{2}" app.log
2024-01-01
2024-01-01
2024-01-01
2024-01-01
2024-01-01
2024-01-01
```

### Recursive search

Recurse into a directory tree, with line numbers (`-rn`):

```powershell
PS> gg -rn TODO .
src\main.go:3:// TODO: add graceful shutdown
src\util\util.go:3:// TODO: replace with a real implementation
vendor\thirdparty.go:2:// TODO: this should never show up in results
```

Print only the matching filenames instead (`-rl`):

```powershell
PS> gg -rl TODO .
src\main.go
src\util\util.go
vendor\thirdparty.go
```

Skip a whole directory while recursing (`--exclude-dir`) — useful for
keeping `vendor`, `node_modules`, `.git`, etc. out of results:

```powershell
PS> gg -rn --exclude-dir=vendor TODO .
src\main.go:3:// TODO: add graceful shutdown
src\util\util.go:3:// TODO: replace with a real implementation
```

Combine several flags at once — recursive, case-insensitive, only `.go`
files, excluding `vendor`, with line numbers:

```powershell
PS> gg -rni --include=*.go --exclude-dir=vendor "todo" .
src\main.go:3:// TODO: add graceful shutdown
src\util\util.go:3:// TODO: replace with a real implementation
```

### Context lines

Show the lines surrounding each match on both sides (`-C`) — when two
matches' context windows overlap, they merge into one block instead of
duplicating shared lines:

```powershell
PS> gg -C 1 -n ERROR app.log
2-2024-01-01T10:00:01 WARN  cache miss for key=42
3:2024-01-01T10:00:02 ERROR failed to connect to db: timeout
4-2024-01-01T10:00:03 INFO  retrying connection
5:2024-01-01T10:00:04 ERROR failed to connect to db: timeout
6-2024-01-01T10:00:05 INFO  connected
```

Non-adjacent matches get a `--` separator between groups instead — note
that this happens even with `-A0`, which prints zero context lines but
still separates unrelated matches (matching GNU grep's behavior):

```powershell
PS> gg -A0 -n ERROR app.log
3:2024-01-01T10:00:02 ERROR failed to connect to db: timeout
--
5:2024-01-01T10:00:04 ERROR failed to connect to db: timeout
```

### Multiple patterns

Search for several patterns kept in a file (`-f`) — useful for a reusable
list like log severities to flag:

```powershell
PS> Get-Content levels.txt
ERROR
WARN
PS> gg -n -f levels.txt app.log
2:2024-01-01T10:00:01 WARN  cache miss for key=42
3:2024-01-01T10:00:02 ERROR failed to connect to db: timeout
5:2024-01-01T10:00:04 ERROR failed to connect to db: timeout
```

The same search, inline with repeated `-e` instead of a file. Once `-e`
(or `-f`) is used at all, every positional argument becomes a `FILE`, not
a pattern:

```powershell
PS> gg -e ERROR -e WARN -n app.log
2:2024-01-01T10:00:01 WARN  cache miss for key=42
3:2024-01-01T10:00:02 ERROR failed to connect to db: timeout
5:2024-01-01T10:00:04 ERROR failed to connect to db: timeout
```

### Everything else

Anchor a whole-line match (`-x`), e.g. to assert a file's first line is
*exactly* `package main`:

```powershell
PS> gg -x "package main" src\main.go
package main
```

Use `gg` purely for its exit code in a script or CI check — `-q`
suppresses all output and stops at the first match; `0` means found, `1`
means not found:

```powershell
PS> gg -q ERROR app.log; $LASTEXITCODE
0
PS> gg -q nomatch app.log; $LASTEXITCODE
1
```

Force color even when output isn't a terminal, e.g. piping to a file
you'll view later with a color-aware viewer:

```powershell
PS> gg --color=always -n ERROR app.log
3:2024-01-01T10:00:02 ERROR failed to connect to db: timeout
5:2024-01-01T10:00:04 ERROR failed to connect to db: timeout
```

(In a real terminal, `ERROR` renders bold red — Markdown can't show the
ANSI escape codes `gg` emits, but they're really there.)

Search a binary file as if it were text (`-a`), instead of getting the
default `Binary file X matches` message:

```powershell
PS> gg field data.bin
Binary file data.bin matches
PS> gg -a field data.bin
record1 field=42
record2 field=99
```

Or go the other way and skip binary files entirely (`-I`), as if they had
no matches at all — useful when recursively searching a directory that
might contain build artifacts or other binary junk:

```powershell
PS> gg -I field data.bin; $LASTEXITCODE
1
```

## Command-line options

| Flag | Meaning |
|---|---|
| `--help` | show usage |
| `--version` | print version, commit, and build date |
| `-i`, `--ignore-case` | ignore case distinctions |
| `-F`, `--fixed-strings` | treat `PATTERN` as a literal string, not a regex |
| `-v`, `--invert-match` | select non-matching lines |
| `-w`, `--word-regexp` | match only whole words |
| `-x`, `--line-regexp` | match only whole lines |
| `-n`, `--line-number` | prefix each printed line with its line number |
| `-o`, `--only-matching` | print only the matched part of each line |
| `-c`, `--count` | print only a per-file count of matching lines |
| `-l`, `--files-with-matches` | print only names of files with a match |
| `-L`, `--files-without-match` | print only names of files with no match |
| `-q`, `--quiet` | no output at all; exit status only (stops at first match) |
| `-h`, `--no-filename` | never print filenames |
| `-H`, `--with-filename` | always print filenames |
| `-r`, `--recursive` | recurse into directories (does not follow symlinks) |
| `-R`, `--dereference-recursive` | like `-r`, but follow all symlinks |
| `--include=GLOB` | only search files matching `GLOB` when recursing |
| `--exclude=GLOB` | skip files matching `GLOB` when recursing |
| `--exclude-dir=GLOB` | don't descend into directories matching `GLOB` |
| `--color[=auto\|always\|never]` | highlight matches; bare `--color` means `always` |
| `-A NUM`, `--after-context=NUM` | print `NUM` lines of trailing context after each match |
| `-B NUM`, `--before-context=NUM` | print `NUM` lines of leading context before each match |
| `-C NUM`, `--context=NUM` | print `NUM` lines of context on both sides |
| `-e PATTERN`, `--regexp=PATTERN` | add `PATTERN` to the list of patterns to match (repeatable) |
| `-f FILE`, `--file=FILE` | add patterns from `FILE`, one per line (repeatable; `-` means stdin) |
| `-a`, `--text` | treat binary files as text instead of printing `Binary file X matches` |
| `-I`, `--skip-binary` | skip binary files entirely, as if they had no matches |

A few behaviors are worth knowing about if you use these flags together:

- **`--help`/`--version` are long-flag-only.** `gg` follows GNU grep's own
  convention where `-h` means `--no-filename` and `-v` means
  `--invert-match`, not help/version.
- **`-A`/`-B` beat `-C` for their own direction.** `gg -A 1 -C 3` and
  `gg -C 3 -A 1` both resolve to `after=1, before=3`, regardless of flag
  order. Explicit `-A0`/`-B0`/`-C0` still turn on `--` group separators
  between non-adjacent matches, even though no context lines get printed.
  GNU grep's `-NUM` shorthand for `-C NUM` isn't supported — see
  [Not yet supported](#not-yet-supported).
- **`-e`/`-f` replace the positional pattern.** Once either is given,
  every remaining positional argument is treated as a `FILE`, never a
  pattern. Patterns from `-e` and `-f` combine in a simple order: every
  `-e` pattern first, then every `-f` file's lines. A blank line in a
  `-f` file is a literal empty-string pattern (matches every line, like
  `-e ""`). Supplying zero patterns total makes the search match
  *nothing*, not everything.
- **`-a`/`-I` and binary files.** `-a` makes a binary-detected file behave
  exactly like text (matching, context lines, `-v`, raw bytes and all).
  `-I` treats it as if it produced no matches: `-c` reports `0`, `-l`
  never lists it, `-L` always does — though the overall exit code still
  reflects whether *something* matched somewhere, same as plain grep. If
  both `-a` and `-I` are given, `-I` wins.

## Exit status

| Code | Meaning |
|---|---|
| `0` | at least one match was found (also: `--version`/`--help` succeeded) |
| `1` | ran successfully, found no matches |
| `2` | usage error, invalid pattern, or a per-file problem (missing file, permission denied, a directory given without `-r`) |

Per-file problems (missing file, "Is a directory", etc.) are reported to
stderr as they're found, and the search continues with any remaining
files — matching GNU grep's behavior — with the exit code still ending up
`2`.

## Not yet supported

These are deliberate, known gaps rather than hidden bugs:

- **`-NUM` shorthand for `-C NUM`** (e.g. `grep -3 pattern file`)
- **`-m NUM`** (stop after N matches per file)
- **`-z`** (NUL-separated records)
- **PCRE (`-P`)** — `gg` uses Go's `regexp` package (RE2 syntax): no
  backreferences, no lookaround. This is a stated design choice, not
  planned for the future.
- **No-BOM UTF-16 detection** — there's no reliable signature to detect
  this, so such files read as garbage text.
- **De-duplication of repeated file arguments** — passing the same file
  twice searches it twice.
- **Recursive (`**`) glob patterns** in filename arguments — use
  `-r`/`--include` instead.

See [PROCESS.md](PROCESS.md) for the full design rationale and rollout
plan behind these decisions.

## Building and testing

```powershell
# quick local build, no version metadata
go build -o bin/gg.exe ./cmd/gg

# release-style build with git-derived version/commit/date
./scripts/build.ps1

# run the full test suite (unit + black-box end-to-end tests)
go vet ./...
go test ./...
```

Requires Go 1.26+. No CGO, no external system dependencies.

## Project layout

```
cmd/gg/           entrypoint
internal/cli/     Cobra command tree, flag parsing
internal/grep/    composition root that wires everything together
internal/matcher/ pattern matching (fixed-string + regex)
internal/lineio/  line splitting (CRLF/LF/CR-aware, streaming)
internal/encoding/ BOM detection and UTF-16 transcoding, binary sniffing
internal/walker/  directory traversal, glob expansion, include/exclude
internal/output/  result formatting (filenames, line numbers, color)
internal/termenv/ terminal/color-capability detection
internal/source/  Windows-safe file opening (works with concurrent writes/deletes)
test/e2e/         black-box tests against the compiled binary
```

See [PROCESS.md](PROCESS.md) for the detailed design decisions and
verification notes behind each package, and [BENCHMARK.md](BENCHMARK.md)
for performance measurements.

## Contributing

Bug reports and pull requests are welcome. If you're changing behavior
that overlaps with GNU grep, please note any manual comparison you did
against real grep in your PR description — this project treats grep's
actual behavior as the source of truth over the man page or intuition.

## See also

- [PROCESS.md](PROCESS.md) — full design and build plan this project follows
- [BENCHMARK.md](BENCHMARK.md) — performance measurements
- `grep(1)`, and Go's [RE2 syntax reference](https://github.com/google/re2/wiki/Syntax)

## License

MIT — see [LICENSE](LICENSE).
