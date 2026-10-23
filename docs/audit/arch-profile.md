# Architecture Profile

Generated: 2026-10-23

## Detected Patterns

### Layered / N-Tier — Medium confidence

Evidence:

- Two internal packages and a composition root: `internal/ui` (presentation:
  bubbletea model, views, key routing) over `internal/findings` (core: the
  `Finding` model, loading, parsing, sanitizing, filtering, sorting), wired by
  `main.go`.
- Dependency direction is strictly top-down: `main` imports `internal/ui` and
  `internal/findings`; `internal/ui` imports `internal/findings` and the charm
  modules; `internal/findings` imports the standard library only (`go list`).
- The boundary is declared and enforced, not only observed: CLAUDE.md
  ("internal/findings imports the standard library only", "charm modules stay
  in internal/ui") and the `internal-findings-stdlib-only` depguard rule in
  `.golangci.yml`.
- Ambiguity that keeps it at Medium: the directories are not named for layers
  (`presentation/`, `business/`, `data/`), and there are only two of them.

### Hexagonal / Ports & Adapters — Low confidence

Evidence:

- `findings.Load` depends on `fs.FS` rather than the OS filesystem; `main`
  supplies the adapter (`os.OpenRoot(dir).FS()`), tests supply
  `fstest.MapFS`.
- `ui.Reload` is a function-typed port the presentation layer calls and `main`
  implements.
- No `ports/` or `adapters/` structure and no `*Port` / `*Adapter` names, so this
  is an idiom inside the layered structure, not the organising pattern.

## Detected Combination

Layered (presentation over core, composition root in `main`), with
dependency-injected I/O at the core boundary.

## Inferred Structural Rules

- `internal/findings` imports the standard library only and never imports
  `internal/ui`.
- `internal/findings` reads only through the `fs.FS` it is given; it never opens
  paths through `os`.
- `internal/ui` never touches the filesystem (no `os` or `io/fs` access); store
  data reaches it as a `findings.Store` and rescans go through `ui.Reload`.
- `main.go` is the only place that opens the store (`os.OpenRoot`) and the only
  place that wires `internal/ui` to `internal/findings`.
- Nothing below `main` writes: the core receives a read-only `fs.FS`.

## Ambiguities & Contradictions

None observed. The rules above hold in the current import graph.
