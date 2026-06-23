# Offline-Verifiable Authorization: a Policy a Server Enforces but Can't Forge

*A deep dive into Haven's signed access-policy chain — the most distinctive piece of the project, and a pattern that generalizes well beyond a VCS.*

## The problem with where access rules live

In a normal git workflow, "who can read `staging`?" is answered by a row in GitHub's (or GitLab's, or Gitea's) database. That has three consequences people rarely state out loud:

1. **The policy doesn't travel with the repo.** Clone it elsewhere and the access rules don't come along — they were never *in* the repository.
2. **You must trust the server's word.** When the server says "you're allowed," there's nothing to check it against. A compromised or malicious host can claim anything.
3. **There's no portable, verifiable history** of who granted whom what. The audit log is, again, a vendor table.

Haven's goal was to make authorization a **property of the repository** — something that travels with every clone, verifies with nothing but a public key, and that a server can *enforce* but cannot *forge or rewrite*. That last clause is the whole design challenge: how do you let an untrusted server gate access without trusting it to define access?

## The shape of the answer

A **policy** is one signed version: a keyring (actor → public keys), a set of capability **grants** (subject, verb, resource-glob), groups, and a list of restricted refs. Versions are **parent-linked into a chain** — each version stores the hash of the previous policy object — and each is **ed25519-signed** by the actor who created it.

```
v0 (root, self-signed) ── v1 (signed by an admin) ── v2 (signed by an admin) ── HEAD
        keyring                  +grant bob read           revoke bob
        +root admin              staging
```

Verification walks the chain and answers one question at each link: *was this change authorized by the policy that preceded it?* If every link checks out back to a root the verifier trusts, the whole policy is sound — no server consulted.

## The one idea that makes it secure: evaluate authority against the *parent*

Here's the subtle part, and the part I'd put on a whiteboard. When verifying version *N*, you do **not** ask "does version *N* say its signer is an admin?" — that's circular. An attacker would simply write a version that grants *themselves* admin and signs it.

Instead you ask: **"did the signer of version *N* hold admin in version *N−1*?"** Authority to change the policy is always evaluated against the policy *as it stood before the change*. From the actual `Verify`:

```go
// Later version: signer must have held admin in the parent.
m, ok := parent.Keyring[p.Signer]
if !ok {
    return fmt.Errorf("policy v%d signer %q not in parent keyring", p.Version, p.Signer)
}
if !p.verifySig(m.Sign) {
    return fmt.Errorf("policy v%d signature invalid", p.Version)
}
if !parent.Eval(p.Signer, Admin, "refs/policy") {
    return fmt.Errorf("policy v%d signer %q lacked admin", p.Version, p.Signer)
}
```

The root version (`parent == nil`) is the only one that's self-signed — by a member in its *own* keyring — which is the trust anchor a clone pins. Every subsequent version derives its legitimacy from the one before it. You can't bootstrap authority you weren't already given.

## Why the signature covers canonical JSON minus itself

A version signs over its own bytes with the signature field blanked:

```go
func (p *Policy) signingBytes() []byte {
    clone := *p
    clone.Sig = ""        // sign everything *except* the signature
    b, _ := json.Marshal(clone)
    return b
}
```

Two things matter here. First, you obviously can't sign the signature, so it's zeroed before signing and verifying — both sides compute identical bytes. Second, the signed bytes include the `Parent` hash, so the signature transitively commits to the entire history behind it. Change any ancestor and every descendant's parent-hash linkage breaks; the chain stops verifying. That's what makes history **rewrite-evident** without a server: the hashes are load-bearing, not decorative.

## Verification walks root-upward

Verifying the chain collects it from HEAD back to root, then verifies **forward** so each version has its already-trusted parent available:

```go
// Verify from root upward so each has its parent available.
for i := len(chain) - 1; i >= 0; i-- {
    var parent *Policy
    if i+1 < len(chain) {
        parent = chain[i+1]
    }
    if err := chain[i].Verify(parent); err != nil {
        return 0, err
    }
}
```

The result: a clone fetched from an **untrusted mirror** can prove, offline, that the policy it received is a legitimate descendant of the root it trusts — that no grant was forged and no version was spliced out or reordered. The mirror could refuse to serve data, but it cannot lie about *who is allowed*.

## The access decision: a verb hierarchy with a need-to-know escape hatch

Enforcement is one function, `Eval(actor, verb, resource)`, that scans grants for a match. Two design choices are worth calling out:

- **Verbs form a hierarchy** (`read < write < force < admin`), so a single admin grant implies the lower capabilities — you don't enumerate every verb per actor.
- **Restricted refs suppress the wildcard.** A `*` (everyone) grant is the common "public read" case, but on a ref marked *restricted* (need-to-know), wildcard grants are skipped — only a concrete actor or group can reach it:

```go
// On a restricted ref, wildcard ("*") grants do not apply —
// only concrete actors/groups can reach it.
if g.Subject == "*" && restricted {
    continue
}
```

This lets "public by default, locked down by exception" coexist in one grant list without contradiction.

## How the server fits — enforce, never forge

The server holds the same policy (it travels in the repo) and runs the same `Eval` to gate ref listing, object fetch, and writes. But because it only ever *evaluates* a chain it cannot produce a valid signature for, the worst a malicious server can do is **deny** — it can't *grant*. When a client pushes a new policy version, the server doesn't take its word either; it verifies the incoming chain extends the current one (no history rewrite) before accepting it. Authority and enforcement are deliberately separated: the cryptography is the source of truth, the server is just a gate.

## Why this generalizes

Nothing here is VCS-specific. "A portable, signed, offline-verifiable capability chain whose authority to change is always evaluated against its own prior state" is a pattern for **any** system that wants access rules to travel with the data and survive an untrusted intermediary — config repos, document stores, package registries, signed software supply chains. That portability is precisely why the [gitsafe pivot](../../gitsafe) keeps this engine and discards the VCS around it: the idea is the asset.

## What I'd flag in review

- **Root trust still has to be pinned out-of-band.** Offline verification proves "descends from root X"; you must obtain root X's fingerprint through a trusted channel, exactly as with TLS roots or a git tag signing key. The design reduces trust to one pinned key — it doesn't eliminate it.
- **JSON canonicalization is a sharp edge.** Signing `json.Marshal` output assumes deterministic field ordering (Go's `encoding/json` orders struct fields by declaration, so this holds here) — but a reimplementation in another language with different ordering would fail to verify. A production version should sign a canonical encoding (sorted keys / a defined canonical form), not "whatever the language's marshaller emits."
- **No revocation of a signing key mid-chain beyond the keyring `status` field.** Compromise of an admin key requires rolling forward a new version; there's no CRL-style fast revocation. Documented as a known boundary.
