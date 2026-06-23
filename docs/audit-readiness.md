# Haven — Security Audit Readiness

This document orients an external reviewer. Haven's guarantees are
confidentiality of secrets and correct enforcement of access; the review should
try to break those. It complements [`threat-model.md`](threat-model.md) (the
model) by pointing at the code and tests that implement each control.

## Scope to review (highest value first)

1. **Secret data path** — `internal/secret/`, `internal/workspace/checkout.go`.
   Encryption, decryption, the plaintext-hash integrity check, file permissions
   (0600), atomic writes. Tests: `internal/workspace/checkout_test.go`,
   `internal/secret/*_test.go`.
2. **Access policy** — `internal/policy/`. The signed chain, `Verify` (authority
   evaluated against the *parent*), `Eval`, `VerifyExtension`, `RootSignKey`.
   Tests: `internal/policy/*_test.go`, `enforce_test.go`.
3. **Wire protocol** — `internal/protocol/`. Request signing
   (`canonicalRequest`: method+path+time+bodyhash+nonce+host), nonce replay
   table, reachability gating, body cap, secret-rewrite gating, origin binding,
   policy-root pinning. Tests: `internal/protocol/enforce_test.go`.
4. **Storage integrity** — `internal/store/`, `internal/object/`, `internal/ref/`.
   Content addressing, compression codec + migration, delta codec
   (`MakeDelta`/`ApplyDelta`, `EncodeDelta`/`DecodeDelta`) and delta-aware
   `Get`/`Each`/`gc`/`repack`, atomic ref CAS, tree-depth bound.

## Controls → code map

| Guarantee | Mechanism | Code |
|---|---|---|
| Secrets unreadable at rest/wire | age X25519; ciphertext only stored/sent | `secret.Encrypt/Decrypt`, server stores ciphertext |
| Decrypted secret not world-readable | 0600/0700 file + 0700 dir | `workspace.fileMode/dirMode` |
| No forged-secret substitution | plaintext re-hashed to object id before write | `workspace.secretContent` |
| Decrypt errors not silent data loss | `ErrNotRecipient` vs other errors | `secret.Decrypt`, `workspace.secretContent` |
| Policy unforgeable / rewrite-evident | ed25519 chain, parent-evaluated authority, parent-hash linkage | `policy.Verify`, `policy.VerifyExtension` |
| Server enforces, can't forge | runs `Eval`, cannot sign a chain | `protocol` + `policy` |
| Request forgery/tamper | per-request ed25519 signature over a canonical string | `protocol.authActor`, `auth.go` |
| Replay (in-window / cross-server) | durable nonce table (server time) + host binding + `--origin` | `protocol.acceptNonce`, `authActor` |
| Open-server takeover | `--policy-root` pins bootstrap root | `protocol.verifyIncomingPolicy` |
| Object fetch leakage | reachability-gated by readable refs | `protocol.reachableSet` |
| Memory DoS | `MaxRequestBytes` body cap | `protocol.limitBody` |
| Stack-overflow DoS | `maxTreeDepth` bound | `object` tree walks |
| Crash mid-write | atomic temp+fsync+rename; objects-before-refs | `workspace`, `store` |
| Private key exposure | 0600 file / 0700 dir, tightened on load | `identity` |

## Suggested attacks to attempt

- Serve a secret object whose ciphertext decrypts (to a valid recipient) but to
  plaintext that doesn't match the object id — confirm it is refused.
- Push a policy chain that grants the pusher admin without parent authority.
- Replay a captured signed request (same server; different server with shared
  keyring and `--origin` set).
- Fetch an object reachable only from a ref you lack read on, by hash.
- Overwrite another reader's secret ciphertext via `PUT /objects`.
- Feed a deeply nested or self-referential tree and a giant request body.
- Interrupt a checkout mid-write; inspect for truncated/partial secret files.
- Inspect the process for plaintext leaks: temp files, logs, error messages.

## Known residual risk (not yet mitigated)

- **No prior external audit** — the cryptographic *composition* (not the
  primitives: age, ed25519, SHA-256) is unreviewed. This document exists to
  enable that review.
- **Scale**: objects are zlib-compressed and `hv repack` delta-compresses
  similar objects against a base (reconstruction self-verified before any
  rewrite; reads bounded to depth-1 chains; gc retains delta bases). There is
  still no streaming for very large single blobs. Not a security control, but a
  reviewer should confirm `ApplyDelta` rejects malformed/circular deltas and
  that delta storage never changes an object's identity hash.

## Build & verify

```sh
make race     # all tests, race detector, uncached
make cover    # coverage
make vet
go test ./... -run '^$' -fuzz FuzzParseCommit -fuzztime 60s ./internal/object
```

CI runs build/vet/gofmt/`-race`/coverage/fuzz on Linux and macOS plus a Windows
cross-build on every push.
