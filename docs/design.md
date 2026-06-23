# Haven (`hv`) — Design

> Haven: a safe harbor for code before it is ready for the public world.
>
> A from-scratch version control system with **structural privacy** and **built-in secrets** — private branches, need-to-know sharing, and encrypted credentials are first-class, not bolted on.

---

## Locked Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Format | **Greenfield, no git interop** | Full control over the private/public model and the security layer |
| Language | **Go** | Single static binary, good CLI ergonomics, fast to ship |
| Storage | **SQLite + content-addressable objects** | Single-file repos, transactions, queryable, zero-config |
| Scope | **Full VCS with remotes + identity/access/secrets** | Not a toy — privacy and secrets are the reason it exists |
| Crypto | **age (X25519) for encryption, Ed25519 for signing** | Audited, native multi-recipient, no hand-rolled primitives |

---

## 1. The Core Insight

Mainstream VCS treats every branch the same and delegates *who-can-do-what* and *secrets* to a hosting platform. Privacy becomes procedural ("don't push that branch!"), permissions live in a vendor's private database and don't travel with the repo, and secrets get dumped into `.env` files that sit in the working tree as plaintext anyone with a clone can read.

**Haven makes three things structural and portable instead of procedural and vendor-locked:**

1. **Privacy** — two ref kinds enforced at the storage and protocol layers:

   | Concept | Visibility | In `hv push`? | Lives where |
   |---|---|---|---|
   | **branch** | public | yes | team remote |
   | **haven** | private | **refused** | personal remote only (via `hv sync`) |

2. **Access** — who may read/write which refs, bound to cryptographic identity, carried in **signed policy that travels with the repo** and is verifiable offline. A server enforces it but cannot forge it.

3. **Secrets** — credentials are **encrypted to the people the access layer already authorizes**. A stolen clone yields ciphertext, never keys.

`hv publish <haven> [--as <branch>]` is a one-way graduation. `hv push` physically cannot leak a haven. `hv sync` carries havens between *your* machines without ever making them team-visible. Everything else is VCS plumbing.

---

## 2. What's Kept vs Thrown Out

Keep the proven storage primitives; throw out the procedural ref/workflow model.

| Keep | Discard |
|---|---|
| Content-addressable objects (SHA-256) | One undifferentiated ref namespace |
| Snapshot DAG of commits | Privacy/permissions by discipline |
| Tree objects (shared unchanged subtrees) | Identity by `$USER` |
| Three-way merge | Plaintext secrets in the working tree |
| Refs as pointers | Detached HEAD (always on a ref) |
|  | Loose-object + packfile format (use SQLite) |

---

## 3. The Two Axes (the core mental model)

Confidentiality is **two orthogonal properties**, not one. Internalize this and the whole system follows:

| Axis | Question | Enforced by | Values |
|---|---|---|---|
| **Access** | *Who may see/change this?* | the server (trusted, per-remote) | `public` · `restricted` (a named group) · `haven` (just you) |
| **Secret** | *Is it ciphertext on disk?* | cryptography (any node, trustless) | `unmarked` (plaintext) · `marked` (encrypted) |

They compose freely, and **encryption recipients always equal the readers of the containing ref** — grant someone read access and they automatically become a decryption recipient. You never manage "who can decrypt" separately from "who can read."

```
              Access:  public          restricted          haven (private)
       Secret:
   unmarked            normal code      team-visible code   your unready code,
                       (the default)    (e.g. staging)      plaintext on your disk
   marked              a .env on a      staging's .env,     sensitive haven: encrypted
                       public branch,   encrypted to        at rest — leaks nothing
                       encrypted to     deployers           from a stolen repo file
                       ops only
```

---

## 4. Identity

Every actor — human, deploy bot, CI runner — holds **two keys**:

| Key | Type | Purpose | Lives where |
|---|---|---|---|
| Signing | Ed25519 | *who you are*: signs policy, authenticates to servers | `~/.config/haven/identity` (0600) |
| Encryption | age / X25519 | *what you can decrypt*: receives encrypted secrets | same |

**Private keys never enter the repo and never reach a server.** Only public keys travel, inside the signed policy. Identity is self-sovereign — you generate your own keypair (`hv key gen`); there is no central CA or account system. The keypair that runs `hv init` becomes the repo's first **admin** — its root of trust.

---

## 5. Signed, Portable Policy

Access policy lives **in the repo** as a signed object on a reserved ref (`refs/policy`) — not in a server database. This is what keeps access control decentralized: it travels with every clone and verifies offline. Shape:

