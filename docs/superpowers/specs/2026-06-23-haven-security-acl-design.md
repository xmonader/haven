# Haven Security: Identity, Access & Secrets — Design Spec

> Status: approved design (session 2026-06-23). Extends `docs/design.md`.
> Scope: two shippable milestones — **M7 Identity & Authz**, **M8 Secrets** — landing *after* M5 (remotes/server).

---

## 0. The Problem (in one paragraph)

Git treats authorization and confidentiality as someone else's job. Permissions live in a hosting platform's private database (GitHub/GitLab) and don't travel with the repo; secrets get dumped into `.env` files that sit in the working tree as plaintext blobs anyone with a clone can read. The result is the two pains this design kills: **"anyone can do anything"** (no portable, enforceable access control) and **"crappy env files"** (secrets are wildly public the moment the repo is cloned, backed up, or pushed). Haven already made *privacy* structural with `haven` vs `branch`; this design generalizes that into a real identity-backed access model and adds cryptographic secrecy — without giving up git's decentralized nature.

---

## 1. Core Insight

**Both pains share one root cause: there is no real identity in the system.** Git and the earlier `git-acl` PoC authenticate with `$USER` and store rules in an unsigned editable file — so authorization is theater and secrets are plaintext. Fix identity once (bind it to keys), and a single foundation drives *both*:

1. **Authorization** — who may read/write which refs and paths, enforced server-side, with *signed* policy the server can verify but cannot forge.
2. **Secrecy** — secrets (and optionally whole branches) encrypted to the people the access layer already authorizes, so a stolen repo copy yields ciphertext, not credentials.

One keypair per actor underpins both. The whole design is "make privacy and policy **cryptographic and portable** instead of procedural and platform-locked."

---

## 2. The Two Axes

The single most important mental model. Confidentiality is **two orthogonal properties**, not one:

| Axis | Question it answers | Enforced by | Values |
|---|---|---|---|
| **Access** | *Who is allowed to see/change this?* | the server (trusted, per-remote) | `public` · `restricted` (a named group) · `haven` (just you) |
| **Secret** | *Is it ciphertext when sitting on disk?* | cryptography (any node, trustless) | `unmarked` (plaintext) · `marked` (encrypted) |

They compose freely, and **encryption recipients always equal the readers of the containing ref.** Grant someone read access and they automatically become a decryption recipient — you never manage "who can decrypt" separately from "who can read."

```
              Access:  public        restricted        haven (private)
            Secret:
         unmarked      normal code    team-visible      your unready code,
                       (git default)  code (staging)    plaintext on your disk
         marked        a .env on a    staging's .env,   sensitive haven: encrypted
                       public branch, encrypted to      at rest, leaks nothing
                       encrypted to   deployers         from a stolen .haven.db
                       ops only
```

---

## 3. Identity

Every actor — human, deploy bot, CI runner — holds **two keys**:

| Key | Type | Purpose | Lives where |
|---|---|---|---|
| Signing key | Ed25519 | *who you are*: signs policy & authenticates to servers | `~/.config/haven/identity` (0600) |
| Encryption key | age / X25519 | *what you can decrypt*: receives encrypted secrets | same |

**Private keys never enter the repo and never reach a server.** Only public keys travel, inside the signed keyring (§4). Identity is self-sovereign: you generate your own keypair (`hv key gen`); there is no central CA, no account system, no identity provider. The keypair that runs `hv init` becomes the repo's first **admin** — its root of trust.

Crypto stack: `filippo.io/age` for encryption (X25519, audited, native multi-recipient — built for exactly this), `crypto/ed25519` (stdlib) for signing. No hand-rolled primitives.

---

## 4. Signed, Portable Policy (Approach A)

Policy lives **in the repo**, as a signed object on a reserved ref (`refs/policy`), not in a server database. This is what makes access control decentralized — it travels with every clone and is verifiable offline. The policy object holds:

