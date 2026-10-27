# Design: 100% statement, line and branch coverage, reported and gated in CI

## Goal

The CI run that executes the tests reports statement, line and branch
coverage, per package and in total, and fails when any of the three is below
100%. The project is brought to 100% on all three first, so the gate never
turns `main` red. CI test runs print `go test -v` output, so skips and their
reasons show in the log.

Success means:

- the Linux job's run summary carries one coverage table with the three
  measures for `.`, `./internal/findings`, `./internal/ui` and the total;
- `mise run coverage` exits non-zero below 100% on any measure, locally and in
  CI, and lists the uncovered blocks and branches as `file:line`;
- every gap is closed by a test of real behavior, or by reshaping code no test
  can reach; nothing is excluded from measurement.

## Decisions (user answers, 2026-10-27)

- `-v`: yes, in CI. A failing test's output is already complete without it;
  `-v` adds skips with their reasons and which tests ran. Local `mise run test`
  stays quiet.
- Code no test can reach as it stands: reshape first, so an unreachable
  failure shares a branch tests reach; add an injection point only where the
  path is real but needs a terminal or a process exit.
- Producing and posting: one mise file task (`mise run coverage`); the job
  summary, not a PR comment or an external service.
- `test-race` stays as a task; `test-cov` is removed.
- Condition coverage (each `&&`/`||` operand both ways) is not gated.
- This spec lives in `docs/plans/`, next to the implementation plan.

Assumptions stated to the user and not corrected: the gate applies to pushes
and pull requests alike; coverage is measured on Linux only (macOS and
Windows run the tests, not the measurement); only non-test code is measured.

## Facts this design rests on (checked 2026-10-27)

- Go 1.27.3 measures statements only: `-covermode` is `set`, `count` or
  `atomic`. There is no line or branch mode; branch coverage is the open
  proposal golang/go#70306.
- gobco v1.3.4 (BSD-2-Clause, last push 2026-05-03) measures branch coverage
  with `-branch` and condition coverage by default. It treats `a && b` as one
  branch in `-branch` mode.
  - It does not accept `./...`; it runs one package per call.
  - It exits 0 when branches are uncovered, so the gate reads its
    `-stats <file>` JSON. Each entry has `TrueCount` and `FalseCount`;
    covered outcomes over twice the entries reproduce its printed total
    (134/152 for `internal/findings`, checked).
- bubbletea v2.1.0 (`tea.go:1036-1043`): with no input option it reads
  `os.Stdin`, and opens the TTY when stdin is not a terminal. An explicit
  `tea.WithInput(os.Stdin)` skips that fallback. An explicit
  `tea.WithOutput(os.Stdout)` behaves as the default.
- Spike (throwaway, deleted), three runs under `-race`: a program given
  `tea.WithInput(strings.NewReader("q"))` and `tea.WithOutput(io.Discard)`
  starts without a terminal, quits on `q` and returns nil; one given a
  cancelled `tea.WithContext` returns `program was killed: context canceled`.
- glamour rejects an unknown standard style name with a "style not found"
  error from `NewTermRenderer`.
- mise (2026.10.19 here, v2026.10.8 in CI): a task runs with the project's
  tools and environment without shell activation, and gets `MISE_TASK_NAME`
  in its environment. With a bare `PATH`, `jq` resolves to the system
  `/usr/bin/jq` and `go`, `golangci-lint` and `govulncheck` are missing;
  under `mise exec` all four come from mise's installs at the pinned
  versions.
- Go: go.mod's `toolchain go1.27.3` line, with `GOTOOLCHAIN=auto` (the
  setting here), makes any Go from 1.21 switch to 1.27.3; with
  `GOTOOLCHAIN=local`, an older Go refuses the `go 1.27.0` line rather than
  build. Plain `go` commands are pinned without mise.

## Starting point (measured 2026-10-27)

| Measure | Covered |
| --- | --- |
| Statements (`-coverpkg=./...`) | 90.6% |
| Branches (gobco `-branch`) | 272/320: main 20/28, findings 134/152, ui 118/140 |
| Conditions (gobco, not gated) | 335/392 |

## Design

### 1. The coverage task and CI

`mise-tasks/coverage`, a bash file task with a `#MISE description=` header.
It runs only through `mise run coverage`, which puts the tools pinned in
`mise.toml` first on `PATH`, the environment `mise exec` builds, so the
commands inside need no `mise exec` of their own. Its first command,
`: "${MISE_TASK_NAME:?run it with: mise run coverage}"`, refuses a direct
run, which would use whatever `jq` and `gobco` the shell has.

1. `go test -v -race -covermode=atomic -coverpkg=./... -coverprofile=<tmp>/cover.out ./...`.
    `-coverpkg` counts each package's statements from every package's tests.
2. The profile holds one copy of each block per test binary, so the blocks
    are merged first, summing counts per block. One awk pass over the merged
    profile then gives, per package and in total:
    - statements: covered statements over all statements, weighted by each
      block's statement count, as `go tool cover` computes them;
    - lines: each source line spanned by a block with statements counts once,
      and is covered when every such block on it ran.
3. `gobco -branch -stats <tmp>/<pkg>.json <pkg>` for `.`,
    `./internal/findings` and `./internal/ui`; jq totals the covered outcomes.
    A gobco failure, or a stats file that is missing or empty, fails the task.
4. A Markdown table (package | statements | lines | branches, each as
    covered/total and %, plus a total row) goes to stdout and, when
    `$GITHUB_STEP_SUMMARY` is set, to the job summary. Below 100%, the
    uncovered blocks and branches follow as `file:line`.
