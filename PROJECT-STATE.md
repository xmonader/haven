# Haven — Project State

## Current State
- **Done:** M0 skeleton. **M1** — object model (`internal/object`: blob/tree/commit, content-addressed store, nested tree build/flatten), `internal/hash`, `internal/ref` (CRUD + visibility), `internal/index` (staging), `internal/workspace` (scan), `internal/config` (author). Commands: `init config add commit status log`. End-to-end + unit tests green, vet clean.
  **M2** — `internal/diff` (tree diff + LCS unified diff), `internal/workspace/checkout.go` (materialize tree, clean check), branch create/switch/list/delete, `diff`. Shared `switchTo`/`resolveTree`/`workingTree` helpers. Tests + vet green.
  **M3** — havens (private refs via HavenPrefix/Private), `hv publish` with fast-forward-only divergence refusal, `IsAncestor` commit walk. Havens never appear in branch list. Tests + vet green.
  **M4** — `internal/merge` (three-way content merge via `diff.Chunks`, git-style conflict markers; tree merge), `MergeBase` (LCA), `hv merge` (fast-forward, clean 3-way commit, conflict-to-working-tree), exact-content rename detection in diff. Tests + vet green.
  **M5** — `internal/protocol` (HTTP server+client), `internal/remote`, object reachability/transfer; `serve`, `remote`, `push` (refuses private), `fetch`, `pull`, `clone`, `sync` (carries havens to personal remotes). Team server also refuses private refs. Full VCS with remotes — DoD 1-7 demonstrated. Tests + vet green.
  **M6** — `internal/lock` (working-copy flock), `fsck` (object integrity + chain verify), `gc` (sweep unreachable, preserving policy chain). Tests + vet green.
  **M7 (full)** — `internal/policy`: portable ed25519-signed policy chain (Member keyring, Grants read/write/force/grant/admin with hierarchy, Groups, Restricted refs, SecretRefs/SecretPaths), parent-linked + offline-verifiable. `internal/identity` v2 (age X25519 enc + ed25519 sign). Commands: `key`, `member`, `group`, `grant`/`revoke`/`restrict`, `policy` (show/log/verify/access). Ref-scoped recipients: a secret encrypts to the active readers of its ref. **Server-side enforcement (M7c):** signed-request auth (X-Haven-Pub/Time/Sig, 300s skew) maps key→actor; gates `GET /refs` (filter readable), `GET /objects` (reachability from readable refs), `POST /refs` (write grant; `refs/policy` requires valid chain extension — no history rewrite), `PUT /objects` (members only). No policy = open mode. push/clone/sync carry the chain. Tests + vet green.
  **M8 (full)** — secret branches: `policy.SecretRefs` + ref-aware `marksOf` encrypt a whole ref's tree at rest (`secret ref`, `haven create --secret`). `ScanBaseline` classifies files from the committed tree first (fixes spurious-modified on cross-ref secret entries). `secret rotate` re-encrypts secret objects to current readers without a new commit; `secret status` flags recipient drift via a stored fingerprint. Tests + vet green.
- **Next:** all milestones (M0–M8) delivered. Future polish only (e.g. richer drift baselining on first commit, perf on `readableObject` reachability scan).
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