```jsonc
{
  "version": 7,
  "parent": "<hash of policy v6>",          // signed hash chain = tamper-evident audit log
  "keyring": {
    "alice":     { "sign": "ed25519:…", "enc": "age1…", "status": "active" },
    "deploybot": { "sign": "ed25519:…", "enc": "age1…", "status": "active" }
  },
  "groups": { "deployers": ["alice", "deploybot"] },
  "grants": [
    { "id":"g1", "subject":"*",         "verb":"read",  "resource":"refs/branches/**",      "issued_by":"alice", "sig":"…" },
    { "id":"g2", "subject":"deployers", "verb":"write", "resource":"refs/branches/staging", "issued_by":"alice", "sig":"…" }
  ],
  "secret_paths": [".env", ".env.*", "*.pem", "*.key", "id_rsa", "secrets/**", "**/credentials.json"],
  "secret_refs":  ["refs/havens/db-rewrite"],
  "sig": "<admin signature over this version>"
}
```

**A new policy version is accepted only if** (1) its signature verifies against the signer's key in the keyring, (2) the signer held an `admin`/`grant` capability **in the parent version**, and (3) `parent` matches the verifier's current head (no forks, no replay). `hv policy verify` re-walks the entire chain on clone — **clients never trust a server's claim about the policy.** Tampering with any field breaks a signature; nobody can edit grants into a stolen database.

### Grants

A grant is a signed statement `{subject, verb, resource, issued_by, expires?, sig}`:

- **verbs**: `read` · `write` · `force` (rewrite history) · `grant` (delegate within what you hold) · `admin` (edit keyring/policy)
- **resource**: multi-segment ref glob — `refs/branches/feature/**`
- **default deny**: no matching grant → refused

The access tiers derive from grant patterns:

| Tier | Grant shape |
|---|---|
| public `branch` | `read: *` |
| **restricted ref** | `read: <group>` only |
| private `haven` | `read+write: <owner>` only, plus structural `push`-refusal to team remotes |

---

## 6. Secrets — Marking, Not Discipline

A "secret" is **a property you mark**, at two granularities. Marking means "encrypt at rest to whoever can read the containing ref." There is no special "save a secret" command to remember — marks are declarative and live in signed policy.

**File-level** — `hv secret add <path-or-glob>` records the path in `secret_paths`. From then on, at `hv add`/`commit` any matching file is **never stored as a plaintext blob**: it is age-encrypted to the current ref's readers and stored as ciphertext. Plaintext exists only in your working tree.
- Works on **any** branch — a `.env.production` on a public branch is ciphertext encrypted to ops only. Secrecy is decoupled from branch visibility.
- **Default marks ship with `hv init`** (`.env`, `*.pem`, `*.key`, …) so a dropped-in `.env` is encrypted by default, zero user action.

**Branch-level** — `hv haven create --secret <name>` (or `hv mark-secret <ref>`) records the ref in `secret_refs`. The branch's **entire tree** is encrypted at rest to its readers; a stolen repo file yields nothing. Cost: the server can't read it, so diff/merge/log for that branch run client-side.

**Recipients = the ref's current readers**, resolved at encrypt time. Each ciphertext records the `policy.version` it was sealed under (the consistency point under concurrent membership changes).

**Revocation is honest.** Removing a reader re-encrypts current secrets to the new set, but old ciphertext in history stays decryptable by anyone who already held a key. So `hv secret rotate` forces rotating the actual value and the CLI warns when a ref's recipients have drifted from its readers. This is inherent to encryption; the tool surfaces it rather than pretending.

**Non-recipients** who check out a branch with an undecryptable secret get a `<path> (locked)` marker — never confusing ciphertext in the tree.

---

## 7. Decentralization Model

Haven stays decentralized: every clone is whole; commit, branch, haven, mark/read secrets, and verify policy all work **offline** with local keys. A server matters only when two parties exchange data. What changes is that **privacy and policy become cryptographic and portable** instead of procedural and vendor-locked.

| Guarantee | Holds where? | Enforced by |
|---|---|---|
| Secret confidentiality | **everywhere, trustless** | encryption — no node can read without a private key |
| Policy integrity (no forged grants) | **everywhere, trustless** | signatures — any client detects tampering |
| Write authorization | **per-remote** | each host enforces on its own copy, using the portable signed policy |

**The honest seam:** you can't force a node you don't control to *enforce* a write rule — anyone can fork and ignore policy on their own copy. But regardless of any node: nobody can **forge** your policy (no admin key), **read** your secrets (no private key), or **push forged history into a host you control** (it rejects unauthorized updates).

**What copying a repo unlocks:**

