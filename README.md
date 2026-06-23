# Haven (`hv`)

**A version control system where privacy and secrets are built in — not bolted on.**

Haven is a from-scratch VCS for code that isn't ready for the public world yet. Private branches, need-to-know sharing, and encrypted credentials are first-class concepts — enforced by storage and cryptography, not by discipline.

🌐 **[Website](https://xmonader.github.io/haven/)** · 📖 **[User guide](docs/userguide.md)** · 🏗️ **[Design](docs/design.md)** · 🛡️ **[Threat model](docs/threat-model.md)**

---

Mainstream VCS makes you *remember* not to push the private branch, ships your `.env` as plaintext in every clone, and keeps access rules in a vendor's database. Haven makes all three structural:

| | Mainstream VCS | Haven |
|---|---|---|
| **Privacy** | procedural ("don't push it") | a **haven** ref `hv push` physically refuses to send |
| **Access** | in a vendor's database, doesn't travel | **signed policy in the repo**, verifiable offline, server-enforced |
| **Secrets** | plaintext `.env` in every clone | **encrypted to whoever can read the branch** |

The rule that ties access and secrets together: **a secret's decryption recipients are exactly the people who can read its branch.** Grant read access and decryption comes with it — you never manage two lists. It stays decentralized like git: every clone is whole, everything works offline.

## Quick start

```bash
hv init                          # new repo (seeds default secret marks: .env, *.pem, …)
hv key gen                       # your encryption identity; you join the repo's members
echo "DATABASE_URL=…" > .env
hv add . && hv commit -m "app"   # .env is encrypted automatically
hv haven create scratch          # private branch; hv push refuses to leak it
hv remote add origin <url> --kind team
hv push origin main              # the secret travels as ciphertext — safe on a public remote
```

## What works

A complete local + networked VCS: `init` `add` `commit` `status` `log` `diff` `reset` `restore` `tag` · `cherry-pick` `revert` `rebase` `stash` `bisect` · `branch` `haven` `publish` `merge` (three-way, rename-aware) · `serve` `remote` `push` `fetch` `pull` `clone` `sync` · `key` `member` `group` `grant`/`revoke` `restrict` `policy` · `secret add`/`ref`/`rotate`/`status` · `fsck` `gc` `repack`.

Single static Go binary; SQLite single-file repos (schema-versioned, zlib + delta-compressed at rest); SHA-256 objects; `age` (X25519) encryption and Ed25519 signatures. CI runs build/vet/gofmt/`-race`/coverage/fuzz on Linux and macOS plus a Windows cross-build.

**Copying the repo never unlocks secrets** — they're ciphertext, and private keys live in `~/.config/haven/identity`, never in the repo or on a server.

## Build

```bash
make build        # build ./hv (version injected from `git describe`)
make test         # run tests
make race         # race detector, uncached — the pre-release gate
make dist         # cross-compiled static release binaries
```

## Security & status

Haven enforces confidentiality with cryptography and access with a signed policy the server can enforce but not forge. The crypto/data path is hardened (private secret-file permissions, decrypted-content integrity checks, atomic writes, request signing with replay protection, bounded resource use) and tested under the race detector. **It has not had an external security audit** — suitable for solo and small-trusted-team use over TLS; get an audit before relying on it as the sole control for high-value secrets in a hostile environment. See [`docs/threat-model.md`](docs/threat-model.md) and [`SECURITY.md`](SECURITY.md).

> **Not yet:** interactive rebase, octopus (>2-parent) merge, and streaming reads for very large single blobs.

## License

[MIT](LICENSE) © 2026 xmonader.
