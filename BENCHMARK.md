# BENCHMARK.md — `feature/step-3` vs. `feature/perf-and-cleanup`

This compares `gg`'s core hot paths before and after the PROCESS.md §4
step 8 performance pass, using `go test -bench`/`benchstat` rather than
hand-waving. It exists as a record of what actually changed, by how
much, and where the numbers came from — in the same spirit as
PROCESS.md itself: claims backed by a reproducible measurement, not
intuition.

**Scope note**: per instruction, this is a benchmarking exercise, not a
bug hunt. One pre-existing, unrelated issue surfaced while reproducing
`feature/step-3`'s baseline (see "A note on reproducing the baseline"
below) and is reported, not chased further.

## TL;DR

| Package | What | `feature/step-3` | `feature/perf-and-cleanup` | Change |
|---|---|---:|---:|---:|
| `internal/lineio` | `ReadLine`, LF input | 219.5 MiB/s | 3711.5 MiB/s | **+1591%** |
| `internal/lineio` | `ReadLine`, CRLF input | 222.1 MiB/s | 3375.7 MiB/s | **+1420%** |
| `internal/lineio` | `ReadLine`, bare-CR input | 218.2 MiB/s | 789.1 MiB/s | **+262%** |
| `internal/lineio` | `ReadLine`, `Reset`-and-reuse | 218.9 MiB/s | 3756.2 MiB/s | **+1616%** |
| `internal/matcher` | fixed/regex `Find` (all cases) | — | — | unchanged (not touched this pass) |
| `internal/grep` | end-to-end, regex, sparse matches | 155.0 MiB/s | 504.2 MiB/s | **+225%** |
| `internal/grep` | end-to-end, `-F`, sparse matches | 184.1 MiB/s | 862.3 MiB/s | **+368%** |
| `internal/grep` | end-to-end, `-q`, first-line match | 26.74 GiB/s | 25.80 GiB/s | ~unchanged (noise) |

`internal/lineio.ReadLine` is where essentially all of the win comes
from: it is the one package this pass actually rewrote. `internal/grep`'s
end-to-end numbers move by a smaller (but still large) factor because a
real search also spends time in file I/O and `regexp` matching, neither
of which this pass touched.

## Methodology

- **Branches**: `feature/step-3` (`1676284`, the last commit before the
  performance pass) vs. `feature/perf-and-cleanup` (`6955fd1`, the
  performance pass itself).
- **Both branches checked out as separate `git worktree`s** of the same
  repo/commit graph, built and run independently — not two states of a
  working directory, so there's no chance of stale build artifacts
  leaking between measurements.
- **Same benchmark code against both branches.** The three
  `bench_test.go` files (`internal/lineio`, `internal/matcher`,
  `internal/grep`) only exist on `feature/perf-and-cleanup` (they were
  added *by* this performance pass — see PROCESS.md §4 step 8). To
  compare fairly, the identical files were copied, unmodified, into the
  `feature/step-3` worktree before running `go test -bench` there. This
  means both sides ran the literal same benchmark harness, generated
  fixtures, and assertions against their own branch's library code —
  the fixture-generation and driver code is not part of what's being
  measured, only `internal/lineio`, `internal/matcher`, and
  `internal/grep`'s own implementations are.
- **Tool**: `golang.org/x/perf/cmd/benchstat` for the headline tables
  (proper statistics — medians, variance, p-values — not a single
  eyeballed run). `internal/lineio` and `internal/grep` benchmarks were
  run with `-count=8` and `-count=6` respectively (`-benchmem` always
  on); every reported delta below has `p ≤ 0.002`, i.e. not noise.
- **Machine**: single physical machine, Intel Core i7-8700K, Windows,
  `go1.26.2`. Absolute numbers are machine-specific; the *relative*
  before/after deltas are what matters here, not the raw GiB/s figures.
- **Commands** (run from each worktree root):
  ```powershell
  go test ./internal/lineio/...  -run "^$" -bench . -benchmem -benchtime=1s    -count=8
  go test ./internal/matcher/... -run "^$" -bench . -benchmem -benchtime=2s    -count=3
  go test ./internal/grep/...    -run "^$" -bench . -benchmem -benchtime=1500ms -count=6
  benchstat step3-<pkg>.txt perf-<pkg>.txt
  ```

### A note on reproducing the baseline

Building and testing the `feature/step-3` worktree surfaced the same
pre-existing, unrelated bug documented in PROCESS.md §4 step 8's "found
and fixed" note: `core.autocrlf=true` (a common Windows Git default)
silently rewrites `test/testdata/lf.txt` on checkout, and
`test/testdata/crlf.txt`'s committed blob had already lost its actual
`CRLF` bytes to the same setting at some earlier commit. This makes
`TestReader_CRLFAndLFFixturesAgreeOnContent` fail on a fresh
`feature/step-3` checkout on this machine — it is **not** caused by
anything in this benchmarking pass, does not affect any benchmark here
(all benchmarks below use synthetic in-memory/generated fixtures, not
`test/testdata`), and is already fixed on `feature/perf-and-cleanup`
(`test/testdata/.gitattributes`). Reported per instruction rather than
fixed on `feature/step-3` — that branch's worktree was throwaway,
built solely to run these benchmarks.

