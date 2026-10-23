# Plan: nitpicker-tui — a terminal browser for the nitpicker findings store

Date: 2026-10-23
Status: DRAFT — awaiting approval to implement
Revision 1 (2026-10-23): severity and category filters added; a picker replaces cycling for every
filter; `r` rescans the store.

## Goal

A read-only Go TUI, `nitpicker-tui`, that loads a project's `docs/audit/findings/` store (open
findings and the resolved ledger) and lists every finding. It searches the findings as you type,
filters them by state, lens, severity and category, and rescans the store when you press `r`. The
values offered by each filter come from the loaded findings, and you choose them in a fuzzy picker.
A selected finding's markdown is rendered with glamour v2, the renderer glow is built on. Lint config
and release pipeline are copied from `ivuorinen/git-dirty`.

## Scope & constraints

### Decisions taken in Understand (user answers, 2026-10-23)

- **Lens = the `auditor` field**: the command that filed the finding, which is also the store's
  directory name. Across the 43 stores on this machine it takes about 35 distinct values. No
  finding records a `lens` field.
- **Renderer = `charm.land/glamour/v2` v2.0.1**, in-process. glow (`charm.land/glow/v3`) is an
  application, and its go.mod renders through exactly this module and version.
- **Scaffolding = mirror git-dirty**: mise tasks, CI, CodeQL, release, goreleaser, golangci,
  editorconfig, prek pre-commit, markdownlint, yamllint, renovate, Claude Code lint hook.
- **Binary = `nitpicker-tui`**. Module: `github.com/ivuorinen/nitpicker-findings-store-tui`.
- **Rev 1: four filters** (state, lens, severity, category), each chosen in a picker. The
  picker replaces the `s/S`/`l/L` cycling from the first draft.
- **Rev 1: `r` does a full rescan**, chosen over incremental ledger reads. The evidence:
  - **The ledger is not append-only.** `findings.py` `write_ledger` (line 871) rewrites the whole
    file in three paths:
    - `new --force` removes a record (line 1409);
    - re-resolve removes a record and re-appends it (lines 1470 and 1482).

    git checkout, pull and rebase can also rewrite the file. A read of the last n lines cannot see
    a removal.
  - **A full rescan is already fast.** On the largest real store (`skills`: a 1.77 MB ledger with
    925 records) it takes about 23 ms:
    - reading the ledger: 0.17 ms;
    - parsing, sanitizing and indexing it: 20.3 ms;
    - re-reading the 144 open files of the store with the most of them (`escobar.digital`):
      2.3 ms.

    At 10× that ledger size (synthetic) the parse takes 202 ms.

### The input contract (read-only consumer of nitpicker 3.1.0 `findings.py`)

Checked against 43 real stores: 202 open files and 4075 ledger records.

- `docs/audit/findings/<auditor>/open/<id>.md`: frontmatter between a leading `---\n` and the
  first `\n---\n`. Keys: `id auditor severity category area status found`, and optionally
  `location location_sha`. Then a `# Title` line and the body (`## Problem/Evidence/Impact/Fix`).
- `docs/audit/findings/resolved.jsonl`: one JSON object per line, with keys
  `id auditor severity category area status found resolved title body` and an optional
  `extra.{location,location_sha}`. Status is `fixed` (3945) or `invalid` (130), and 1367 records
  have an empty `category`. The largest ledger is 1.7 MB; the longest line is 11.8 KB.
- How `findings.py` writes:
  - resolving appends one record (a single `O_APPEND` write plus fsync, line 830) and then
    deletes the open file (line 1488);
  - the rewrite paths replace the file atomically (tmp file plus rename);
  - every writer holds a `flock` on `.lock`.
- Ignored: `INDEX.md`, `baseline.json`, `.lock`, `.gitattributes`, `.gitignore`.
- Parsing rules copied from `findings.py` `parse_frontmatter` / `parse_finding`: `\r\n` becomes
  `\n`; only `key: value` lines that do not start with a space or tab count; one matching
  surrounding quote pair is stripped; the title is the first line starting with `#` and a space.
- Open findings always show status `open`, whatever their frontmatter says (as `findings.py
  list` does). If the frontmatter has no `auditor`, the directory name is used.
- Real stores contain damaged files: `f2b/.../NIT-119.md` and `tests-13080842.md` had their
  frontmatter rewritten into a `## id: …` heading by a markdown formatter. These must still be
  listed, each with a load warning. They must never be dropped silently.

### Toolchain (checked 2026-10-23)