5. Exit 1 when any measure is below 100%.

`mise.toml`:

- add `"go:github.com/rillig/gobco" = "1.3.4"` under `[tools]`;
- remove `test-cov`; keep `test` and `test-race`;
- `all` runs `coverage` in place of `test-race` (`coverage` is the race run).

`.github/workflows/ci.yml`:

- the Linux job's test step runs `mise run coverage`;
- the macOS and Windows jobs run `go test -v ./...`;
- no new permissions: writing the job summary needs none.

### 2. Production changes (behavior unchanged)

`main.go`:

- `var exit = os.Exit`; `main` calls `exit(run(os.Args[1:], os.Stdout, os.Stderr))`.
- `run(args []string, stdout, stderr io.Writer, opts ...ui.Option) int`
  passes `opts` to `ui.Run`. `main` passes none, so bubbletea keeps its own
  terminal handling, the piped-stdin TTY fallback included.
- `findStore`'s error and the UI's error share one exit path:
  `if err == nil { err = ui.Run(newLoader(dir), opts...) }`, then one
  `if err != nil`. A missing store, a cancelled context and a normal quit
  reach every side.
- `refuseSymlinks`: `if err == nil && <symlink> { err = <refusal> }`, then
  one `if err != nil`. A failing `Lstat`, reachable only when the path
  vanishes between the `Stat` and the `Lstat`, is still returned, through
  the branch the symlink test exercises.

`internal/ui`:

- `type Option = tea.ProgramOption`, so `main.go` imports no charm module.
- `Run(reload Reload, opts ...Option) error` loads the first store through
  `reload`, the loader every rescan already uses, then runs the program
  with `opts`. A load error returns before the program starts; `main`
  prints it as today.
- `renderBody`: the renderer-building error and the render error share one
  `if err != nil`; an unknown style name exercises it.

No change to `facet.go`, `load.go`, `frontmatter.go`, `model.go` or
`rescan.go`: their gaps are reachable from tests.

### 3. Tests

Each asserts behavior, not only that a line ran. Each folded branch in
section 2 gets a mutation check in a scratch copy: with the branch broken,
its test must fail.

`main_test.go`:

- `main()` through `exit`, with `os.Args` set to `-version`: exit 0
  (not parallel; restores `os.Args` and `exit`).
- `run` on the fixture store with `ui.Option`s: input `q` gives 0; a
  cancelled context gives 1 and `program was killed` on stderr.
- `newLoader` after the store directory is deleted: the `OpenRoot` error.
- `findStore(".")` from a deleted working directory (`t.Chdir`, not
  parallel, skipped on Windows, which cannot delete one): the `Abs` error.

`internal/findings`:

- an out-of-range `Facet`: `String` and `Of` give `""`, `order` gives nil;
- `splitLines` on every `str.splitlines` separator, adding `\v`, `\f`,
  `\x1c`-`\x1e`, U+0085 and U+2029, written as UTF-8 byte escapes so no raw
  separator lands in a source file;
- an open directory that fails to read (a file in its place), an open file
  and the ledger that fail to read (a fault-injecting `fs.FS` wrapping
  `fstest.MapFS`), and a ledger that is not a regular file; each yields a
  warning naming the file, and every other finding still loads;
- ledger lines whose `id`, `extra` or `extra.location` has the wrong JSON
  type: a warning per line.

`internal/ui`:

- `Run` without a terminal: input `q` returns nil (covering `Init`); a
  failing `reload` returns its error before the program starts;
- `renderBody` with an unknown style: "render failed", the error and the
  body, through `keepStyling`;
- an unknown message, a resize and a background-color reply in each of the
  list, picker and detail views; down in the picker; a non-Back key in
  detail; a search key that leaves the text unchanged; Enter on an empty
  list;
- a rescan from the warnings view, with the detail finding gone, with an
  empty store, and with no selection.

### 4. Order

1. Section 2's changes, each test-first, then lint and `mise run test-race`.
2. Section 3's tests, package by package, watched with `mise run coverage`
    (the task exists from here; CI does not call it yet).
3. The CI change from section 1, once a local `mise run coverage` reports
    100% on all three.
4. A real-terminal check through tmux: start, `/`, `l`, `enter`, `q`, and
    `echo | nitpicker-tui testdata/store` for the TTY fallback.
5. Docs:
    - CLAUDE.md:
      - commands: add `mise run coverage`, drop `test-cov`, and make `all`
        run lint + coverage + vuln + build;
      - a rule under "Rules that are easy to break": 100% statements, lines
        and branches, no exclusions, a gap closed by a test or by reshaping
        unreachable code;
      - a one-off command for a mise-installed tool is written with
        `mise exec --` (such as `mise exec -- gobco -branch ./internal/ui`
        for one package's branches); `go` commands need none.
    - README line 75 (`mise run all`): name the coverage gate.
6. Commits in groups, each green on its own, each made only on the user's
    go-ahead.

## Rollback

Every step is its own commit; reverting the CI commit removes the gate
without touching the code or tests.

## Risks

- gobco is one maintainer's tool with no release since 2024-03. If it stops
  working with a future Go, the branch column fails the gate loudly, never
  silently: the task fails when a stats file is missing or empty.
- The deleted-working-directory test depends on POSIX `getcwd` failing for an
  unlinked directory; it is measured on Linux, where that holds.
- `exit` and `os.Args` are process-wide; the test that swaps them does not
  run in parallel and restores both.