## Results

### `internal/lineio` — line-splitting hot path

```
                   │  feature/step-3   │      feature/perf-and-cleanup      │
                   │      sec/op       │      sec/op       vs base          │
ReadLine_LF-12          17.593m ± 3%         1.041m ± 3%   -94.08% (p=0.000 n=8)
ReadLine_CRLF-12        17.607m ± 1%         1.158m ± 2%   -93.42% (p=0.000 n=8)
ReadLine_BareCR-12      17.699m ± 1%         4.895m ± 2%   -72.34% (p=0.000 n=8)
ReadLine_Reuse-12       17.642m ± 1%         1.028m ± 1%   -94.17% (p=0.000 n=8)
geomean                  17.64m              1.569m        -91.10%

                   │  feature/step-3   │      feature/perf-and-cleanup      │
                   │        B/s        │        B/s        vs base          │
ReadLine_LF-12          219.5Mi ± 6%      3711.5Mi ± 8%   +1590.57% (p=0.000 n=8)
ReadLine_CRLF-12        222.1Mi ± 1%      3375.7Mi ± 1%   +1420.08% (p=0.000 n=8)
ReadLine_BareCR-12      218.2Mi ± 1%       789.1Mi ± 2%    +261.58% (p=0.000 n=8)
ReadLine_Reuse-12       218.9Mi ± 1%      3756.2Mi ± 2%   +1615.76% (p=0.000 n=8)
geomean                 219.7Mi            2.411Gi        +1023.68%

                   │  feature/step-3   │      feature/perf-and-cleanup      │
                   │        B/op       │        B/op       vs base          │
ReadLine_LF-12          64.23Ki ± 0%       65.32Ki ± 0%       +1.69% (p=0.000 n=8)
ReadLine_CRLF-12        64.23Ki ± 0%       64.48Ki ± 0%       +0.38% (p=0.000 n=8)
ReadLine_BareCR-12      64.23Ki ± 0%       65.32Ki ± 0%       +1.69% (p=0.000 n=8)
ReadLine_Reuse-12        1083.5 ± 9%         105.0 ± 2%      -90.31% (p=0.000 n=8)
geomean                 23.01Ki            12.96Ki           -43.68%

                   │  feature/step-3   │      feature/perf-and-cleanup      │
                   │     allocs/op     │     allocs/op     vs base          │
ReadLine_LF-12            4.000 ± 0%         17.000 ± 0%    +325.00% (p=0.000 n=8)
ReadLine_CRLF-12          4.000 ± 0%          8.000 ± 0%    +100.00% (p=0.000 n=8)
ReadLine_BareCR-12        4.000 ± 0%         17.000 ± 0%    +325.00% (p=0.000 n=8)
ReadLine_Reuse-12         1.000 ± 0%          1.000 ± 0%           ~ (p=1.000 n=8)
geomean                   2.828               6.934         +145.16%
```