| In the clone | Form | Copying it gives an attacker |
|---|---|---|
| commits/trees/blobs (unmarked) | plaintext | the code |
| policy: keyring, groups, grants | public keys + signatures only | nothing to unlock with |
| secret files / secret branches | ciphertext | garbage without an authorized private key |

**Copying the repo never unlocks secrets; it exposes only unmarked code** — code you'd already shared with that copy's holder. Mark it secret and even a stolen disk yields nothing.

**Trade-off, named:** revocation is **eventually-consistent**, not instant — an offline clone learns of a revoked key only when it next syncs the new policy version.

---

## 8. Storage — SQLite

Single file, transactions, queryable, zero-config. Schema:

```sql
objects(hash TEXT PRIMARY KEY, type TEXT, size INT, content BLOB)  -- blob/tree/commit; secret ciphertext is just an object
refs(name TEXT PRIMARY KEY, visibility TEXT, target TEXT, mtime INT)  -- visibility in {public, restricted, private, policy}
remotes(name TEXT PRIMARY KEY, url TEXT, kind TEXT)  -- kind in {team, personal}
config(key TEXT PRIMARY KEY, value TEXT)
staging(path TEXT PRIMARY KEY, hash TEXT)
secret_manifest(ref TEXT, name TEXT, object TEXT, policy_version INT, PRIMARY KEY(ref, name))
```

- **Hash:** SHA-256. **Repo layout:** `.haven/haven.db` + `.haven/HEAD` + `.haven/wclock`.
- Identity private keys are **never** in `haven.db`. The keyring (public keys) lives inside the signed policy object, so it travels and is tamper-evident.
- **Tradeoff:** SQLite BLOB storage gets sluggish past ~GB objects. Acceptable for v1; large-file offload is v2.

---

## 9. Project Structure (Go)

```
haven/
  cmd/hv/main.go             # entrypoint, subcommand dispatch
  internal/
    cli/                     # one cmd_*.go per subcommand, parse.go
    hash/                    # sha256 helpers
    object/                  # blob/tree/commit + SQLite store
    repo/                    # open/init, paths
    index/                   # staging area
    workspace/               # working-copy scan, checkout, locked-file markers
    ref/                     # ref CRUD, visibility
    merge/                   # three-way merge (client-side for secret refs)
    diff/                    # tree diff, unified diff
    identity/                # keypair gen/load, agent integration
    policy/                  # keyring, grants, signed-chain verify, eval (the access decision)
    secret/                  # mark resolution, age encrypt/decrypt, manifest, rotation, drift
    scanner/                 # optional commit-time secret heuristics (off by default)
    remote/                  # HTTP client; signed-challenge auth
    protocol/                # wire format
    server/                  # HTTP server: signed-challenge auth, ACL-gated endpoints
    config/                  # hv config
  go.mod
  Makefile
  README.md
  docs/design.md
  testdata/
```

**Single access decision point:** every read/write — local checkout, `GET /objects`, `POST /refs` — funnels through `policy.Eval(actor, verb, ref) -> allow|deny` (default deny). The only place to audit authorization.

---

## 10. CLI Surface

```
hv init | clone | config

hv status | add | reset | commit [-m] [--amend] | diff | log | show <ref>:<path>

hv branch  create|switch|list|delete <name>          # PUBLIC
hv haven   create [--secret] | switch|list|delete <name>   # PRIVATE (optionally encrypted at rest)
hv publish <haven> [--as <branch>]                   # graduate private -> public

hv merge <ref>                                       # three-way, conflicts to working tree

# identity & access
hv key     gen | show | export-pub
hv member  add <actor> <pubfile> | revoke <actor> | list
hv group   create <g> | add <g> <actor...> | list
hv grant   <actor|group> <read|write|force|grant|admin> <ref-pattern> [--expires <when>]
hv revoke  <grant-id>
hv restrict <ref> --read <group>                     # need-to-know sharing
hv policy  show | log | verify | access <ref>        # access explained / chain audit

# secrets
hv secret  add <path-glob> | list <ref> | rotate <ref> [<name>]
hv mark-secret <ref>                                 # encrypt a whole branch at rest

# remotes
hv remote  add <name> <url> --kind team|personal
hv push    [<remote>] [<branch>...]   # refuses private refs by default
hv pull | fetch
hv sync    [<remote>]                  # personal remote; carries havens
hv serve   [<addr>]                    # run a haven host

# maintenance
hv fsck                                # verify object integrity + policy chain
hv gc                                  # drop unreachable objects (keeps delta bases)
hv repack                              # delta-compress similar objects, then VACUUM
```

`hv secret set/get/export <ref> <K=V>` survive as optional sugar for values that should never be a file (CI injection), on the same recipients-=-readers machinery.

