# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## Project

`nitpicker-tui` is a read-only Go TUI for a nitpicker findings store
(`docs/audit/findings/`): it lists the open findings and the resolved ledger,
searches them, filters them by state, lens (the `auditor` field), severity and
category, renders a finding's markdown with glamour, and rescans on `r`.

## Rules that are easy to break

- **Never write the store.** Everything below `main` gets an `fs.FS`. Reads go
  through `os.Root`, and only regular files and directories are read: a
  symlinked root entry, `<auditor>/open` directory or open file, or a FIFO, is
  skipped with a warning, and `findStore` refuses a store reached through a
  symlinked `docs`, `docs/audit` or `docs/audit/findings`, as `findings.py`
  refuses symlinks.
- **Never execute programs at runtime.** forbidigo rejects `exec.Command`.
- **internal/findings imports the standard library only.** depguard in
  `.golangci.yml` fails on any other import; charm modules stay in internal/ui.
- **Everything in the store may come from a hostile clone.** Two filters stand
  between it and the terminal, and both are needed:
  - `clean` (load.go) runs every field through `Sanitize` at load (control
    characters, bidi controls, invalid UTF-8) and turns line breaks and tabs
    into spaces in every field except `Body`, which each draw on one line.
  - `keepStyling` (detail.go) filters glamour's output down to SGR and OSC 8.
    Goldmark decodes character references such as `&#x1b;` after load, and
    bubbletea forwards terminated DCS, APC, SOS and PM sequences to the
    terminal, so sanitized input alone is not enough.
- **A problem with one file is a warning, never a dropped finding.** A file
  without frontmatter is listed under its file name; a bad ledger line is
  reported by line number. `Load` fails only when the root cannot be listed.
- **The parser mirrors nitpicker's `findings.py` (3.1.0).** `parseFrontmatter`,
  `parseFinding`, `splitLines` and `pyStrip` follow `parse_frontmatter`,
  `parse_finding`, `str.splitlines` and `str.strip`. Open files are always
  status `open`; an id falls back to the file name only when the key is
  missing. Ledger lines are split with `splitLines`, decoded by their exact
  keys, and the last record per id wins; the location is `location`, else
  `extra.location`. A store format change there means changing
  `frontmatter.go` and `load.go` and their tests together.
- **`Load` keeps no state.** A rescan is another `Load`; the ledger is not
  append-only (`findings.py` rewrites it on `new --force` and re-resolve), so
  it is always read whole.
- **One rescan at a time.** `rescanning` blocks a second `r` until the first
  `rescanMsg` arrives, so results never overlap or land out of order. A rescan
  reopens an open picker with its typed text and re-renders an open detail or
  warnings view.
- **bubbles list quirks the model works around.**
  - v2.2.1 binds the list's `Quit` to `v` (disabled); `h`/`l` page (removed,
    `l` is the lens picker).
  - `SetFilterState` does not fill the filtered items, so the picker opens by
    sending the list `/`; `SetFilterText` filters but leaves typing mode, so
    `reopenPicker` restores it.
  - The list ends typing mode on its AcceptWhileFiltering keys, so the picker
    empties them and the model handles `enter`, `esc` and the arrows itself.
  - The list replaces filtered rows without moving the cursor, so
    `clampPicker` runs after every key and result.
  - `FilterMatchesMsg` carries no identity, so every picker command is tagged
    with the picker's generation (`tagPicker`) and a closed picker's results
    are dropped.
- **Tests build stores through `findings.Load`** (or `clean` inside the
  package). `Finding.haystack` is unexported, so a struct literal has no
  search text.
- **golangci-lint findings are fixed, not suppressed.** A PostToolUse hook in
  `.claude/settings.json` runs `mise run lint:hook` after every `.go` edit.
- **Coverage is 100% statements, lines and branches.** `mise run coverage`
  fails below it, and CI runs it. Nothing is excluded: a gap is closed by a
  test, or, for code no test can reach, by reshaping it so the failure shares
  a branch tests do reach (as `renderBody` and `refuseSymlinks` do). Branches
  come from gobco, pinned in `mise.toml`; a one-off command for a
  mise-installed tool is run with `mise exec --`.

## Development Commands

```bash
mise install         # Go (go.mod toolchain line) and the pinned dev tools
mise run build       # Build ./nitpicker-tui
mise run test        # Run the full test suite
mise run test-race   # Tests with the race detector
mise run coverage    # race tests; fails below 100% statements, lines, branches
mise run lint        # golangci-lint (.golangci.yml), no fixes: the CI check
mise run lint:fix    # golangci-lint, applying its fixes
mise run lint:all    # every pre-commit hook (prek) on every tracked file
mise run hooks:install  # install the prek pre-commit hook
mise run vuln        # govulncheck
mise run all         # lint + coverage + vuln + build (the CI gates)
go test ./internal/findings/ -run TestParseFinding   # One test
mise exec -- gobco -branch ./internal/ui              # One package's branch coverage
go test -run '^$' -bench . ./internal/findings/       # Filter and Counts benchmarks
go run . testdata/store                               # Browse the fixture store
```

## Architecture

`main.go` parses flags, finds the store by walking up from the given directory
(`findStore`), and builds the loader (`newLoader`) that startup and every
rescan call: `os.OpenRoot` -> `findings.Load(root.FS())` -> close.

- **internal/findings/**: stdlib-only. `Load(fs.FS)` reads
  `<auditor>/open/*.md` and `resolved.jsonl` into a `Store` of findings and
  warnings, each finding through `clean`. `frontmatter.go` parses a finding
  file, `sanitize.go` strips control and bidi characters, `facet.go` holds the
  four filter facets, their value order and `Sort`, and `filter.go` holds
  `Query`, `Filter` and `Counts`.
- **internal/ui/**: bubbletea v2 model with one view on screen at a time (list,
  picker, detail). `model.go` routes keys by view, `picker.go` is the facet
  picker (a second bubbles `list`), `detail.go` renders the header and the
  glamour body through `keepStyling`, `rescan.go` runs the reload as a
  `tea.Cmd`.

## Build & Release

- Releases are CalVer tags (`20*`). `mise run release` tags the next version
  with gh-calver and pushes it; `.github/workflows/release.yml` runs
  goreleaser at the version mise.toml pins (keep the two in step), which
  builds linux, darwin and windows `nitpicker-tui` binaries, signs the
  checksums with cosign keyless, and attaches `LICENSE` and the go-licenses
  `THIRD_PARTY_NOTICES.txt`.
- CI (`.github/workflows/ci.yml`) runs golangci-lint, every pre-commit hook,
  the race tests with the 100% coverage gate (its table in the job summary)
  and the build through mise, `go test -v` on macOS and Windows, and
  govulncheck.
