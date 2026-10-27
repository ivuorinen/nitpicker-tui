# 100% Coverage Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Report statement, line and branch coverage in the Linux CI job's summary and fail below 100% on any of them, after bringing all three to 100%.

**Architecture:** One mise file task, `mise-tasks/coverage`, measures
statements and lines from one `go test -coverprofile` run and branches from
`gobco -branch -stats` per package, prints a table, appends it to
`$GITHUB_STEP_SUMMARY` and exits 1 below 100%. Failures no test can produce
are folded into branches tests reach; `run` and `ui.Run` pass bubbletea
program options through, so tests drive the real program without a
terminal; `main` exits through a variable.

**Tech Stack:** Go 1.27.3, bubbletea v2.1.0, glamour v2.0.1, gobco 1.3.4, mise, bash, awk, jq 1.8.2, GitHub Actions.

**Spec:** `docs/plans/2026-10-27-coverage-design.md`

## Global Constraints

- go.mod keeps `go 1.27.0` and `toolchain go1.27.3`, and its `require`
  blocks do not change: gobco is a mise tool, not a module dependency.
- gobco is pinned as `"go:github.com/rillig/gobco" = "1.3.4"` under
  `[tools]` in `mise.toml`.
- 100% means covered equals total for each measure. No exclusions, no
  `//nolint`, no change to `.golangci.yml`.
- `internal/findings` imports the standard library only. `main.go` imports
  no charm module (`ui.Option` aliases `tea.ProgramOption`); test files may.
- Production passes no `ui.Option`: `main` calls
  `run(os.Args[1:], os.Stdout, os.Stderr)`.
- Tests call `t.Parallel()`, except those that swap process state
  (`exit`, `os.Args`, `t.Chdir`).
- U+0085, U+2028 and U+2029 appear in Go source only as UTF-8 byte escapes:
  `"\xc2\x85"`, `"\xe2\x80\xa8"`, `"\xe2\x80\xa9"`.
- Tools run through `mise run <task>`; a one-off command for a
  mise-installed tool is `mise exec -- <tool>`; `go` needs neither.
- EditorConfig: Go uses tabs; every other file uses 2-space indents, and
  Markdown continuation lines are indented in multiples of 2. The Go
  snippets below are indented with spaces for that reason; the code itself
  is gofmt-formatted.
- A commit is made only on the user's go-ahead, stages explicit paths, runs
  the hooks (no `--no-verify`) and carries no attribution trailer. `main` is
  pushed only in Task 7, on the user's go-ahead.
- The PostToolUse hook runs `mise run lint:hook` after every Go edit; its
  findings are fixed, never suppressed.

## Review Focus

1. Piped stdin at startup: `echo | nitpicker-tui testdata/store` must still
    open the terminal and run, as bubbletea does when it is given no input
    option (`tea.go:1036`). Pinned by the tmux check in Task 7, Step 4.
2. A store `findStore` accepts but that cannot be opened must still print
    `nitpicker-tui: <error>` and exit 1, now that the first load runs inside
    `ui.Run`. Pinned by `TestRunReportsUnreadableStore` in Task 2.
3. A failing test must fail `mise run coverage`, under `go test` and under
    gobco alike, not only lower a number. Pinned by Task 1, Step 6.
4. A local run, with `GITHUB_STEP_SUMMARY` unset, must print the table and
    fail on nothing but the numbers. Pinned by Task 1, Steps 3 and 4.
5. The headless program tests must pass on the macOS and Windows runners.
    bubbletea's own tests drive programs with `bytes.NewReader` input on
    every platform; pinned by the CI watch in Task 7, Step 7.

---

### Task 1: The coverage task

**Files:**

- Create: `mise-tasks/coverage` (executable)
- Modify: `mise.toml` (`[tools]`; remove `[tasks.test-cov]`)

**Interfaces:**