| Item | Version | Source |
| --- | --- | --- |
| Go | `go 1.27.0`, `toolchain go1.27.3` (latest release) | go.dev/dl; local is 1.27.2, GOTOOLCHAIN=auto fetches 1.27.3 |
| bubbletea / bubbles / lipgloss | v2.1.0 / v2.2.1 / v2.0.6 (`charm.land/…/v2`) | proxy.golang.org `@latest`, the same versions git-dirty uses |
| glamour | v2.0.1 (`charm.land/glamour/v2`) | proxy `@latest`; glow v3 pins the same |
| golangci-lint | 2.15.0 via mise (local 2.14.0) | git-dirty mise.toml |
| goreleaser | 2.18.3 via mise | git-dirty mise.toml |

There are 4 direct dependencies and 30 transitive modules. All are MIT or BSD-3-Clause, checked by
running `go-licenses report` against a probe module.

### Must not change / hard rules

- The tool never writes to the store. Everything below `main` receives an `fs.FS`, which has
  no write methods, so it is read-only by construction.
- No `exec.Command` at runtime (git-dirty's forbidigo rule, kept).
- `internal/findings` imports only the standard library (git-dirty's depguard pattern).
- Every string read from the store is sanitized once, at load (see the security lens).

### Package layout

```text
main.go             run(args, stdout, stderr) int: flags, store discovery, the load closure (os.OpenRoot → Load → Close)
internal/findings/  stdlib-only: Finding, frontmatter parser, Sanitize, Load(fs.FS), Facet, Query, Filter, Values, Counts, Sort
internal/ui/        bubbletea model: list, search input, facet picker, detail viewport, rescan; glamour rendering
testdata/store/     synthetic fixture project (docs/audit/findings/...) for main_test and manual runs
```

### UI behaviour

- **Four views:**
  - **list** (the start view);
  - **picker**, opened with a filter key, `esc` closes it;
  - **detail**, opened with `enter`;
  - **warnings**, opened with `w`.

  `esc`, `q` or `backspace` returns from detail and warnings. Only one view shows at a time;
  nothing is overlaid. A finding's body is rendered only when it is opened.
- **List rows:** title is `SEVERITY  <title>`; description is
  `<id> · <auditor> · <status> · <category> · <area>`. Sorted by status (open, fixed, invalid,
  other), then severity (critical → advisory, unknown last), then `found` newest first, then id.
- **List title bar:**
  - lists each filter not set to "all" as `name: value`, then `<n>/<total>`;
  - adds `⚠ <k> warnings (w)` when there are load warnings;
  - shows `rescanning…` while a rescan is running, then `rescanned: <total> findings` or
    `rescan failed: <error>`;
  - is cut to the terminal width.
- **Facets:** `state`, `lens` (`auditor`), `severity`, `category`. One generic `Facet` type
  covers all four, so no facet has code of its own.
  - Each facet's values are the distinct values in the loaded findings, open and resolved. An
    empty value is shown as `(none)`.
  - Order: state as open, fixed, invalid, then others A–Z; severity critical → advisory, then
    unknown values A–Z, then `(none)`; lens and category A–Z, then `(none)`.
  - Defaults: state = `open`, the other three = all.
- **Picker:** a second bubbles `list` with its built-in fuzzy filter (sahilm/fuzzy, which suits
  short labels).
  - It opens already filtering, so typing narrows the values at once.
  - The cursor starts on the current value, which is also marked `✓`.
  - Items are `all (N)` and then `<value> (N)`. N counts findings that match the search text
    and every other active facet, so the numbers show what picking that value would give.
  - `enter` applies the highlighted value. With no match, `enter` does nothing. `esc` closes the
    picker without changing anything.
  - A value that is selected but has no findings after a rescan stays in the picker, with
    count 0.
- **Keys** (list view):
  - `/` focuses search. `enter` keeps the query and leaves the field; `esc` clears it and leaves.
  - `s` opens the state picker, `l` lens, `v` severity, `c` category.
  - `r` rescans, `w` shows warnings, `enter` opens the detail view.
  - `q` and `ctrl+c` quit. `?` shows full help.
- **Search:** case-insensitive substring match. The query is split on whitespace and every term
  must appear (AND) in the finding's id, title, area or body.
- **Rescan (`r`, list view only):**
  - Runs the same load closure that startup uses, as a `tea.Cmd` on its own goroutine, and
    returns a `rescanMsg{store, err}`.
  - Further `r` presses are ignored while one rescan is running.
  - On success: the findings and warnings are replaced; the filters and search text are kept;
    the cursor goes back to the same finding (same id and status) if it is still visible,
    otherwise the index is clamped.
  - On failure: the old data stays on screen and the error is shown in the title bar.
- **Detail:** a lipgloss metadata header (id, severity, category, auditor, area, location, status,
  found, resolved) above the glamour-rendered body. Glamour style is `dark` or `light`, chosen
  from `tea.RequestBackgroundColor`. Word wrap is the terminal width, minimum 20.

## Tasks

TDD throughout: write the test first, watch it fail, then implement (`superpowers:test-driven-development`).
Each task is one commit, and only on your instruction.

1. **Module and lint scaffold.** Files: `go.mod`, `.editorconfig`, `.gitattributes`, `.gitignore`,
    `LICENSE`, `mise.toml`, `.golangci.yml`.
    - go.mod: `go 1.27.0`, `toolchain go1.27.3`.
    - `.editorconfig` and `LICENSE` (MIT, 2026 Ismo Vuorinen) copied as-is.
    - `.gitattributes`: the comment is reworded, since there are no golden files.
    - `.gitignore`: `/nitpicker-tui`; drop `bench.md`.
    - `mise.toml`: git-dirty's file with the binary renamed, and without `hyperfine` and the
      `bench` task. That task benchmarks git-dirty against the bash script it replaced.
    - `.golangci.yml`: copied with these changes:
      - header comment rewritten;
      - forbidigo message: "nitpicker-tui never executes programs; markdown is rendered
        in-process with glamour";
      - depguard rule `internal-findings-stdlib-only` on `**/internal/findings/**` and `!$test`,
        allowing `$gostd`;
      - goimports `local-prefixes`: the module path;
      - the `_test.go` forbidigo exclusion removed, because no test runs a program.
    - Verify: `mise install` succeeds; `golangci-lint config verify` exits 0; `go mod tidy` leaves
      go.mod unchanged.

2. **Frontmatter and finding parser.** Files: `internal/findings/finding.go`,
    `internal/findings/frontmatter.go`, `frontmatter_test.go`.
    - Types: `Finding{ID, Auditor, Severity, Category, Area, Status, Found, Resolved, Location,
      Title, Body, Path, haystack}` and `SeverityRank`.
    - `parseFinding(text) (fm map[string]string, title, body string, ok bool)` follows the
      `findings.py` rules above.
    - Verify: the table test passes for each of these cases:
      - plain frontmatter;
      - CRLF input;
      - a `"quoted"` value, with only one pair stripped;
      - an `id: x` line indented by two spaces, which is ignored;
      - a line without `": "`, which is ignored;
      - no closing `---`, which gives `ok=false` and an empty map;
      - the title taken from the first `# Title` line, not from a `## Heading` line;
      - the body trimmed and ending in exactly one newline.

3. **Sanitizer.** Files: `internal/findings/sanitize.go`, `sanitize_test.go`.
    - `Sanitize(s string) string` drops C0 controls except `\t` and `\n`, `\r`, DEL, C1
      (U+0080–U+009F) and invalid UTF-8 bytes.
    - Verify: the table test passes:
      - the probe's payload `"\x1b]52;c;ZXZpbA==\x07"` (OSC 52 clipboard write) leaves no
        `\x1b` and no `\x07`;
      - `"\u009b31m"` (the 8-bit CSI) is stripped;
      - `"\x7f"` is stripped;
      - `"a\tb\nc"` is unchanged;
      - `"ä 漢字 🙂"` is unchanged.

4. **Store loader.** Files: `internal/findings/load.go`, `load_test.go`.
    - `Load(fsys fs.FS) (Store{Findings []Finding; Warnings []string}, error)`. It reads `.` and,
      for each directory, `<dir>/open/*.md`, then `resolved.jsonl`.
    - `Load` keeps no state between calls, so a rescan is simply another call.
    - The ledger is read whole with `fs.ReadFile`, split on `\n`, and each line goes through
      `json.Unmarshal`. A `bufio.Scanner` would fail on lines over 64 KiB.
    - Only regular files are read (`DirEntry.Type().IsRegular()`); any other entry type gives a
      warning, which is the same refusal of symlinks that `findings.py` makes.
    - Every string field passes through `Sanitize`, then the lowercase search haystack is built.
    - A file without frontmatter is still listed: id = file stem, auditor = directory, severity
      empty, plus a warning.
    - A malformed ledger line gives a warning and loading continues.
    - A missing `resolved.jsonl` is not an error. A store with no findings at all returns an
      empty Store and no error.
    - Verify, using `fstest.MapFS`:
      - two auditors with open files load with the expected fields;
      - a ledger with one malformed line out of three gives 2 findings and 1 warning;
      - a ledger line with a 100 KiB body loads;
      - a ledger body containing the JSON escape `\u001b]52;…\u0007` decodes without ESC or BEL;
      - a file without frontmatter is listed and gives a warning;
      - `open/notes.txt` is ignored;
      - a `resolved.jsonl` entry with `Mode: fs.ModeSymlink` is skipped with a warning.
    - One on-disk test: `t.TempDir()` holding `resolved.jsonl` as a symlink to a file outside
      the root, read through `os.OpenRoot(dir).FS()`. The outside content must never appear in
      the result. The test is skipped if `os.Symlink` fails (Windows without the privilege).

5. **Facets, filter, counts, sort.** Files: `internal/findings/facet.go`, `filter.go`,
    `facet_test.go`, `filter_test.go`, `filter_bench_test.go`.
    - `type Facet int` with the constants `State, Lens, Severity, Category`; `Facet.Of(Finding)
      string`; `Facet.String()`.
    - `type Query{Text string; Sel map[Facet]string}`, where a facet missing from `Sel` means all.
    - `Filter([]Finding, Query) []int` returns indices.
    - `Values([]Finding, Facet) []string` returns the distinct values in the order above.
    - `Counts([]Finding, Query, Facet) (total int, per map[string]int)`: one pass over the
      findings that match the text and every facet except the one asked about.
    - `Sort([]Finding)` implements the row order, using `slices.SortStableFunc`.
    - Verify:
      - table tests pass for each facet alone, for all four combined with text, and for `""`
        selected (`(none)`) matching only empty values;
      - two-term AND: `"race lock"` matches only findings containing both terms;
      - uppercase query text matches lowercase content;
      - an empty Query returns everything;
      - `Values(Lens)` contains an auditor that appears only in the ledger;
      - `Values(Severity)` is in rank order, with an unknown value after `advisory` and `""` last;
      - `Counts(Lens)` with `state=open` counts only open findings and ignores the current lens
        selection;
      - the sort test puts critical before advisory, and an unknown severity last;
      - `go test -bench . -benchmem ./internal/findings`, over 5000 synthetic findings with 2 KiB
        bodies, query `"race condition"` and all four facets set, reports under 16 ms/op (one
        frame) for both `Filter` and `Counts`.

6. **List view.** Files: `internal/ui/model.go`, `keys.go`, `model_test.go`.
    - The model holds `list.Model` (`SetFilteringEnabled(false)`), `textinput.Model`,
      `[]findings.Finding`, the current `Query`, and the view mode.
    - List keymap changes:
      - drop `l` and `h` from `NextPage`/`PrevPage`, so `l` is free for the lens picker;
      - disable the list's own `Quit`, because bubbles v2.2.1 binds it to `v` with the help text
        "select" (`list/keys.go:91-94`);
      - add `/ s l v c r w` through `AdditionalShortHelpKeys`.
    - While search has focus, every key except `ctrl+c` goes to the textinput.
    - Verify: `Update`-driven tests with `WindowSizeMsg` and `KeyPressMsg`, on a fixture of 3
      open, 2 fixed and 1 invalid findings:
      - the start view shows the 3 open findings;
      - `/` then typing `tok` narrows the list;
      - `q` while search has focus appends `q` to the query and does not return `tea.Quit`;
      - `q` in the list returns `tea.Quit`;
      - `v` returns neither `tea.Quit` nor a quit through the list (a regression test for the
        bubbles binding);
      - the title bar contains `state: open` and `3/6`.

7. **Facet picker.** Files: `internal/ui/picker.go`, `picker_test.go`.
    - One `picker{facet findings.Facet; list list.Model}`, built from `Values` and `Counts`, plus
      the current selection if `Values` no longer contains it.
    - It opens with `SetFilterState(list.Filtering)` and `Select(<current>)`.
    - The model catches `enter` and `esc` before the picker list sees them: `enter` applies
      `SelectedItem()` (nil → no-op); `esc` closes without changes.
    - Verify, on a fixture with 3 auditors:
      - `l` shows `all` plus 3 lenses, with the cursor on `all ✓`;
      - typing `sec` narrows to `security`, `enter` applies it, and the list then shows only
        security findings;
      - `esc` after typing leaves the lens unchanged;
      - with `state=open`, every count in the lens picker counts open findings only;
      - `enter` with filter text that matches nothing changes nothing and keeps the picker open;
      - `s` and choosing `all` shows all 6 findings;
      - `c` lists `(none)` when one fixture finding has an empty category;
      - `v` lists severities in rank order.

8. **Detail and warnings view.** Files: `internal/ui/detail.go`, `detail_test.go`.
    - `viewport.Model`; the header comes from lipgloss and the body from
      `glamour.NewTermRenderer(WithStandardStyle(style), WithWordWrap(w))`.
    - The renderer is rebuilt on `WindowSizeMsg` or `BackgroundColorMsg`. If the detail view is
      open, it re-renders and the scroll offset is clamped.
    - If rendering fails, the sanitized raw markdown is shown with a one-line error above it.
    - `w` puts the warnings list in the same viewport.
    - Verify:
      - `enter` shows the title in `View().Content`, and `esc` returns to the list;
      - **end-to-end:** `findings.Load` of a MapFS whose body contains the OSC 52 payload, then
        the model, then `enter`, gives a `View().Content` with no `"\x1b]52"` and no `"\x07"`;
      - after `BackgroundColorMsg` with a light colour, the light style is used;
      - a resize to width 10 does not panic, and the wrap is clamped to 20;
      - `w` lists the fixture's damaged-file warning.

9. **Rescan.** Files: `internal/ui/rescan.go`, `rescan_test.go`.
    - `ui.Run(store, reload func() (findings.Store, error))`.
    - `r` in the list view sets `rescanning` and returns a `tea.Cmd` that calls `reload`.
    - `rescanMsg` clears `rescanning`, applies the result or the error as described above, and
      rebuilds the picker if one is open.
    - Verify, with a fake `reload` returning a prepared Store:
      - `r` returns a non-nil Cmd and the title shows `rescanning…`; a second `r` before the
        message returns nil;
      - running the Cmd and feeding its message back shows the new findings, keeps the filters
        and search text, and leaves the cursor on the same id;
      - a selected finding that was removed leaves the cursor clamped, with no panic;
      - a `reload` error keeps the old list and shows `rescan failed:` in the title;
      - `r` while search has focus types `r`.

10. **Entry point and fixture.** Files: `main.go`, `main_test.go`, `testdata/store/docs/audit/findings/**`.
    - `findStore(start)` walks up from `start` to the first ancestor holding
      `docs/audit/findings/`.
    - `newLoader(dir)` returns the closure used at startup and for each rescan:
      `os.OpenRoot(dir)` → `findings.Load(root.FS())` → `root.Close()` (deferred). No file
      descriptor stays open between loads. Reopening by path means a store directory that git
      replaces is picked up.
    - Exit codes: 0 for success and `-version`; 1 for a missing store, a failed first load or a
      `Program.Run` error; 2 for a usage error (more than one positional argument).
    - `version` is set by `-X main.version` and defaults to `dev`.
    - The fixture holds synthetic findings across 3 auditors, all 3 states, one empty category
      and one damaged file, plus a ledger. It contains nothing copied from real stores.
    - Verify:
      - `run([-version])` prints `dev` and returns 0;
      - two positional arguments return 2 with usage on stderr;
      - `findStore` from `testdata/store/docs/audit` finds the store; from `t.TempDir()` it
        errors;
      - `newLoader` on a `t.TempDir()` store: after a first load, writing a new open file,
        appending a ledger line and deleting an open file, the second load shows all three
        changes;
      - manual check: `go run . testdata/store` shows the list; `/`, `s`, `l`, `v`, `c`, `r`,
        `w` and `enter` behave as specified; `NO_COLOR=1 go run . testdata/store` renders
        without colour.

11. **Release pipeline.** Files: `.goreleaser.yaml`, `notices.tpl`, `.github/workflows/release.yml`.
    - git-dirty's files: `binary: nitpicker-tui`, comments that name this project's dependency
      set (charm TUI libraries, glamour, goldmark, chroma, bluemonday; MIT and BSD-3-Clause),
      the same go-licenses hook, cosign keyless signing of the checksum, the same SHA-pinned
      actions, and tags `20*` (CalVer through `mise run release` and gh-calver).
    - Verify:
      - `goreleaser check` exits 0;
      - `goreleaser release --snapshot --clean --skip=sign` builds the 5 targets (Windows arm64
        is excluded) and writes `THIRD_PARTY_NOTICES.txt`;
      - that file names every module from the license lens, and no module reads `Unknown`;
      - `dist/` stays gitignored.

12. **CI, CodeQL, hooks, repo hygiene.** Files: `.github/workflows/ci.yml`, `.github/workflows/codeql.yml`,
    `.pre-commit-config.yaml`, `.markdownlint.json`, `.yamllint.yml`, `renovate.json`, `.claude/settings.json`.
    - Copied from git-dirty. The `test-os` comment is reworded to the real OS risk here: `os.Root`
      and path handling on Windows and macOS.
    - The pre-commit `testdata/` exclusions are removed, because no fixture is compared byte for
      byte.
    - Verify: `mise run hooks:install`, then `mise run lint:all` (prek: actionlint, zizmor
      `--persona=auditor`, yamllint, markdownlint, editorconfig-checker, shellcheck) exits 0 with
      no warnings.

13. **Docs.** Files: `README.md`, `CLAUDE.md`.
    - README covers install (release binaries, cosign verify, `go install`), usage
      (`nitpicker-tui [dir]`), the four filters and the picker, `r` rescan, and "press `?` for
      every key". The key table lives only in the help generated from the bindings, so the two
      cannot drift.
    - CLAUDE.md uses git-dirty's sections. Its rules:
      - never exec;
      - `internal/findings` is stdlib-only;
      - sanitize at load;
      - never write the store;
      - `Load` is stateless (a rescan is another `Load`);
      - the parse rules mirror `findings.py`, so a nitpicker store-format change means updating
        `frontmatter.go`/`load.go` and their tests together.
    - Verify: markdownlint through prek exits 0; every command shown in README runs as shown.

14. **Final gate.**
    - Verify: `mise run all` (lint with `--fix=false`, `test-race`, `vuln`, `build`) exits 0 with
      zero warnings; `go test ./...` passes on this machine; `editorconfig-checker` is clean.

## Adversarial hardening

- **complexity**
  - First draft: examined every component against the request.
  - Cut in the first draft:
    - exec'ing glow;
    - sahilm/fuzzy as a direct dependency (the picker reaches it through bubbles `list`);
    - fuzzy matching for the main search: subsequence matching over 2 KB bodies matches almost
      any short query, so substring AND is used;
    - cobra/viper (stdlib `flag` covers one flag and one argument);
    - config file and env vars;
    - any write or resolve action;
    - split-pane preview;
    - render cache (3 ms per render, measured);
    - teatest (plain `Update` calls cover the model);
    - the `bench` task and `hyperfine`;
    - the test forbidigo exclusion;
    - the testdata pre-commit exclusions.
  - Rev 1:
    - Added: one generic `Facet`, which handles all four filters with one picker and one
      `Counts`, so there is no per-facet code.
    - Removed: the `s/S`/`l/L` cycling, because the picker replaces it and two ways of
      selecting would be redundant.
    - Rejected for rescan, with measurements:
      - incremental ledger reads: the full parse is 20 ms, and the ledger is rewritten in place
        by three `findings.py` paths;
      - tracking changes per open file: re-reading all of them takes 2.3 ms;
      - taking `findings.py`'s `.lock`: `fcntl` exists only on Unix, so it would need build
        tags, and it would block a rescan while an audit runs.
    - Not added: a clear-all-filters key, which was not requested.

- **review** (edge cases)
  - First draft:
    - missing store → exit 1;
    - empty store → empty list;
    - damaged frontmatter (f2b) → listed with a warning;
    - malformed ledger line → warning;
    - ledger line over 64 KiB → whole-file read;
    - CRLF handling;
    - unknown severity → sorted last;
    - unknown status → visible only under state `all`;
    - the same id open and in the ledger (re-opened) → shown twice, once per state;
    - zero matches → the list's empty state;
    - width under 20 → wrap clamped;
    - resize in detail → re-render with offset clamped;
    - `q` typed into search must not quit;
    - `l`/`h` paging keys rebound.
  - Rev 1 (picker):
    - empty values are shown as `(none)` (1367 ledger records have no category) and can be
      selected;
    - a selected value that disappears after a rescan stays in the picker with count 0;
    - `enter` with no match does nothing (`SelectedItem()` is nil);
    - `esc` after typing leaves the selection unchanged;
    - `v` is the list's Quit binding in bubbles v2.2.1 → Quit disabled, plus a regression test.
  - Rev 1 (rescan):
    - a selected finding that was removed → cursor clamped;
    - a reload error → old data kept;
    - `r` typed into search must not rescan;
    - a rescan finishing while the picker is open → picker rebuilt.

- **security**
  - Trust boundary: store text is written by audits quoting repository content, and a cloned
    third-party repository can ship a crafted store.
  - Sink 1, the terminal. The probe showed that glamour v2.0.1 removes OSC 0 and `CSI 2J` but
    passes `ESC ]52;c;…BEL` (clipboard write) through unchanged. Its sanitizing is bluemonday,
    which handles HTML only. Fix: `Sanitize` at load for every field, including JSON-decoded
    `\u001b` (tasks 3 and 4), plus an end-to-end test through `View()` (task 8). OSC 8 links
    that glamour emits carry only sanitized URLs, so they cannot break out of the sequence.
  - Sink 2, the filesystem. A committed symlink could point outside the store. Fix: read through
    `os.OpenRoot().FS()`, which confines, and read only regular files, as `findings.py` does
    (security-3fbe26a3). Tested on disk (task 4).
  - No network and no exec at runtime: forbidigo enforces the second, and the dependency set
    has no HTTP client in use.
  - Rev 1: no new trust boundary.
    - Picker labels and counts come from store values that were already sanitized at load.
    - A rescan goes through the same `Load`, with the same confinement and regular-file rule.
  - Size limits: accepted risk (see below).

- **errors / leaks**
  - Failure paths:
    - discovery failure → stderr and exit 1;
    - failed first load → exit 1;
    - a per-file read or parse failure → a warning that is counted in the title bar and listed
      under `w`, never dropped;
    - a renderer failure → raw markdown plus an error line;
    - a `Program.Run` error → exit 1.
  - Rev 1: a failed rescan keeps the old data and shows the error, and the app keeps running.
  - The one acquired resource is the `os.Root`. It is opened and closed (deferred) inside every
    load call (task 10), so no descriptor outlives a load; `fs.ReadFile` closes its own files.
  - The only goroutine is bubbletea's, running the rescan Cmd. It finishes when `Load` returns,
    and `Load` is bounded by the store size.

- **migrations**: N/A. There is no schema, database or data migration; the tool reads a store
  it never writes. Rev 1 adds no persisted state.

- **concurrency**
  - First draft: nothing shared. `Load` finished before `tea.NewProgram` started.
  - **Rev 1 changes this.** A rescan runs `reload` on a Cmd goroutine:
    - `reload` touches only its own locals and the filesystem;
    - its result reaches the model only as `rescanMsg`, inside `Update`;
    - the `rescanning` flag blocks a second rescan while one runs, so results cannot overlap or
      arrive out of order (task 9).
  - Writers in other processes (an audit running at the same time):
    - a rescan can land between `findings.py`'s ledger append and its open-file delete, or
      between `new --force`'s rewrite steps, so a finding can show twice (open and fixed) for
      one scan;
    - a half-written last line gives a one-scan warning.

    Both go away on the next `r`. *Accepted* (see below).
  - `test-race` in CI runs the rescan tests.

- **contract**
  - New public surface: the CLI (`nitpicker-tui [dir]`, `-version`, exit codes 0/1/2). This is
    the first release, so there is nothing to stay compatible with, and versions are CalVer
    with no semver bump.
  - Consumed contract: the nitpicker 3.1.0 store format. Its parse rules are copied and tested
    (task 2), and the coupling is recorded in CLAUDE.md (task 13).
  - Rev 1: key bindings are UI, not API.

- **arch**
  - Dependency direction is `main → internal/ui → internal/findings`. findings is stdlib-only,
    enforced by depguard as git-dirty enforces it for `internal/git`; all charm dependencies
    stay in `ui`.
  - The read-only property is structural (`fs.FS`), so it needs no lint rule.
  - Rev 1:
    - `ui` gets `reload` as a function value and never touches the filesystem; `main` owns
      `os.OpenRoot`;
    - facet logic stays in `findings`, where it is stdlib-only and testable without a TUI.

- **perf**
  - Measured: one glamour render of a 1.9 KB body takes 3.1 ms. Rendering happens on
    `enter`/resize only, with no cache.
  - Filtering scans precomputed lowercase haystacks.
  - Rev 1:
    - `Counts` is one O(n) pass, run each time a picker opens;
    - the benchmark gate (task 5) covers `Filter` and `Counts` with all four facets set, under
      16 ms/op at 5000 findings;
    - a full rescan of the largest real store takes 23 ms, and 202 ms at 10× the ledger size;
      it runs off the UI goroutine, so keys stay responsive during it.

- **tests**
  - Every check asserts on observable output: visible item counts, picker items and counts,
    `View().Content`, title-bar text, decoded fields, warnings, exit codes.
  - Real payloads are used: the OSC 52 string, a 100 KiB ledger line, an on-disk escaping
    symlink, and an on-disk rescan after real file changes (task 10).
  - Rev 1 regression tests: the bubbles `v` Quit binding, `r` and `q` typed into search, and a
    rescan pressed while one is running.
  - The fixtures are synthetic.

- **config**
  - No config file and no environment variables are added.
  - NO_COLOR, CLICOLOR and CLICOLOR_FORCE are honoured by bubbletea v2's
    `colorprofile.Detect` (source checked: `tea.go:1111`, colorprofile `env.go`); task 10
    checks this by hand.
  - Tools are pinned in mise.toml. golangci-lint 2.15.0 is built with go1.27.0, which is at
    least go.mod's 1.27.
  - Rev 1 adds no configuration.

- **privacy**: N/A. The tool displays local files read-only, with no network, telemetry or
  on-disk logging. Kept: fixtures are synthetic, not copied from the 43 real stores, which hold
  other projects' internal paths. Rev 1 adds no data flow.

- **a11y**
  - Keyboard-only by design.
  - Severity is shown as a text label, never as colour alone.
  - `?` gives full help.
  - NO_COLOR is honoured.
  - The light or dark style follows the terminal background.
  - Rev 1:
    - the picker marks the current value with `✓` as text, not only by highlight;
    - counts are shown as numbers;
    - rescan status is shown as text in the title bar.

- **i18n**: N/A. No locale scope is declared and the UI strings are English. Kept:
  - the sanitizer must not touch non-ASCII text (test in task 3);
  - display width is left to lipgloss/displaywidth.

- **observability**: N/A. This is an interactive local tool with no logs, metrics or traces.
  Load problems are shown in the UI, and fatal errors go to stderr with an exit code. Rev 1:
  rescan results and errors appear in the title bar.

- **license** (extra)
  - `go-licenses report` on the dependency set gives MIT or BSD-3-Clause for all 30 modules.
    Rev 1 adds no module (sahilm/fuzzy was already a transitive dependency through bubbles).
  - Without a LICENSE file go-licenses reported the main module as `Unknown`, so LICENSE is
    created in task 1, before the notices hook first runs (task 11).

## Rollback / abort

- New repository with nothing deployed: each task is one commit, so abort means stopping, and
  rollback means `git revert` of the task commits.
- No release exists until `mise run release` pushes a `20*` tag.
- A bad release is withdrawn by deleting the GitHub release and its tag; the next CalVer tag
  supersedes it. Nothing is published to a package registry.

## Open questions & accepted risks

- **Layout**: list ↔ detail toggle, glow's model, rather than a split preview pane; the picker
  also takes the full screen instead of an overlay. *Assumed* as the simplest layout.
- **Default state filter is `open`.** *Assumed*: the ledger outnumbers open findings 20:1.
- **Search is substring AND over id, title, area and body.** It is not fuzzy and does not
  rank. *Assumed*. The picker does use fuzzy matching, over short labels.
- **Facet values come from all findings**, open and resolved. They are not narrowed by the
  other filters; the counts show the effect of each value instead. *Assumed*.
- **`r` works only in the list view**, not in detail, picker or warnings. *Assumed*: the result
  shows up in the list.
- **No clear-all-filters key.** Each picker's `all` resets that one filter. *Assumed*.
- **Rescan during a concurrent audit** can show a finding twice, or give one malformed-line
  warning, for one scan. *Accepted*: the next `r` corrects it, and sharing `findings.py`'s
  flock would be Unix-only and would block while an audit runs.
- **Rescan cost grows with the ledger**: 23 ms on the largest real store, 202 ms at 10×.
  *Accepted*: it is an explicit keypress and runs off the UI goroutine.
- **Read-only**: no resolve or edit from the TUI, no automatic reload (`r` rescans on demand),
  `baseline.json` not shown.
- **Format coupling**: a nitpicker store-format change can break parsing. *Accepted*;
  mitigated by the copied parse rules, the tests, and the CLAUDE.md note.
- **No file-size cap**: a crafted multi-GB store can exhaust memory. *Accepted*: it is a local
  viewer, the user can kill it, and the largest real ledger is 1.7 MB.
- **License is MIT**, the same as git-dirty. *Assumed*.
- **Renovate** extends `local>ivuorinen/renovate-config` and **CodeQL** uses the
  `ivuorinen/actions` composite. Both assume the repo is published as
  `github.com/ivuorinen/nitpicker-findings-store-tui`.
- **Local tools**: golangci-lint 2.14.0 and go 1.27.2 differ from the pins (2.15.0 and
  1.27.3). `mise install` and GOTOOLCHAIN=auto resolve both (task 1).