```jsonc
{
  "version": 7,
  "parent": "<hash of policy v6>",     // signed hash chain = tamper-evident audit log
  "keyring": {
    "alice": { "sign": "ed25519:…", "enc": "age1…", "status": "active", "added_by": "alice" },
    "deploybot": { "sign": "ed25519:…", "enc": "age1…", "status": "active", "added_by": "alice" }
  },
  "groups": { "deployers": ["alice", "deploybot"] },
  "grants": [
    { "id":"g1", "subject":"*",         "verb":"read",  "resource":"refs/branches/**",      "issued_by":"alice", "sig":"…" },
    { "id":"g2", "subject":"deployers", "verb":"write", "resource":"refs/branches/staging", "issued_by":"alice", "sig":"…" }
  ],
  "secret_paths": [".env", ".env.*", "*.pem", "*.key", "id_rsa", "secrets/**", "**/credentials.json"],
  "secret_refs": ["refs/havens/db-spike"],   // whole branches marked secret
  "sig": "<admin signature over this version>"
}
```

**Trust rules the server (and any verifier) enforces on a policy update:**
1. The version's signature verifies against the signer's `sign` key in the keyring.
2. The signer held an `admin` (or appropriately-scoped `grant`) capability **in the parent version**.
3. `parent` matches the verifier's current head → no forks, no replay.

`hv policy verify` re-walks the entire chain on clone — **clients never trust a server's claim about what the policy is.** Tampering with any field breaks a signature; nobody can edit grants into a stolen DB.

### Grants (capabilities)

A grant is a signed statement `{subject, verb, resource, issued_by, expires?, sig}`:

