# Haven (`hv`)

**A version control system where privacy and secrets are built in — not bolted on.**

Haven is a from-scratch VCS for code that isn't ready for the public world yet. Private branches, need-to-know sharing, and encrypted credentials are first-class concepts, enforced by storage and cryptography rather than by discipline.

> Status: a working VCS with remotes and encrypted secrets. See [`docs/design.md`](docs/design.md).

### What works today

A complete local + networked VCS with a signed-policy ACL and encrypted secrets:

- **Local:** `init` · `add` · `commit` · `status` · `log` · `diff` · `config` · `reset` · `restore` · `tag` (symlinks tracked)
- **History:** `cherry-pick` · `revert` · `rebase` (linear, roll-back on conflict) · `stash` (save/list/pop) · `bisect` (start/good/bad/reset)
- **Branches & havens:** `branch` · `haven` (private) · `publish` · `merge` (three-way, rename-aware, conflict markers)
- **Remotes:** `serve` (HTTP/HTTPS) · `remote` · `push` (refuses havens) · `fetch` · `pull` · `clone` · `sync` (carries havens between your machines)
- **Identity & access:** `key` · `member` · `group` · `grant`/`revoke` · `restrict` · `policy` — a **portable, ed25519-signed policy chain** in the repo is the authorization root. Grants (`read`/`write`/`force`/`grant`/`admin`) are verifiable offline; `restrict` removes public access for need-to-know refs. The server **enforces** it: each request is signed over method+path+time+body+nonce (a captured signature can't be replayed against a different body, nor reused verbatim — the server rejects seen nonces within the skew window), maps the key to a keyring actor, and gates ref listing, object fetch, ref updates, and policy extension.
- **Secrets:** `secret add`/`ref`/`rotate`/`status` — files matching a mark (`.env`, `*.pem`, … by default) are **encrypted to the ref's current readers on `add`** and decrypted on checkout; `secret ref` / `haven create --secret` encrypt a whole branch's tree at rest; `rotate` re-encrypts to the current readers after a membership change (no new commit) and `status` flags recipient drift. The object store and any server hold only ciphertext.

Hardening: `fsck` · `gc` · working-copy flock; objects are **zlib-compressed at rest**; the database is **schema-versioned** (`PRAGMA user_version`, migration runner, refuses a DB newer than the binary); the server **caps request bodies** (no memory-exhaustion DoS) and **gates secret-ciphertext rewrites** behind ref write access. Verified end-to-end — ciphertext at rest, non-recipient lockout, offline policy verification, tamper/replay/history-rewrite rejection, restricted-ref hiding from anonymous clients.

---

## Why

Mainstream VCS gets two things wrong, and you pay for both daily:

- **Privacy is a sticky note.** Every branch is equal; keeping one private means *remembering* not to push it. One slip and your half-baked work is public.
- **Secrets are plaintext.** A `.env` in the working tree becomes a plaintext blob in every clone, backup, and push. Access rules live in a hosting vendor's database and don't even travel with the repo.

Haven makes all three structural:

| | Mainstream VCS | Haven |
|---|---|---|
| **Privacy** | procedural ("don't push it") | a **haven** ref `hv push` physically refuses to send |
| **Access** | in a vendor's database, doesn't travel | **signed policy in the repo**, verifiable offline, server-enforced |
| **Secrets** | plaintext `.env` in every clone | **encrypted to whoever can read the branch** |

It stays decentralized like git — every clone is whole, everything works offline — but privacy and policy become *cryptographic and portable* instead of procedural and vendor-locked.

---

## The model in one picture

Confidentiality is **two independent things**:

- **Access** — *who's allowed?* `public` · `restricted` (a group) · `haven` (just you). Enforced by the server.
- **Secret** — *is it ciphertext on disk?* `unmarked` (plaintext) · `marked` (encrypted). Enforced by crypto.

They compose, and the rule that ties them together: **a secret's decryption recipients are exactly the people who can read its branch.** Grant read access and decryption comes with it — you never manage two lists.

---

## Quick start

```bash
hv init                          # new repo (seeds default secret marks: .env, *.pem, …)
hv key gen                       # generate your encryption identity; you join the repo's members
echo "DATABASE_URL=…" > .env
hv add . && hv commit -m "app"   # .env is encrypted automatically (matches a default mark)
hv remote add origin <url> --kind team
hv push origin main              # the secret travels as ciphertext — safe even on a public remote
```

Private, unready work:

```bash
hv haven create scratch          # private branch; hv push will refuse to leak it
# … commit freely …
hv publish scratch --as feature  # one-way graduation to a public branch when ready
```

Sync a private haven between your own machines:

```bash
hv remote add laptop <url> --kind personal
hv sync laptop                   # carries havens (and branches) to your personal remote only
```

Need-to-know tiers and whole-branch encryption are built too:

```bash
hv group create deployers && hv group add deployers bob
hv restrict staging --read deployers   # removes public read on 'staging'
hv haven create db-rewrite --secret    # whole tree encrypted at rest
```

Secrets encrypt to a ref's **current readers** (not the whole keyring); see `docs/design.md` for the full signed-policy model.

> **Not yet:** delta/packfile storage (objects are compressed but stored whole), Windows (`flock`),
> interactive rebase, and octopus (>2-parent) merge. See the limitations in [`docs/userguide.md`](docs/userguide.md).

---

## What copying the repo gives an attacker

| In the clone | Form | They get |
|---|---|---|
| unmarked code | plaintext | the code (as in git) |
| policy (keyring, grants) | public keys + signatures | nothing to unlock with |
| secret files / branches | ciphertext | garbage without an authorized private key |

**Copying the repo never unlocks secrets.** Private keys live in `~/.config/haven/identity` — never in the repo, never on a server.

---

## Install & develop

```bash
make build        # build the hv binary
make test         # run tests
make lint         # static analysis
```

Single static Go binary; SQLite-backed single-file repos (`.haven/haven.db`, schema-versioned, objects zlib-compressed at rest); SHA-256 objects; `age` (X25519) encryption and Ed25519 signatures.

---

## Documentation

- [`docs/userguide.md`](docs/userguide.md) — practical tour: install, the two-axis model, and walkthroughs of every workflow.
- [`docs/design.md`](docs/design.md) — full design: the two axes, signed portable policy, secrets, decentralization model, architecture, protocol, milestones, and adversarial review.
- [`docs/case-study.md`](docs/case-study.md) — engineering case study: the hard parts, key decisions and trade-offs, and war stories.
- [`docs/deepdive-signed-policy.md`](docs/deepdive-signed-policy.md) — deep dive into the offline-verifiable signed access-policy chain.
