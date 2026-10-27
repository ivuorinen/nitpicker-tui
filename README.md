# nitpicker-tui

A terminal browser for a [nitpicker](https://github.com/ivuorinen/skills)
findings store: the open findings under `docs/audit/findings/<lens>/open/` and
the resolved ledger `docs/audit/findings/resolved.jsonl`. Search them as you
type, filter by state, lens, severity and category, read a finding rendered as
markdown, and rescan the store while an audit runs.

It only reads. It never changes the store.

## Install

Download a binary from the
[releases](https://github.com/ivuorinen/nitpicker-tui/releases).
Each release's checksums file is signed with cosign, keyless, by the release
workflow. To verify a download:

```sh
cosign verify-blob \
  --bundle nitpicker-tui_<version>_checksums.txt.bundle \
  --certificate-identity-regexp '^https://github.com/ivuorinen/nitpicker-tui/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  nitpicker-tui_<version>_checksums.txt
sha256sum --ignore-missing -c nitpicker-tui_<version>_checksums.txt
```

With Go installed:

```sh
go install github.com/ivuorinen/nitpicker-tui@latest
```

To build from source:

```sh
mise install        # Go (from go.mod) and the dev tools
mise run build      # ./nitpicker-tui
```

## Usage

```sh
nitpicker-tui [-version] [dir]
```

`dir` defaults to the current directory. Like git looking for `.git`, it uses
the `docs/audit/findings` of `dir` or of the nearest directory above it.

The list starts on the open findings, most severe first. Press `?` to see every
key.

- **Search** (`/`): every word must appear in the finding's id, title, area or
  body, in any case. `enter` keeps the query, `esc` clears it.
- **Filters**: `s` state, `l` lens (the command that filed the finding), `v`
  severity, `c` category. Each opens a picker of the values the store holds,
  with the number of findings each would show under the other filters. Type to
  narrow it, `↑`/`↓` to move, `enter` to apply, `esc` to leave it unchanged.
  Pick `all` to clear a filter; `(none)` matches findings without a value.
- **Read** (`enter`): the finding's fields and its markdown body. `esc`, `q` or
  `backspace` goes back.
- **Rescan** (`r`): reads the store again and keeps the filters, the search
  and the finding under the cursor.
- **Warnings** (`w`): files and ledger lines that could not be read cleanly.
  They are listed, never dropped; the title bar shows how many there are.

Text from the store is shown with terminal control sequences and bidi
controls removed, including sequences spelled as markdown character references
(`&#x1b;`), and symlinks are refused both inside the store and in the path to
it. The store quotes repository content, so a cloned repository cannot use it
to control your terminal or to read files outside the store.

## Development

```sh
mise run all        # lint + race tests with the 100% coverage gate + govulncheck + build (the CI gates)
mise run test       # tests
mise run lint:all   # every pre-commit hook on every file
go run . testdata/store   # browse the fixture store
```

## License

MIT. See [LICENSE](LICENSE).