- **verbs**: `read` · `write` · `force` (rewrite history) · `grant` (delegate within what you hold) · `admin` (edit keyring/policy)
- **resource**: multi-segment ref glob — `refs/branches/feature/**` (fixing the PoC's one-segment `*` bug)
- **default deny**: no matching grant → refused

The access tiers are *derived* from grant patterns, with the `refs.visibility` enum kept as a fast-path + structural backstop:

| Tier | Grant shape |
|---|---|
| public `branch` | `read: *` |
| **restricted ref** | `read: <group>` only |
| private `haven` | `read+write: <owner>` only **and** structural `push`-refusal to team remotes (existing M3 rule, retained as defense-in-depth) |

---

## 5. Secrets — Marking, Not Discipline

A "secret" is **a property you mark**, at two granularities. Marking means "encrypt at rest to whoever can read the containing ref." The user never has to remember a special "save secret" command — marking is declarative and survives in signed policy.

### File-level
`hv secret add <path-or-glob>` records the path in `policy.secret_paths`. From then on, at `hv add`/`commit`, any matching file is **never stored as a plaintext blob** — it is age-encrypted to the current ref's readers and stored as a ciphertext object. Plaintext exists only in your working tree.

- Works on **any** branch: a `.env.production` on public `main` is ciphertext encrypted to ops only — secrecy is decoupled from branch visibility.
- **Default marks ship with `hv init`** (`.env`, `*.pem`, `*.key`, `id_rsa`, …) so the careless "I just dropped a `.env` in there" case is encrypted by default, with zero user action.

### Branch-level
`hv haven create --secret <name>` (or `hv mark-secret <ref>`) records the ref in `policy.secret_refs`. The **entire tree** of that branch is encrypted at rest to the branch's readers. A secret branch leaks **nothing** from a stolen `.haven.db`. Cost: the server cannot read it, so diff/merge/log for that branch run client-side (§7).

### Recipients & rotation
- **Recipients = the ref's current readers**, resolved from policy at encrypt time. The encrypted object records the `policy.version` it was sealed under (consistency point under concurrent membership changes).
- **Revocation is loud and honest.** Removing a reader re-encrypts current secrets to the new set, but *old ciphertext in history stays decryptable by anyone who already held a key*. So `hv secret rotate` forces rotating the actual value, and the CLI warns whenever a ref's recipients have **drifted** from its readers. This limitation is inherent to all encryption (sops/age included); the design surfaces it rather than pretending otherwise.

### Working-tree behavior for non-recipients
Checking out a branch that contains a secret you can't decrypt yields a **`<path> (locked)` marker**, never confusing ciphertext garbage in the tree.

### Safety net (optional, off by default)
A commit-time content scanner (entropy + known-secret regexes, gitleaks-style) can *warn or block* when a secret-looking string lands in a non-secret-classified file — catching stragglers that no pattern matched. This is a backstop, not the mechanism; the mechanism is explicit marks + sane defaults.

---

## 6. Decentralization Model

Haven stays decentralized like git — every clone is whole; commit, branch, haven, mark/read secrets, and verify policy all work **offline** with local keys. The server matters only when two parties exchange data, exactly as in git. What changes is that **two things git leaves procedural become cryptographic and portable**: privacy and policy.

**What's guaranteed where:**

| Guarantee | Holds where? | Enforced by |
|---|---|---|
| Secret confidentiality | **everywhere, trustless** | encryption — no node can read without a private key |
| Policy integrity (no forged grants) | **everywhere, trustless** | signatures — any client detects tampering |
| Write authorization | **per-remote** | each host enforces on its own copy, using the portable signed policy |

**The honest seam:** you cannot force a node you don't control to *enforce* a write rule — anyone can fork and ignore your policy on their own copy, same as git. But regardless of any node's cooperation: nobody can **forge** your policy (no admin key), nobody can **read** your secrets (no private key), and nobody can **push forged history into a host you control** (it rejects unsigned/unauthorized updates).

**What copying a repo does and does not unlock:**

| In the clone | Form | Copying it gives an attacker |
|---|---|---|
| commits/trees/blobs (unmarked code) | plaintext | the code (as in git) |
| policy: keyring, groups, grants | public keys + signatures only | nothing to unlock with; can't sign or decrypt as anyone |
| secret files / secret branches | ciphertext | garbage without an authorized private key |

So: **copying the repo never unlocks secrets; it exposes only unmarked code** — i.e. code you'd already shared with that copy's holder. Mark it secret and even a stolen disk yields nothing.

**Decentralized trade-off, named:** revocation is **eventually-consistent**, not instant. An offline clone learns a key was revoked only when it next syncs the new policy version — you can't reach into every clone on earth. Git has the identical limitation; the signed policy chain just makes "what's current" verifiable when you do sync.

---

## 7. Architecture

### 7.1 New / changed packages (extends `docs/design.md` §6)

```
internal/
  identity/      # keypair gen, load (~/.config/haven/identity), agent integration
  policy/        # policy object: parse, sign, verify chain, grant evaluation
    keyring.go   #   actor -> pubkeys, groups
    grant.go     #   capability model, ref-glob matching (multi-segment)
    chain.go     #   signed hash-chain verify (parent linkage, replay defense)
    eval.go      #   "can actor A do verb V on ref R?" -> bool   (the access decision point)
  secret/        # mark resolution, age encrypt/decrypt, manifest, rotation, drift detection
  scanner/       # optional commit-time secret content heuristics (off by default)
  server/        # EXTENDED: signed-challenge auth, ACL-gated objects/refs, policy endpoint
  remote/        # EXTENDED: signed-challenge client, encrypted-object transfer
```

### 7.2 Storage schema additions (SQLite, extends `docs/design.md` §3)

```sql
-- policy versions form a signed hash chain; head pointer in refs as visibility='policy'
objects(...)                          -- secret ciphertext stored as ordinary content-addressed objects
refs(name, visibility, target, mtime) -- visibility gains 'restricted' and 'policy'; secret marks live in policy, not here
secret_manifest(ref TEXT, name TEXT, object TEXT, policy_version INT,
                PRIMARY KEY(ref, name))   -- maps (ref, logical secret) -> ciphertext object + version sealed under
```

Identity private keys are **never** in `haven.db`. The keyring (public keys) lives inside the signed policy object, not as a separate table — so it travels and is tamper-evident.

### 7.3 The access decision (single choke point)

Every read/write — local checkout, server `GET /objects`, server `POST /refs` — funnels through one function:

```
policy.Eval(actor, verb, ref) -> allow | deny     // default deny
```

This is the *only* place authorization is decided, so it's the only place to audit. Object fetches are gated by **reachability**: `GET /objects/<hash>` is allowed only if `<hash>` is reachable from a ref the actor can `read` — otherwise restricted code leaks via raw hash enumeration (a hole this design explicitly closes).

### 7.4 Data flow — committing a file marked secret

```
hv commit
  └─ for each staged path:
       matches policy.secret_paths OR on a secret_ref?
         ├─ yes → recipients = policy readers of current ref
         │        ciphertext = age.Encrypt(plaintext, recipients)
         │        store ciphertext as object; record secret_manifest(ref,name,hash,version)
         └─ no  → store plaintext blob (normal path)
  └─ build tree/commit; HEAD advances
```

### 7.5 Data flow — push to a team remote

```
hv push origin staging
  └─ client: signed-challenge auth (Ed25519)  → server maps pubkey → actor
  └─ server: policy.Eval(actor, "write", refs/branches/staging)?  → deny ⇒ 403
  └─ server: objects uploaded idempotently (ciphertext stays ciphertext; server can't read secrets)
  └─ server: conditional ref update (parent-hash match)         → atomic, resumable
  └─ structural backstop: a haven ref is refused regardless of grants (needs --force --i-know)
```

---

## 8. Wire Protocol Changes (extends `docs/design.md` §5)

| Endpoint | Change |
|---|---|
| **auth** | Ed25519 **signed-challenge** replaces bearer token. Server issues nonce → client signs → server maps signing pubkey to actor via keyring. No long-lived secret on the wire. |
| `GET /refs` | response **filtered** to refs the actor can `read` |
| `GET /objects/<hash>` | allowed only if reachable from a readable ref (**reachability-gated**) |
| `POST /refs` | server verifies `write`/`force` via `policy.Eval` before the conditional update |
| `POST /policy` | accepts a new signed policy version; verifies signature + admin-in-parent + parent-hash linkage |

Secret ciphertext objects transfer like any other object; the server serves them to anyone who can read the owning ref, but they're useless without keys (defense in depth).

---

## 9. User Journeys

### Journey A — Solo dev with a `.env` they keep leaking
```bash
hv init                       # creates identity keypair; you are admin; ships default secret_paths
echo "DATABASE_URL=…" > .env
hv add . && hv commit -m "app + config"
#   → .env matched a default secret mark → stored ENCRYPTED to you, automatically.
#     Nothing you had to remember. The blob in haven.db is ciphertext.
hv push origin main           # .env travels as ciphertext; safe even on a public remote
```
*Pain killed:* the `.env` is in the repo, versioned, pushable — and never plaintext on disk or wire.

### Journey B — Staging branch with secrets, visible to deployers only
```bash
hv member add alice alice.pub
hv member add deploybot deploybot.pub
hv group create deployers && hv group add deployers alice deploybot

hv branch create staging
hv restrict staging --read deployers     # need-to-know: not wildly public, not just-you
hv secret add config/staging.env         # mark the secret file on this ref
echo "PROD_DB=…" > config/staging.env
hv add . && hv commit -m "staging config"
#   → staging.env encrypted to alice + deploybot (staging's readers). Bob can't read the branch
#     OR decrypt the secret — same ACL governs both.
hv push origin staging
#   → server: bob's pull of staging is filtered out; deploybot's pull succeeds and decrypts.
```
*Pain killed:* "anyone can do anything" — staging and its secrets are scoped to exactly the right people, enforced server-side, portable in signed policy.

### Journey C — Sensitive spike you don't want on any disk in the clear
```bash
hv haven create --secret db-rewrite      # private + encrypted-at-rest, to you only
# … hack on a risky DB migration …
hv add . && hv commit -m "wip"
hv sync personal                         # carried to your other laptop (still ciphertext on disk)
```
*Property:* if either laptop is stolen, the `db-rewrite` branch is unreadable garbage. Later `hv publish db-rewrite --as feature-x` graduates it (decrypt → plaintext public branch).

### Journey D — Revoking a teammate
```bash
hv member revoke deploybot              # new signed policy version; deploybot dropped from deployers
hv secret rotate staging                # re-encrypt to remaining readers + prompt to rotate values
#   → CLI warns: "deploybot held staging secrets in history; ROTATE the actual credentials —
#      old ciphertext remains decryptable by past key-holders."
hv push origin staging
#   → other clones learn of the revocation when they next pull policy (eventually-consistent).
```
*Honesty:* the tool refuses to pretend a crypto-revoke retroactively secures already-shared secrets.

### Journey E — Verifying trust after cloning from an untrusted mirror
```bash
hv clone https://sketchy-mirror/acme/app
hv policy verify                        # re-walks the signed policy chain locally
#   → "policy chain valid: v0(alice,self-signed) → v7. 0 unsigned/forged versions."
#     If the mirror tampered with grants or keyring, verification FAILS here — you don't
#     trust the mirror's word, you trust the signatures.
```

---

## 10. Adversarial Review — Failure Modes Designed For

- **Root-key compromise** → attacker becomes admin-of-everything. Mitigate: root key can stay offline; all admin actions recorded in the signed chain (audit); multi-admin and threshold approval are a v-next path.
- **Object enumeration** → raw `GET /objects/<hash>` could leak restricted code. Closed: fetch is **reachability-gated** by readable refs.
- **Stale secret recipients** → reader removed but old ciphertext in history still decryptable by them. Handled by forced `rotate` + drift warnings; documented as inherent.
- **Policy fork / replay** → parent-hash chain + server rejects non-linear updates.
- **Lost private key** → you can't decrypt your own secrets; **no recovery by design**. Optional opt-in **break-glass escrow recipient** per repo for teams that need it.
- **Server downgrade / strip** → client verifies the signed chain on clone (`hv policy verify`); never trusts the server's assertion of "current policy".
- **Secret committed before being marked** → already plaintext in history. Default-on marks shrink the window to near-zero; scanner warns; history-rewrite needed to purge (same as git-crypt/BFG).
- **Haven leak via push** → structural refusal retained independent of grants (defense in depth).
- **Concurrent membership change vs encrypt** → each ciphertext records the `policy.version` it was sealed under; the version is the consistency point.
- **Unmarked secret in odd path** → optional content scanner blocks/ warns at commit.

---

## 11. Milestones

Security threads in **after M5** (enforcement is meaningless without the server), as two shippable, independently-planned milestones:

| # | Milestone | Delivers |
|---|---|---|
| **M7** | **Identity & Authz** | `hv key`, signed policy object + chain verify, grants, groups, `restrict`, `member`, server signed-challenge auth + ACL-gated refs/objects/policy endpoints. Restricted tier works end-to-end. |
| **M8** | **E2E Secrets** | file marks (`secret add`) + default marks, branch marks (`--secret`/`mark-secret`), age encrypt/decrypt at commit/checkout, manifest, `rotate` + drift detection, locked-file markers, optional scanner. |

Each gets its own spec → plan → implement cycle. This moves "encrypted-at-rest havens" and the `.env`/authz problems **out of `docs/design.md`'s out-of-scope list** and into committed scope.

---

## 12. CLI Surface (additions to `docs/design.md` §4)

```
hv key     gen | show | export-pub                          # self-sovereign identity
hv member  add <actor> <pubfile> | revoke <actor> | list    # keyring (admin)
hv group   create <g> | add <g> <actor...> | list
hv grant   <actor|group> <read|write|force|grant|admin> <ref-pattern> [--expires <when>]
hv revoke  <grant-id>
hv restrict <ref> --read <group>                            # sugar: need-to-know ref
hv policy  show | log | verify | access <ref>               # access explained / chain audit
hv secret  add <path-glob> | list <ref> | rotate <ref> [<name>]   # marking + rotation
hv mark-secret <ref>                                        # encrypt a whole branch at rest
hv haven create [--secret] <name>                           # private (optionally encrypted)
```
`hv secret set/get/export <ref> <K=V>` survive as **optional sugar** for values that should never be a file (CI injection), built on the same recipients-= -readers machinery.

---

## 13. Open Questions (resolve before each milestone)

- **M7 — admin model:** single root admin for v1, or multi-admin from the start? Recommendation: **single root + multiple `admin` grants** for v1; threshold/quorum is v-next.
- **M7 — auth fallback:** signed-challenge only, or bearer token too for simple CI? Recommendation: **signed-challenge only**; CI gets its own actor keypair.
- **M8 — secret branch merges:** three-way merge on encrypted branches must run client-side. Confirm the merge engine can operate purely on decrypted-in-memory trees without server tree-diff.
- **M8 — escrow:** ship the opt-in break-glass escrow recipient in M8, or defer? Recommendation: **defer to v-next** unless a team use case forces it.
- **Default secret-path list:** finalize the shipped defaults (lean vs broad). Recommendation: **lean + documented**, easy to extend.

---

## 14. Definition of Done

On top of `docs/design.md`'s v1 DoD, a user can:

1. `hv init` and have a `.env` committed **encrypted by default**, never plaintext on disk or wire.
2. `hv restrict` a staging branch to a group; non-members can neither read the branch nor decrypt its secrets, enforced server-side.
3. `hv haven create --secret` a spike that is unreadable from a stolen `.haven.db`.
4. `hv member revoke` a teammate, with the tool **forcing value rotation** and warning honestly about history.
5. `hv policy verify` a clone from an untrusted mirror and detect any forged grant or keyring tampering.
6. Confirm — by inspecting the raw DB — that copying the repo yields **ciphertext for every secret** and forged-policy attempts fail signature verification.
