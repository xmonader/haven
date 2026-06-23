# Haven — Project State

## Current State
- **Done:** M0 — skeleton: `go.mod`, `Makefile`, SQLite schema (`internal/store`), repo init/open/discover (`internal/repo`), CLI dispatch (`internal/cli`), `hv init`. Build + tests green.
- **Next:** M1 — objects (blob/tree/commit) + `add`/`commit`/`status`/`log`, HEAD advancement.
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
