# Haven (`hv`)

**A version control system where privacy and secrets are built in — not bolted on.**

Haven is a from-scratch VCS for code that isn't ready for the public world yet. Private branches, need-to-know sharing, and encrypted credentials are first-class concepts, enforced by storage and cryptography rather than by discipline.

> Status: design complete, implementation in progress. See [`docs/design.md`](docs/design.md).

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
hv init                          # new repo; generates your keypair, you become admin
echo "DATABASE_URL=…" > .env
hv add . && hv commit -m "app"   # .env is encrypted automatically (default secret mark)
hv push origin main              # the secret travels as ciphertext — safe even on a public remote
```

Private, unready work:

```bash
hv haven create scratch          # private branch; hv push will refuse to leak it
# … commit freely …
hv publish scratch --as feature  # one-way graduation to a public branch when ready
```

Need-to-know sharing:

```bash
hv group create deployers && hv group add deployers alice deploybot
hv branch create staging
hv restrict staging --read deployers   # only deployers can read the branch or its secrets
hv secret add config/staging.env       # encrypted to alice + deploybot, nobody else
```

Sensitive work that must never sit in the clear, even on disk:

```bash
hv haven create --secret db-rewrite    # entire branch encrypted at rest — a stolen laptop yields nothing
```

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

Single static Go binary; SQLite-backed single-file repos (`.haven/haven.db`); SHA-256 objects; `age` (X25519) encryption and Ed25519 signatures.

---

## Documentation

- [`docs/design.md`](docs/design.md) — full design: the two axes, signed portable policy, secrets, decentralization model, architecture, protocol, milestones, and adversarial review.