---

## 11. Wire Protocol — HTTPS REST

Plain HTTP(S), debuggable with curl. Auth is a **per-request Ed25519 signature** (no server round-trip, no long-lived secret on the wire): the client signs the canonical string `method\npath\ntime\nsha256(body)\nnonce` with its signing key; the server maps the signing pubkey to an actor via the keyring, rejects a clock skew beyond ±300s, and rejects a nonce it has already seen (durable `seen_nonces` table) so a captured request can't be replayed even within the window. The body hash binds the signature to the payload, so a tampered body invalidates it. Request bodies are capped (`MaxRequestBytes`) so a client can't exhaust server memory. Run behind TLS for confidentiality — the signature prevents forgery, not eavesdropping.

```
GET  /info                  -> repo metadata
GET  /refs                  -> ref listing, FILTERED to refs the actor can read
GET  /objects/<hash>        -> raw object bytes, allowed only if reachable from a readable ref
PUT  /objects/<hash>        -> idempotent upload (ciphertext stays ciphertext; server can't read secrets).
                               Rewriting a secret's ciphertext requires write access to a ref reaching it.
POST /refs (conditional)    -> atomic compare-and-swap ref update; server verifies write/force via policy.Eval.
                               A policy ref update instead verifies the incoming signed chain extends the current one.
```

Object fetch is **reachability-gated**: `<hash>` is served only if reachable from a ref the actor can `read`, so restricted code can't leak via raw hash enumeration. A "haven host" is any HTTP(S) server speaking this; team and personal remotes use the same protocol.

---

## 12. Milestones (each shippable + tested)

| # | Milestone | Lands |
|---|---|---|
| **M0** | Skeleton: `hv init`, SQLite schema, CLI dispatch, `make build/test` | scaffolding |
| **M1** | Objects + commit: `add`/`commit`/`status`/`log`, HEAD | usable locally |
| **M2** | Branches + checkout + `diff` between refs | branching works |
| **M3** | **Havens + `publish`** (visibility, push-refusal) | the haven moment |
| **M4** | Three-way merge, conflicts, exact-content rename detection | can integrate work |
| **M5** | Remotes: HTTP server, `remote add --kind`, `push`/`pull`/`fetch`/`sync`/`clone`; push refuses private | full VCS with remotes |
| **M6** | `fsck`, `gc`, working-copy locking, perf on 10k files/10k commits | production-ish |
| **M7** | **Identity & Access**: `hv key`, signed policy + chain verify, grants, groups, `restrict`, signed-challenge auth, ACL-gated server | enforceable, portable access |
| **M8** | **Secrets**: file/branch marks + defaults, age encrypt/decrypt, manifest, `rotate` + drift, locked-file markers | encrypted credentials |

M7/M8 land after M5 — enforcement is meaningless without the server.

**Status: M0–M8 are implemented and tested**, plus a hardening pass beyond the original plan: body- and nonce-bound request signatures with a durable replay table, atomic compare-and-swap ref updates, at-rest zlib compression, `PRAGMA user_version` schema versioning with a migration runner, a request-body size cap, ref-write-gated secret rewrites, symlink tracking, and the history porcelain below. Linear `rebase`, `cherry-pick`/`revert`, `stash`, and `bisect` (originally out of scope) are built; interactive rebase and octopus merge remain out.

---

## 13. Adversarial Review — Failure Modes Designed For

- **`hv push` leaking a haven** → structurally refused; `--force --i-know` to override. Tests prove refusal.
- **Object enumeration** → raw object fetch is reachability-gated by readable refs.
- **Root-key compromise** → root key can stay offline; all admin actions in the signed chain; multi-admin/threshold is v-next.
- **Policy fork/replay** → parent-hash chain; server rejects non-linear updates.
- **Server downgrade/strip** → clients verify the signed chain on clone; never trust the server's assertion.
- **Stale secret recipients** → forced `rotate` + drift warnings; old history stays decryptable by past holders (documented, inherent).
- **Lost private key** → no recovery by design; optional opt-in break-glass escrow recipient (v-next).
- **Secret committed before marking** → already plaintext in history; default-on marks shrink the window; rewrite needed to purge.
- **Concurrent `hv commit`** → SQLite tx for objects; `.haven/wclock` (flock) for working-copy ops.
- **Publishing a haven whose public twin diverged** → refuse, require explicit merge.
- **Sync conflict (two laptops, same haven)** → v1 refuses, instructs manual merge.
- **Large files** → objects are zlib-compressed at rest (incompressible data stored raw, never enlarged) and `hv repack` delta-compresses similar objects, but each object is still loaded whole into memory per operation; streaming reads are future work (§16). Fine for source trees, not huge binaries — documented limitation.
- **Memory-exhaustion DoS** → request bodies are capped at `MaxRequestBytes` before buffering.
- **Secret lock-out** → a member can read a secret but cannot rewrite its ciphertext to lock others out: a content-changing rewrite requires write access to a ref reaching the secret; identical bytes stay idempotent.
- **Schema drift across versions** → the database carries `PRAGMA user_version`; migrations run forward on open, and a database newer than the binary is refused rather than mis-read.
- **Empty repo / first commit / no HEAD** → every command handles gracefully.
- **Truncated/corrupt SQLite** → `hv fsck` detects; restore from backup, no silent repair.
- **Filenames** → UTF-8 NFC normalize; tree stores mode; regular, executable, and symlinks (stored as their target, mode 120000) tracked.
- **Mid-push network failure** → idempotent upload + conditional ref update; safe to resume.