- Produces: `mise run coverage`, which exits 0 only when statements, lines
  and branches each have covered equal to total, and prints the table and
  the uncovered list. Tasks 3 to 6 use it to find what is left.

- [ ] **Step 1: Pin gobco and drop `test-cov`**

  Add `"go:github.com/rillig/gobco" = "1.3.4"` under `[tools]`, with a
  comment that it measures branch coverage for `mise run coverage` because
  Go measures statements only. Remove `[tasks.test-cov]`. Keep `test` and
  `test-race`.

  Run: `mise install && mise exec -- gobco -version`
  Expected: gobco installs and prints its version, 1.3.4.

- [ ] **Step 2: Write `mise-tasks/coverage`**

  - Header: `#!/usr/bin/env bash`, then
    `#MISE description="Race tests with statement, line and branch coverage; fails below 100%"`,
    then `set -euo pipefail`, then
    `: "${MISE_TASK_NAME:?run it with: mise run coverage}"`.
  - A `mktemp -d` work directory, removed by an `EXIT` trap.
  - Packages come from `go list ./...`, not from a list in the script. A
    package is shown as `.` for the module path (`go list -m`) and as
    `./<rest>` otherwise; files are shown relative to the module root.
  - Statements and lines:
    `go test -v -race -covermode=atomic -coverpkg=./... -coverprofile="$tmp/cover.out" ./...`;
    its output goes to stdout and its failure fails the task. One awk pass
    over the profile, after its `mode:` line:
    - a block's key is its `file:SL.SC,EL.EC` field; its statement count
      is the next field; the counts of equal keys are summed (each test
      binary writes its own copy of every block);
    - a block's package is its file's directory;
    - statements: the statement counts of blocks with a count above 0,
      over all statement counts;
    - lines: every line SL to EL of a block with statements is one line of
      its file; a line is covered when every such block spanning it has a
      count above 0;
    - uncovered: each block with count 0, as `file:SL`.
  - Branches: for each package,
    `gobco -branch -stats "$tmp/<n>.json" <pkg>`, its output into
    `$tmp/gobco-<n>.log`, printed to stderr only when gobco fails. A
    non-zero exit (gobco exits 1 when a test fails, checked 2026-10-27), or
    a stats file that is missing or zero bytes, fails the task; `[]` is 0/0.
    Per package, covered is the sum over entries of `TrueCount > 0` plus
    `FalseCount > 0`, and total is twice the entries; uncovered is each
    entry's `Start`, its `Code` and `never true` or `never false`.
  - Output: a Markdown table with the header
    `| Package | Statements | Lines | Branches |`, a row per package and a
    `| Total | ... |` row, each cell `covered/total (P%)` with P to one
    decimal. When anything is uncovered, a `### Uncovered` heading follows
    with one list item per gap. Printed to stdout; appended to
    `$GITHUB_STEP_SUMMARY` when `${GITHUB_STEP_SUMMARY:-}` is not empty.
  - Exit 1 when any measure has covered below total (compare the counts,
    never the rounded percentages).

  Run: `chmod +x mise-tasks/coverage`

- [ ] **Step 3: Check the numbers against the tools**

  Run: `mise run coverage; echo "exit=$?"`
  Expected:
  - `exit=1`, with no `unbound variable` error (`GITHUB_STEP_SUMMARY` is
    unset here);
  - the Total statements percentage equals the total that
    `go tool cover -func` prints for the same profile (90.6% when this
    plan was written);
  - the branch cells read `20/28` for `.`, `134/152` for
    `./internal/findings` and `118/140` for `./internal/ui`;
  - the uncovered list includes `main.go:31` (`main`),
    `internal/ui/model.go:42` (`FilterValue`) and `internal/ui/model.go:122`
    (`Run`).

- [ ] **Step 4: The job summary**

  Run:

  ```bash
  sum=$(mktemp); GITHUB_STEP_SUMMARY="$sum" mise run coverage > "$sum.out"
  tail -n "$(wc -l < "$sum")" "$sum.out" | diff - "$sum"; rm "$sum" "$sum.out"
  ```

  Expected: no diff: the summary file holds the table and the uncovered
  list that stdout ends with.