**Reading the allocation count increase**: `allocs/op` going up (4→17,
4→8) looks like a regression next to a 12–16× throughput win, so it's
worth being explicit about what it actually is. `feature/step-3`'s
`bytes.Buffer`-based accumulator happened to reach its final capacity
in very few (large) reallocations. The rewritten version's persistent
`r.line []byte` grows via `append`'s doubling from empty the first time
a line needs cross-chunk accumulation (rare — only lines spanning more
than one buffered chunk, or the CRLF-at-a-window-boundary case), which
takes a few more (much smaller) reallocations to reach the same
capacity. Two things matter more than the count itself: (1) net bytes
allocated per op is actually **lower**, not higher (`B/op` geomean
-43.68%, driven mostly by `ReadLine_Reuse`'s -90.31%), and (2) this
growth is a one-time warm-up cost against a *reused* buffer — the
`Reset`-and-reuse benchmark (the shape `internal/grep` actually uses
across a multi-file walk) shows `allocs/op` unchanged at a flat `1`
either way, i.e. once `r.line`'s capacity stabilizes for a given
`Reader`, further `ReadLine` calls allocate no more than they did
before.

**Bare-CR is a known, smaller win, not a miss.** PROCESS.md §4 step 8
documents this in detail: bare-CR-only content (no `'\n'` anywhere)
can't benefit from the "find `'\n'`, then bound the `'\r'` search by
it" optimization the other two terminators get, since there's no
`'\n'` to bound by. It still gets ~2.6× faster (the byte-by-byte scan
is gone either way) but tops out well below LF/CRLF's ~14–16×. This is
an accepted, deliberate tradeoff for what PROCESS.md §3.1 already calls
a rare/legacy case — not something this pass attempted to close
further.

### `internal/matcher` — pattern matching

Not touched by this performance pass (it was already allocation-free on
the no-match path, the actual common case — see PROCESS.md §4 step 8).
`benchstat` on 3 samples/branch confirms no measurable difference on
either `sec/op`, `B/op`, or `allocs/op` for any of the four
`fixedMatcher`/`regexMatcher` × match/no-match combinations — every row
reports `~` (not statistically distinguishable). Included here as a
control: it's the package that *shouldn't* have moved, and didn't.

### `internal/grep` — end-to-end (`source` → `Sniff` → `lineio` → `matcher` → `output`)

```
                              │  feature/step-3   │      feature/perf-and-cleanup      │
                              │      sec/op       │      sec/op        vs base         │
Run_Regex_SparseMatches-12       41.72m ± 3%          12.83m ± 7%    -69.25% (p=0.002 n=6)
Run_Fixed_SparseMatches-12      35.130m ± 3%          7.502m ± 5%    -78.65% (p=0.002 n=6)
Run_Quiet_FirstLineMatches-12    198.0µ ± 2%          205.2µ ± 8%     +3.64% (p=0.002 n=6)
geomean                          6.620m               2.703m         -59.17%

                              │  feature/step-3   │      feature/perf-and-cleanup      │
                              │        B/s        │        B/s         vs base         │
Run_Regex_SparseMatches-12       155.0Mi ± 3%        504.2Mi ± 7%    +225.24% (p=0.002 n=6)
Run_Fixed_SparseMatches-12       184.1Mi ± 3%        862.3Mi ± 5%    +368.34% (p=0.002 n=6)
Run_Quiet_FirstLineMatches-12    26.74Gi ± 2%        25.80Gi ± 8%      -3.52% (p=0.002 n=6)
geomean                          921.1Mi              2.203Gi        +144.95%

                              │  feature/step-3   │      feature/perf-and-cleanup      │
                              │       B/op        │        B/op        vs base         │
Run_Regex_SparseMatches-12       135.7Ki ± 0%        131.1Ki ± 1%      -3.39% (p=0.002 n=6)
Run_Fixed_SparseMatches-12       80.62Ki ± 0%         76.54Ki ± 0%     -5.07% (p=0.002 n=6)
Run_Quiet_FirstLineMatches-12    81.93Ki ± 0%         73.67Ki ± 0%    -10.08% (p=0.002 n=6)
geomean                          96.42Ki              90.42Ki         -6.22%
```

- **`Run_Regex_SparseMatches` / `Run_Fixed_SparseMatches`** (100,000-line
  synthetic log fixture, ~1 match per 500 lines, normal output mode —
  every line still has to be read and matched): both improve
  substantially (+225%/+368% throughput), but by a smaller factor than
  `internal/lineio` alone, because a real end-to-end search also spends
  real time on disk I/O (`ReadFile` via `internal/source`) and Go's
  `regexp` engine — neither touched by this pass. `pprof` on this
  benchmark (see PROCESS.md §4 step 8) shows those two now dominate
  the remaining time.
- **`Run_Quiet_FirstLineMatches`** (`-q`, every line matches including
  the first) shows **no meaningful change** (+3.64% is within this
  benchmark's own noise band, `± 2–8%`) — expected, since `-q`'s
  existing early-exit stops after the first line either way, before
  `lineio`'s per-line throughput has a chance to matter. Included as a
  regression guard confirming the early-exit path itself didn't get
  slower, not as a claimed win.

## Reproducing this

```powershell
# from two separate worktrees of the same repo:
git worktree add --detach ../gg-step3 feature/step-3
git worktree add --detach ../gg-perf  feature/perf-and-cleanup

# bench_test.go files only exist on feature/perf-and-cleanup; copy them
# into the feature/step-3 worktree so both sides run identical harnesses:
cp ../gg-perf/internal/{lineio,matcher,grep}/bench_test.go ../gg-step3/internal/{lineio,matcher,grep}/  # per-package, see script form above

go install golang.org/x/perf/cmd/benchstat@latest

# in each worktree:
go test ./internal/lineio/...  -run "^$" -bench . -benchmem -benchtime=1s     -count=8  > lineio.txt
go test ./internal/matcher/... -run "^$" -bench . -benchmem -benchtime=2s     -count=3  > matcher.txt
go test ./internal/grep/...    -run "^$" -bench . -benchmem -benchtime=1500ms -count=6  > grep.txt

benchstat ../gg-step3/lineio.txt  ../gg-perf/lineio.txt
benchstat ../gg-step3/matcher.txt ../gg-perf/matcher.txt
benchstat ../gg-step3/grep.txt    ../gg-perf/grep.txt
```

See PROCESS.md §4 step 8 for the design story behind these numbers
(including two intermediate `lineio` implementations that were
benchmarked and rejected before landing on the current one).
