# Haven — Project State

## Current State
- **Done:** M0 skeleton. **M1** — object model (`internal/object`: blob/tree/commit, content-addressed store, nested tree build/flatten), `internal/hash`, `internal/ref` (CRUD + visibility), `internal/index` (staging), `internal/workspace` (scan), `internal/config` (author). Commands: `init config add commit status log`. End-to-end + unit tests green, vet clean.
  **M2** — `internal/diff` (tree diff + LCS unified diff), `internal/workspace/checkout.go` (materialize tree, clean check), branch create/switch/list/delete, `diff`. Shared `switchTo`/`resolveTree`/`workingTree` helpers. Tests + vet green.
- **Next:** M3 — havens: `hv haven create/switch/list/delete` (private visibility, reusing refCreate/refSwitch), `hv publish <haven> [--as <branch>]`.
- **Blocked:** nothing.

## Key Decisions
- SQLite via `modernc.org/sqlite` (pure Go, no cgo) — keeps the single static binary promise. (2026-06-23)
- HEAD is a file `.haven/HEAD` holding `ref: <name>`; refs live in the `refs` table. (2026-06-23)
- Module path `haven`; internal-only packages. (2026-06-23)

## Architecture Notes
- `internal/store`: owns the DB connection + schema (WAL, foreign keys, busy_timeout).
- `internal/repo`: `.haven/` layout, Init/Open/discover (walks up for `.haven`), HEAD read/write.
- `internal/cli`: `Command` registry + `Dispatch`; one `cmd_*.go` per subcommand.
- Milestones: M0 skeleton · M1 objects+commit · M2 branches+checkout+diff · M3 havens+publish · M4 merge · M5 remotes · M6 hardening · M7 identity/access · M8 secrets. See `docs/design.md`.