---

## 14. Explicitly Out of Scope (v1)

Hunk-level staging · signed commits (GPG/SSH) · submodules · LFS / large-file offload · streaming reads for huge single blobs · interactive rebase · octopus (>2-parent) merge · subcommand aliases · git interop · GUI/TUI · patch-review flow · multi-admin threshold approval · break-glass escrow · key revocation lists beyond the signed-chain `status` field.

(Linear `rebase`, `cherry-pick`/`revert`, `stash`, and `bisect` were originally listed here but are now implemented.)

---

## 15. Definition of Done for v1

A user can, end to end and without git installed:

1. `hv init` a repo, commit files, see history — with a dropped-in `.env` committed **encrypted by default**, never plaintext on disk or wire.
2. `hv haven create scratch` and commit work **structurally invisible** to `hv push`; `hv publish scratch --as feature-x` to graduate it.
3. `hv restrict staging --read deployers` so non-members can neither read the branch nor decrypt its secrets, enforced server-side.
4. `hv haven create --secret db-rewrite` — unreadable from a stolen `.haven.db`.
5. `hv serve` on one machine, `hv clone` from another; `hv sync` private havens between two personal laptops.
6. `hv member revoke` a teammate (tool forces value rotation) and `hv policy verify` a clone from an untrusted mirror, detecting any forged grant.
7. `hv push` to a team remote with certainty no haven leaks.
```

---

## 16. Delta storage (built) and streaming reads (future)

**Status: delta encoding + compaction shipped; streaming reads remain future.**
Objects are stored whole and zlib-compressed by default, and `hv repack` now
delta-compresses similar objects so successive versions of a file share bytes.
The remaining *scale* gap is per-object memory (each object is still loaded
whole) — addressed by streaming reads (step 3), still future. None of this is a
correctness or security defect.

**1. Delta encoding (built).** `internal/store/delta.go` implements a
copy/insert delta (`MakeDelta`/`ApplyDelta`); `EncodeDelta`/`DecodeDelta` in
`codec.go` wrap a delta as stored content (a `codecDelta` tag, the base hash,
the compressed delta program). The object hash stays over the *reconstructed*
content, so addressing, the secret plaintext-hash, and the wire protocol are
untouched — `uploadReachable`/the server's `getObject` both read via
`store.Get`, which reconstructs, so the wire only ever carries whole objects.
`ApplyDelta` validates every offset/length and rejects malformed or truncated
deltas. Reconstruction is bounded by `maxDeltaDepth`.

**2. Packing/compaction (built).** `hv repack` (`internal/cli/cmd_repack.go`)
walks whole blobs/trees/commits, groups them by type and size, and stores each
against a recent same-type base when that shrinks it, then `VACUUM`s. Safety
invariants: bases stay full objects (read chains are depth-1); every rewrite is
self-verified to reconstruct byte-for-byte before it is committed and is a no-op
if it would not shrink the object (never bloats); `gc` retains the base of every
reachable delta (`addDeltaBases`) since delta links are invisible to the object
graph. Secrets and the signed policy chain are left whole. Loose objects remain
the write path; repack is opt-in background compaction.

**3. Streaming reads (future).** Add an `io.Reader`-returning accessor so large
blobs need not be fully resident; the codec and delta-reconstruction become
streaming. This is what actually removes the memory-per-object ceiling — the one
remaining scale item.

**Acceptance (met for 1–2):** a multi-revision repo shows a materially smaller
`.haven/haven.db` after `hv repack` (measured 98304→86016 bytes on a 5-revision
fixture), `hv repack` is idempotent, and `hv fsck` still verifies every object's
content hash after reconstruction.
