# Haven Threat Model

What Haven defends against, what it explicitly does not, and the trust
assumptions in between. This is the reference for [`../SECURITY.md`](../SECURITY.md).

## Assets

1. **Secret plaintext** — the contents of marked files (`.env`, `*.pem`, …) and
   whole secret refs. Must never be recoverable without an authorized private key.
2. **Access policy integrity** — the signed keyring and grants. Must be
   unforgeable and rewrite-evident.
3. **Repository/working-tree integrity** — objects and refs must not be silently
   corrupted or substituted.
4. **Private keys** — the user's age (X25519) and ed25519 keys in
   `~/.config/haven/identity`. Out of Haven's control to protect, but Haven must
   never copy them into the repo, a server, logs, or temp files.

## Trust boundaries

- **Trusted:** the user's machine, their private key, and the local `hv` binary.
- **Semi-trusted:** a Haven server. It is trusted for *availability* but **not**
  for confidentiality, integrity of access decisions, or honesty about object
  content. The design goal is "a server enforces policy but cannot forge it, and
  cannot read secrets."
- **Untrusted:** the network, other users on a shared host, anyone who obtains a
  copy of the repository database or a backup.

## What an attacker gets from a stolen repository

| In the clone | Form | They get |
|---|---|---|
| unmarked code | plaintext | the code (as with git) |
| policy (keyring, grants) | public keys + signatures | nothing to unlock with |
| secret files / refs | ciphertext (age) | garbage without an authorized key |

Copying the repo never unlocks secrets. Private keys are never in it.

## Defended failure modes

| Threat | Defense | Where |
|---|---|---|
| Read a secret from the DB/backup/wire | age X25519 encryption; only ciphertext at rest and on the wire | `internal/secret`, server stores ciphertext only |
| **Secret written world-readable on checkout** | secrets materialized 0600 (exec 0700), parent dirs 0700 | `workspace/checkout.go` |
| **Forged/substituted secret content** | decrypted plaintext is verified against the object's content hash before writing; mismatch refused | `workspace/checkout.go` |
| **Decryption error masked as data loss** | "not a recipient" is distinguished from corruption; corruption errors out instead of overwriting the file | `secret.Decrypt`, `workspace` |
| Server substitutes a non-secret object | client verifies downloaded content hashes to the requested id | `cli/transfer.go` |
| Forge a grant / forge the policy | ed25519-signed policy chain; each version's authority is checked against its **parent** | `internal/policy` |
| Rewrite policy history | parent-hash linkage; server requires an incoming policy to extend the current head | `policy.VerifyExtension` |
| **Hostile takeover of an open server** | `serve --policy-root <hex>` pins the root signing key a first policy must match | `protocol`, `cli/cmd_serve.go` |
| Request forgery / tampering | per-request ed25519 signature over method+path+time+sha256(body)+nonce | `internal/protocol/auth.go` |
| Replay (even within skew) | durable `seen_nonces` table keyed on **server** receive time | `protocol.acceptNonce` |
| Overwrite another reader's secret ciphertext | content-changing rewrites require write access to a ref reaching the secret | `protocol.putObject` |
| Object-enumeration leak | object fetch is reachability-gated by readable refs | `protocol.reachableSet` |
| Memory-exhaustion DoS | request bodies capped at `MaxRequestBytes` | `protocol.limitBody` |
| Stack-overflow DoS via deep tree | tree walks bounded at `maxTreeDepth` | `object/build.go` |
| Crash mid-write corrupts a file | atomic temp-file + fsync + rename on checkout; objects written before refs advance | `workspace`, `store` |
| Private key left group/world-readable | identity dir forced 0700; key file tightened to 0600 on load | `internal/identity` |

## Out of scope / known limitations

- **Compromise of the endpoint or the private key.** If the attacker has the
  user's machine or `~/.config/haven/identity`, they are that user.
- **Confidentiality over plaintext transport.** Signed requests prevent forgery,
  not eavesdropping. Run the server behind TLS for confidentiality.
- **Plaintext history before a file was marked.** Already public by then; a
  history rewrite is required to purge it.
- **Cross-server replay when keyrings are shared.** The canonical request binds
  method, path, time, body, and nonce — but **not** the server's host/origin. A
  captured signed request is therefore replayable against a *different* Haven
  server that shares the same keyring (until its nonce window passes). Not an
  issue for the common single-server-per-keyring deployment; if you share one
  keyring across servers, treat each server's nonce table as independent and
  prefer per-server keys. *(Tracked follow-up: bind origin into the signature.)*
- **Scale/availability**: no delta/packfile storage; objects load whole into
  memory. Unix only (flock). These are robustness/scope limits, not security
  guarantees.
- **No external audit.** The cryptographic *composition* (not the primitives)
  has not been independently reviewed. See `SECURITY.md`.

## Cryptographic primitives

- **Encryption:** `filippo.io/age` (X25519, ChaCha20-Poly1305).
- **Signatures:** `crypto/ed25519`.
- **Hashing / content addressing:** SHA-256.

Haven composes these; it does not implement primitives itself.