- [ ] **Step 5: A direct run is refused**

  Run: `./mise-tasks/coverage; echo "exit=$?"`
  Expected: `MISE_TASK_NAME: run it with: mise run coverage` on stderr and
  a non-zero exit.

- [ ] **Step 6: A failing test fails the task**

  In a scratchpad copy (`git ls-files -z | xargs -0 cp --parents -t "$copy"`,
  plus `mise-tasks/coverage`), invert one assertion in
  `internal/findings/sanitize_test.go`.

  Run, in the copy: `MISE_TRUSTED_CONFIG_PATHS="$copy" mise run coverage; echo "exit=$?"`
  Expected: a non-zero exit before any table. gobco's failure path is its
  own exit status, 1 for a failing test, which `set -e` turns into the
  task's. Delete the copy.

- [ ] **Step 7: Lint**

  Run: `prek run --files mise-tasks/coverage mise.toml`
  Expected: every hook passes, shellcheck and editorconfig-checker included.

- [ ] **Step 8: Commit (on the user's go-ahead)**

  ```bash
  git add mise-tasks/coverage mise.toml
  git commit -F <message file>   # build: add mise run coverage with gobco branch coverage
  ```

### Task 2: Drive the real program from tests

**Files:**

- Modify: `internal/ui/model.go:121-126` (`Run`)
- Modify: `main.go:31-33` (`main`), `main.go:37-82` (`run`)
- Test: `internal/ui/model_test.go`, `main_test.go`

**Interfaces:**

- Produces:
  - `type Option = tea.ProgramOption` in `internal/ui/model.go`;
  - `func Run(reload Reload, opts ...Option) error`: calls `reload` once
    and returns its error before any program starts; otherwise runs
    `tea.NewProgram(New(store, reload), opts...)` and returns its error;
  - `var exit = os.Exit` in `main.go`;
  - `func run(args []string, stdout, stderr io.Writer, opts ...ui.Option) int`.

- [ ] **Step 1: Write the failing `ui.Run` tests** (`internal/ui/model_test.go`)

  ```go
  // altScreen is what a starting program writes first.
  const altScreen = "\x1b[?1049h"

  func TestRunQuitsOnQ(t *testing.T) {
    t.Parallel()
    store := loadStore(t, fixtureFiles())
    var screen bytes.Buffer
    err := Run(func() (findings.Store, error) { return store, nil },
      tea.WithInput(strings.NewReader("q")), tea.WithOutput(&screen))
    if err != nil {
      t.Fatalf("Run = %v, want nil after q", err)
    }
    if !strings.Contains(screen.String(), altScreen) {
      t.Errorf("the program never started: output %q", screen.String())
    }
  }

  func TestRunReturnsLoadErrorBeforeStarting(t *testing.T) {
    t.Parallel()
    errGone := errors.New("store gone")
    var screen bytes.Buffer
    err := Run(func() (findings.Store, error) { return findings.Store{}, errGone },
      tea.WithInput(strings.NewReader("q")), tea.WithOutput(&screen))
    if !errors.Is(err, errGone) {
      t.Errorf("Run = %v, want %v", err, errGone)
    }
    if screen.Len() != 0 {
      t.Errorf("a program started after the failed load: %q", screen.String())
    }
  }
  ```

- [ ] **Step 2: Run them to see them fail**

  Run: `go test ./internal/ui/ -run 'TestRun'`
  Expected: a build failure: `Run` is called with a `func` where it takes
  `findings.Store`, and with too many arguments.

- [ ] **Step 3: Implement `Option` and `Run(reload Reload, opts ...Option) error`**

  Update `Run`'s doc comment: it loads the first store through `reload`,
  the loader every rescan uses.

- [ ] **Step 4: Write the failing `main` tests** (`main_test.go`)

  ```go
  // Not parallel: it swaps os.Args and exit.
  func TestMainExitsWithRunsCode(t *testing.T) {
    args, prev := os.Args, exit
    t.Cleanup(func() { os.Args, exit = args, prev })
    os.Args = []string{"nitpicker-tui", "-version"}
    code := -1
    exit = func(c int) { code = c }
    main()
    if code != 0 {
      t.Errorf("main exited with %d, want 0", code)
    }
  }
  ```

  | Test | Call | Assertions |
  | --- | --- | --- |
  | `TestRunBrowsesAndQuits` | `run([]string{"testdata/store"}, &stdout, &stderr, tea.WithInput(strings.NewReader("q")), tea.WithOutput(&screen))` | returns `0`; `stderr.Len() == 0`; `screen` contains `"\x1b[?1049h"` |
  | `TestRunReportsUIErrors` | as above, with `tea.WithContext(ctx)` for a cancelled `ctx` and empty input | returns `1`; `stderr` starts with `"nitpicker-tui: program was killed"` |
  | `TestRunReportsUnreadableStore` | `docs/audit/findings` made under `t.TempDir()`, then `os.Chmod(findings, 0o000)`; `run([]string{project}, ...)` with `q` input | skips on Windows or when `os.Geteuid() == 0`; returns `1`; `stderr` starts with `"nitpicker-tui: "` and contains `"permission denied"`; `screen.Len() == 0` |

  The mode-000 directory is empty, so `t.TempDir`'s cleanup removes it
  without restoring the mode.

- [ ] **Step 5: Run them to see them fail**

  Run: `go test . -run 'TestMain|TestRunBrowses|TestRunReports'`
  Expected: a build failure: `undefined: exit` and too many arguments in
  the calls to `run`.

- [ ] **Step 6: Implement `exit` and the new `run`**

  `main` becomes `exit(run(os.Args[1:], os.Stdout, os.Stderr))`. In `run`,
  after the flag handling, `findStore`'s error and the UI's share one exit
  path:

  ```go
  dir, err := findStore(cmp.Or(flags.Arg(0), "."))
  if err == nil {
    err = ui.Run(newLoader(dir), opts...)
  }
  if err != nil {
    _, _ = fmt.Fprintln(stderr, "nitpicker-tui:", err)

    return 1
  }

  return 0
  ```

  Document `exit` (tests replace it to run `main` in-process) and `opts`
  (tests pass input and output; `main` passes none, so bubbletea keeps its
  own terminal handling, the TTY it opens for piped stdin included).

- [ ] **Step 7: Run the suite**

  Run: `mise run test-race`
  Expected: every package `ok`; `runArgs` and the existing `run` tests
  compile unchanged.

- [ ] **Step 8: Mutation check of the exit path**

  In a scratchpad copy, change the `if err == nil {` above
  `err = ui.Run(` to `if err != nil {`.
  Run, in the copy: `go test . -run TestRunBrowsesAndQuits`
  Expected: FAIL (no `\x1b[?1049h`: the program never started). Delete the
  copy.

- [ ] **Step 9: Commit (on the user's go-ahead)**

  ```bash
  git add main.go main_test.go internal/ui/model.go internal/ui/model_test.go
  git commit -F <message file>   # refactor: let tests drive the program through run and ui.Run
  ```

### Task 3: Route the unreachable failures through reachable branches

**Files:**

- Modify: `main.go` (`refuseSymlinks`), `internal/ui/detail.go:52-62`
  (`renderBody`)
- Test: `internal/ui/detail_test.go`

**Interfaces:**

- Consumes: `mise run coverage` (Task 1).
- Produces: no signature changes.

- [ ] **Step 1: See the gaps**

  Run: `mise run coverage`
  Expected: the uncovered list has `internal/ui/detail.go:54` and `:56`
  (`err == nil` never false) and `refuseSymlinks`' `err != nil` (never true).

- [ ] **Step 2: Write the characterization test** (`internal/ui/detail_test.go`)

  ```go
  func TestRenderBodyFallsBackOnRenderError(t *testing.T) {
    t.Parallel()
    got := renderBody("body \x1b[2Jtext", "no-such-style", 80)
    const want = "render failed: no-such-style: style not found\n\n"
    if !strings.HasPrefix(got, want) {
      t.Errorf("renderBody = %q, want the prefix %q", got, want)
    }
    if !strings.Contains(got, "body text") || strings.Contains(got, "\x1b[2J") {
      t.Errorf("fallback body %q: want the text, without the escape", got)
    }
  }
  ```

  Run: `go test ./internal/ui/ -run TestRenderBodyFallsBackOnRenderError`
  Expected: PASS. It pins today's behavior before the reshape.

- [ ] **Step 3: Reshape `renderBody`**

  The `NewTermRenderer` error and the `Render` error share one branch,
  and there is one return:

  ```go
  r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(wrapWidth(width)))
  var out string
  if err == nil {
    out, err = r.Render(body)
  }
  if err != nil {
    out = fmt.Sprintf("render failed: %v\n\n%s", err, body)
  }

  return keepStyling(out)
  ```

- [ ] **Step 4: Reshape `refuseSymlinks`**

  In the loop, `if err == nil && info.Mode()&fs.ModeSymlink != 0`, set
  `err` to the refusal; then a single `if err != nil { return err }`.
  The doc comment says a failing `Lstat` is returned through the same
  branch.

- [ ] **Step 5: Run the suite and the coverage**

  Run: `mise run test-race && mise run coverage`
  Expected: the tests pass; the three gaps from Step 1 are gone from the
  uncovered list.

- [ ] **Step 6: Mutation checks**

  In a scratchpad copy:
  - delete the line that sets `err` to the refusal in `refuseSymlinks`; run
    `go test . -run TestFindStoreRefusesSymlinkedComponent`; expect FAIL;
  - change `renderBody`'s `if err == nil {` to `if err != nil {`; run
    `go test ./internal/ui/`; expect FAIL.

  Delete the copy.

- [ ] **Step 7: Commit (on the user's go-ahead)**

  ```bash
  git add main.go internal/ui/detail.go internal/ui/detail_test.go
  git commit -F <message file>   # refactor: route unreachable failures through reachable branches
  ```

### Task 4: The main package to 100%

**Files:**

- Test: `main_test.go`

**Interfaces:**

- Consumes: `newLoader`, `findStore` (unchanged signatures).

- [ ] **Step 1: Write the tests**

  | Test | Setup | Assertions |
  | --- | --- | --- |
  | `TestLoaderReportsUnopenableStore` | `newLoader(filepath.Join(t.TempDir(), "missing"))()` | `errors.Is(err, fs.ErrNotExist)` |
  | `TestFindStoreReportsUnresolvableWorkingDir` | not parallel; skip on Windows (it cannot remove a working directory); `os.Mkdir(gone, 0o750)`, `t.Chdir(gone)`, `os.Remove(gone)`, then `findStore(".")` | `errors.Is(err, fs.ErrNotExist)` (`getwd: no such file or directory`), and the error does not contain `"no docs/audit/findings"` |

- [ ] **Step 2: Run them, then the coverage**

  Run: `go test . -run 'TestLoaderReportsUnopenableStore|TestFindStoreReportsUnresolvableWorkingDir' -v && mise run coverage`
  Expected: both PASS; the `.` row reads 100.0% for statements, lines and
  branches. A gap left in `main.go` gets a test of the same kind here.

- [ ] **Step 3: Commit (on the user's go-ahead)**

  ```bash
  git add main_test.go
  git commit -F <message file>   # test: cover the loader and working-directory errors
  ```

### Task 5: internal/findings to 100%

**Files:**

- Test: `internal/findings/filter_test.go` (facets),
  `internal/findings/frontmatter_test.go` (`splitLines`),
  `internal/findings/load_test.go` (loader errors, `failFS`)
- Modify: `docs/plans/2026-10-27-coverage-design.md` (one sentence; see
  Step 3)

**Interfaces:**

- Produces, in `load_test.go`:
  - `var errInjected = errors.New("injected read error")`;
  - `type failFS struct { fstest.MapFS; fail map[string]bool }`, whose
    `ReadFile(name string) ([]byte, error)` returns `errInjected` for a
    name in `fail` and defers to `MapFS.ReadFile` otherwise. `fs.ReadFile`
    prefers a `ReadFileFS`, so overriding `ReadFile` is enough.

- [ ] **Step 1: Write the tests**

  | Test | Input | Assertions |
  | --- | --- | --- |
  | `TestFacetOutOfRange` | `f := Facet(len(Facets))` | `f.String() == ""`; `f.Of(Finding{Status: "open", Auditor: "a", Severity: "high", Category: "c"}) == ""`; `f.order() == nil` |
  | `TestSplitLinesSeparators` | for each `sep` of `"\n"`, `"\r"`, `"\r\n"`, `"\v"`, `"\f"`, `"\x1c"`, `"\x1d"`, `"\x1e"`, `"\xc2\x85"`, `"\xe2\x80\xa8"`, `"\xe2\x80\xa9"`: `splitLines("a" + sep + "b")` | equals `[]string{"a", "b"}`, as Python's `str.splitlines` gives for the same string |
  | `TestLoadReportsUnreadableFiles` | `failFS` over `a/open/A-1.md` and `a/open/A-2.md` (`openFile("id: A-1\n", "One", "")`, likewise `A-2`) and the ledger line `{"id":"R-1","status":"fixed"}`, failing `a/open/A-1.md` and `resolved.jsonl` | the findings are exactly `A-2`; the warnings are, in order, `a/open/A-1.md: injected read error` and `resolved.jsonl: injected read error` |
  | `TestLoadReportsOpenThatIsNotADirectory` | `fstest.MapFS{"a/open": {Data: []byte("x")}}` | no findings; one warning, starting with `a/open:` (MapFS's `ReadDir` fails on a file) |
  | `TestLoadSkipsIrregularLedger` | `fstest.MapFS{LedgerName: {Mode: fs.ModeNamedPipe}}` | no findings; the one warning is `resolved.jsonl: not a regular file; skipped` |
  | `TestLoadLedgerWrongFieldTypes` | ledger lines `{"id":5}`, `{"id":"R-2","extra":5}`, `{"id":"R-3","extra":{"location":5}}`, `{"id":"R-4","status":"fixed"}` | the findings are exactly `R-4`; the warnings start with `resolved.jsonl:1: field "id":`, `resolved.jsonl:2: extra:` and `resolved.jsonl:3: extra: field "location":` |

- [ ] **Step 2: Run them, then the coverage**

  Run: `go test ./internal/findings/ -v -run 'TestFacetOutOfRange|TestSplitLinesSeparators|TestLoadReports|TestLoadSkipsIrregularLedger|TestLoadLedgerWrongFieldTypes' && mise run coverage`
  Expected: all PASS; the `./internal/findings` row reads 100.0% on all
  three. A gap left in the package gets a test of the same kind here.

- [ ] **Step 3: Correct the spec's wording**

  The spec says each loader error "yields a warning, never a dropped
  finding". An unreadable file has nothing to list, and the code skips it
  with a warning, which `TestLoadReportsUnreadableFiles` pins. Replace the
  phrase with "yields a warning naming the file, and every other finding
  still loads".

- [ ] **Step 4: Commit (on the user's go-ahead)**

  ```bash
  git add internal/findings/filter_test.go internal/findings/frontmatter_test.go internal/findings/load_test.go docs/plans/2026-10-27-coverage-design.md
  git commit -F <message file>   # test: cover the loader's error paths and every line break
  ```

### Task 6: internal/ui to 100%

**Files:**

- Test: `internal/ui/model_test.go`, `internal/ui/picker_test.go`,
  `internal/ui/detail_test.go`, `internal/ui/rescan_test.go`

**Interfaces:**

- Consumes: the helpers in `model_test.go` (`newTestModel`, `step`,
  `press`, `typeText`, `update`, `settle`, `visibleIDs`) and
  `rescan_test.go` (`newReloadingModel`, `reloadFrom`, `rescannedFiles`,
  `selectedID`); `type otherMsg struct{}`, declared in `model_test.go`.

- [ ] **Step 1: Write the tests**

  | File | Test | Setup | Assertions | Gap |
  | --- | --- | --- | --- | --- |
  | model_test.go | `TestUnknownMessagesReachTheViewOnScreen` | `otherMsg{}` stepped into the list, a picker (`l`) and a detail view (`enter`) | the mode is unchanged in each; the list's `visibleIDs` are unchanged | model.go:149, 220, 226, 231 |
  | model_test.go | `TestOutOfRangeMode` | `m.mode = modeDetail + 1`, then `View()`, `/`, `otherMsg{}` and `rescanMsg{store: loadStore(t, fixtureFiles())}` | `View().Content == ""` (`View` draws only the three views); after `/`, `m.search.Focused()` (keys, messages and rescans fall back to the list); no panic; the mode is still out of range | model.go:164, 210, 231; rescan.go:68 |
  | model_test.go | `TestBackgroundColorInListSetsStyle` | `tea.BackgroundColorMsg{Color: color.White}` in the list | `m.style == styles.LightStyle`; `m.mode == modeList` | model.go:196 |
  | model_test.go | `TestEnterWithNothingSelected` | `/`, then type `zzzz`, `enter` (accept), `enter` (open) | `m.mode == modeList`; `m.detail == nil` | model.go:295 |
  | model_test.go | `TestSearchKeyThatKeepsText` | `/`, type `token`, then `left` | `m.query.Text == "token"`; `visibleIDs` equal those after typing | model.go:332 |
  | model_test.go | `TestItemFilterValueIsTitle` | `f := findings.Finding{Title: "T"}`; `item{f: &f}` | `FilterValue() == "T"` | model.go:42 |
  | picker_test.go | `TestPickerDownMovesCursor` | `l`, then `down` | `selectedTitle(m) == pickerTitles(m)[1]` | model.go:411 |
  | detail_test.go | `TestDetailKeepsNonBackKeys` | `enter`, then `down` | `m.mode == modeDetail` | model.go:454 |
  | rescan_test.go | `TestRescanRefreshesOpenDetail` | search `secret`, accept, `enter` (detail of `idSecurityOpen`); reload from a copy of `fixtureFiles()` whose `security/open/security-00000002.md` has `# Secret in logs, again` for its title line; `r`, settle | `m.detail.Title == "Secret in logs, again"`; the view shows it | rescan.go:79, 81 (true) |
  | rescan_test.go | `TestRescanKeepsVanishedDetail` | search `stale`, accept, `enter` (detail of `docs-00000003`); reload from `rescannedFiles()`, which drops it; `r`, settle | `m.detail.ID == "docs-00000003"`; the view still shows `Stale README` | rescan.go:81 (false) |
  | rescan_test.go | `TestRescanWithNothingSelected` | search `zzzz` (no rows), accept; reload from `fixtureFiles()`; `r`, settle | no panic; `selectedID(m) == ""` and `len(m.list.Items()) == 0` while the query still matches nothing | rescan.go:56, 102 |

- [ ] **Step 2: Run them, then the coverage**

  Run:

  ```bash
  go test ./internal/ui/ -v -run 'TestUnknownMessages|TestOutOfRangeMode|TestBackgroundColorInList|TestEnterWithNothing|TestSearchKeyThatKeeps|TestItemFilterValue|TestPickerDown|TestDetailKeepsNonBack|TestRescanRefreshesOpenDetail|TestRescanKeepsVanished|TestRescanWithNothing' \
    && mise run coverage; echo "exit=$?"
  ```

  Expected: all PASS; every row and the Total read 100.0% on all three;
  `exit=0`. A gap left in `internal/ui` gets a test of the same kind here.

- [ ] **Step 3: Commit (on the user's go-ahead)**

  ```bash
  git add internal/ui/model_test.go internal/ui/picker_test.go internal/ui/detail_test.go internal/ui/rescan_test.go
  git commit -F <message file>   # test: cover every view's messages, keys and rescans
  ```

### Task 7: Turn the gate on

**Files:**

- Modify: `.github/workflows/ci.yml` (the Linux job's test step, the
  `test-os` test step), `mise.toml` (`[tasks.all]`), `CLAUDE.md`,
  `README.md:75`

**Interfaces:**

- Consumes: `mise run coverage` exiting 0 (Task 6).

- [ ] **Step 1: CI**

  The Linux job's "Test with race detector" step becomes "Test with race
  detector and coverage", running `mise run coverage`. The `test-os` job's
  test step runs `go test -v ./...`. No `permissions` change.

- [ ] **Step 2: `mise.toml` and docs**

  - `[tasks.all]`: `depends = ["lint", "coverage", "vuln", "build"]`, and
    its description names the coverage gate.
  - `CLAUDE.md`, Development Commands: add
    `mise run coverage     # race tests; fails below 100% statements, lines, branches`,
    update the `mise run all` line to `lint + coverage + vuln + build`, and
    add `mise exec -- gobco -branch ./internal/ui   # One package's branch coverage`.
  - `CLAUDE.md`, "Rules that are easy to break": add a bullet: coverage is
    100% statements, lines and branches (`mise run coverage`, gated in
    CI); nothing is excluded; a gap is closed by a test, or by reshaping
    code no test can reach so its failure shares a reachable branch; a
    one-off command for a mise-installed tool is run with `mise exec --`.
  - `README.md:75` becomes
    `mise run all        # lint + race tests with the 100% coverage gate + govulncheck + build (the CI gates)`.

- [ ] **Step 3: The whole gate locally**

  Run: `mise run all && mise run lint:all`
  Expected: both exit 0; the coverage table shows 100.0% everywhere.

- [ ] **Step 4: The real terminal, through tmux**

  Run `go run . testdata/store` in a tmux session: the list shows; `/`
  then `token` narrows it; `esc`; `l` opens the lens picker; `enter`
  applies it; `enter` opens a finding; `esc`; `q` quits with exit 0.
  Then run `echo | go run . testdata/store` (Review Focus 1): the list
  shows and `q` quits, as before this change.

- [ ] **Step 5: Commit (on the user's go-ahead)**

  ```bash
  git add .github/workflows/ci.yml mise.toml CLAUDE.md README.md
  git commit -F <message file>   # ci: gate on 100% statement, line and branch coverage
  ```

- [ ] **Step 6: Push (on the user's go-ahead)**

  Run: `git push`

- [ ] **Step 7: Watch CI**

  With `gh` through context-mode, wait for every run on the pushed commit.
  Expected: CI and CodeQL succeed; the Linux job's summary carries the
  coverage table at 100.0%; the macOS and Windows test logs list every test
  with `-v`, `TestRunQuitsOnQ` and `TestRunBrowsesAndQuits` passing there
  (Review Focus 5), and `TestFindStoreReportsStatErrors`,
  `TestFindStoreReportsUnresolvableWorkingDir` and
  `TestRunReportsUnreadableStore` showing as `SKIP` on Windows. A failure is
  diagnosed from the log and fixed before this task closes.
