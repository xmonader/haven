# Haven — Engineering Case Study

A portfolio narrative and interview-ready talking points for Haven: a from-scratch version control system in Go with structural privacy and built-in encrypted secrets. This document exists to bank the **career signal** from the project regardless of its business outcome — the engineering is the deliverable.

> Honest framing up front: as a *business*, a git-incompatible VCS is mispositioned (see [`../../gitsafe`](../../gitsafe) for the pivot). As an *engineering artifact*, it demonstrates senior/staff-level systems range — content-addressed storage, a Merkle DAG, three-way merge, applied cryptography, a signed authorization protocol, and a hardened network server. That range is what this document sells.

---

## The one-paragraph version (for a résumé / LinkedIn)

Built **Haven**, a complete version control system from scratch in Go (no git, no libraries for the core): a SHA-256 content-addressed object store over SQLite, a Merkle DAG of commits/trees/blobs, three-way merge with rename detection, and a full client/server sync protocol. Its differentiator is a **portable, ed25519-signed access-policy chain** that verifies offline and a secrets system where a file's **decryption recipients are derived from who can read its branch** (age/X25519). Hardened against real attack surface: signed-request auth with durable replay protection, atomic compare-and-swap ref updates, request-body DoS caps, schema-versioned storage with migrations, and at-rest compression. 16 test packages, green under `-race`.

## The problem it set out to solve

Mainstream VCS gets two things wrong: **privacy is procedural** (you *remember* not to push a branch) and **secrets are plaintext** (`.env` becomes a plaintext blob in every clone and backup, while access rules live in a hosting vendor's database and don't travel with the repo). Haven makes both structural — a private "haven" ref the push protocol physically refuses to send, and secrets encrypted to exactly the people authorized to read the branch, with the authorization itself signed into the repository.

## What makes it technically interesting (the parts worth talking about)

These are the pieces I'd bring up in a systems interview, roughly in order of how distinctive they are:

1. **Offline-verifiable signed access policy.** A parent-linked chain of ed25519-signed policy versions (keyring, grants with a verb hierarchy, groups, restricted refs). Anyone can verify offline that the policy was not forged or rewritten — the server *enforces* it but cannot *forge* it. This is the most novel idea in the project. Full write-up: [`deepdive-signed-policy.md`](deepdive-signed-policy.md).
2. **Recipients = branch readers.** A secret's age recipients are computed from the policy's reader set for that ref. Grant read access and decryption comes with it; revoke and `rotate` re-encrypts. One list, not two. Secrets are addressed by a hash of their *plaintext* (stable identity across re-encryption) while storing only ciphertext.
3. **Signed-request wire protocol.** Each request is signed over `method\npath\ntime\nsha256(body)\nnonce`. The body hash binds the signature to the payload (tamper-evident); a durable nonce table makes a captured request non-replayable even inside the clock-skew window. No long-lived secret on the wire, no server-issued challenge round-trip.
4. **Three-way merge with rename detection.** LCA-based merge base, content merge via a diff-chunk algorithm with git-style conflict markers, and exact-content rename carrying.
5. **Crash-consistency by ordering.** Every operation writes objects *before* advancing the ref (the single atomic commit point, via compare-and-swap), so an interruption leaves only garbage-collectable orphans — never a dangling ref. Same invariant git relies on.

## Decisions & trade-offs (shows judgment, not just coding)

- **SQLite via `modernc.org/sqlite` (pure Go, no cgo)** to keep the single-static-binary promise. Trade-off: no delta/packfile storage, objects loaded whole into memory — fine for source trees, wrong for monorepos. Documented as a known limitation rather than hidden.
- **Rejected ref-scoping secret identity.** Tempting for "correctness," but it would make a byte-identical `.env` on two branches conflict on every merge (the common case) to patch a rare, benign edge (identical plaintext = no incremental disclosure, self-heals via rotate). Chose to document the edge instead of paying a constant tax. *This is the decision I'm most proud of* — it's the one where the obvious "more secure" choice was the wrong engineering call.
- **Compression as a bounded win, not a delta engine.** When asked to "fix storage scale," I shipped at-rest zlib (real, testable) and was explicit that it does *not* solve memory-per-op or add delta encoding — refusing to overstate the fix.

## War stories (the "tell me about a hard bug" answers)

- **The rotation that silently did nothing.** `secret rotate` appeared to work and my first test passed — but the test only checked "still decrypts" + a count. Verifying against the actual binary showed the ciphertext bytes were *unchanged*: the store used `INSERT OR IGNORE`, so rewriting an existing hash was a no-op. Fixed with an explicit `ReplaceContent`/upsert and rewrote the test to prove the bytes change *and* a newly-added reader can decrypt. **Lesson: a test that can't fail is worse than no test — it manufactures false confidence.**
- **A TOCTOU in ref updates.** Resolve-compare-set across three statements raced under concurrent pushers. Replaced with a single-statement `CompareAndSwap`; a 16-goroutine racer test now proves exactly one winner.
- **An unbounded `io.ReadAll(r.Body)`** on the server — a trivial memory-exhaustion DoS. Capped with `http.MaxBytesReader`, made the limit injectable so the test doesn't allocate 256 MiB to prove the cap.
- **A self-inflicted availability hole I introduced while fixing another bug.** Making rotation propagate over push (an upsert on secret objects) meant *any* member could overwrite *any* secret's ciphertext and lock out other readers. Closed it by gating content-changing rewrites behind ref write access while keeping identical-byte re-uploads idempotent. **Lesson: a fix is a change, and a change has its own attack surface — re-review the fix adversarially.**

## Outcomes

- Complete local + networked VCS: 30+ subcommands, client/server sync, signed ACL enforcement, encrypted secrets, history porcelain (rebase/cherry-pick/stash/bisect).
- 16 test packages including fuzz tests on the parsers and concurrency tests on the ref CAS; green under `-race`, vet clean.
- Honest production assessment documented: solid for solo/small-trusted-team over TLS; not "production-grade audited" without an external security review (which, for a tool whose whole pitch is secrets + access control, I argued is mandatory before that claim).

## What I'd do differently

- **Validate positioning before building the engine.** The hardest, best work (the VCS plumbing) has the least market value; the cheap-to-extract idea (signed portable ACL) has the most. I'd have prototyped the idea as a git overlay first. This realization *is* the gitsafe pivot.
- **Write the falsifiable test first.** The rotation bug would have been caught immediately by a test that asserted on the ciphertext bytes rather than on "does it still work."

## Deep-dive write-ups (blog/talk material)

- ✅ [`deepdive-signed-policy.md`](deepdive-signed-policy.md) — Offline-verifiable authorization: a signed policy chain a server enforces but can't forge. **(flagship — fully drafted)**
- ☐ Recipients = branch readers: deriving decryption from access control so you never manage two lists.
- ☐ A signed-request protocol with replay defense in ~100 lines (and why not a server-issued challenge).
- ☐ Crash-consistency for free: why "objects before refs" is the whole trick.
- ☐ Three bugs my passing tests didn't catch, and what each taught me about testing.

Each outline maps to a real, demonstrable part of the codebase — they're talks waiting to be written, not hypotheticals.
