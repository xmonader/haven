# Security Policy

Haven's entire purpose is to keep secrets confidential and to enforce access
control. A vulnerability here can expose secrets or grant unauthorized access,
so security reports are taken seriously and prioritized.

## Reporting a vulnerability

**Do not open a public issue for a security vulnerability.**

Report privately through one of:

- **GitHub Security Advisories** — the "Report a vulnerability" button under the
  repository's *Security* tab (preferred; keeps the report private and tracked).
- **Email** — xmonader@gmail.com, with `[haven-security]` in the subject. PGP
  encryption available on request.

Please include: affected version (`hv version`), a description of the issue, and
the smallest steps or proof-of-concept that reproduces it.

### What to expect

- **Acknowledgement** within 72 hours.
- An initial assessment (severity, affected versions) within 7 days.
- Coordinated disclosure: we agree on a timeline, fix in a private branch, and
  publish a fixed release plus an advisory crediting you (unless you prefer to
  remain anonymous). Default embargo is 90 days or until a fix ships, whichever
  is sooner.

## Supported versions

Haven is pre-1.0. Until a 1.0 release, only the latest tagged release and the
`main` branch receive security fixes.

## Scope

In scope — anything that breaks Haven's stated guarantees:

- Recovering secret **plaintext** without an authorized private key (from a
  repository, a server, a backup, logs, or temp files).
- **Bypassing access control**: reading a restricted ref or an object reachable
  only from it without a grant; forging, replaying, or tampering with a signed
  request; forging or rewriting the signed policy chain.
- **Writing where you shouldn't**: overwriting another reader's secret
  ciphertext, or advancing a ref without the required grant.
- **Data loss / corruption** from non-atomic operations or interruption.
- **Denial of service** reachable by an unauthenticated or low-privilege client
  (e.g. resource exhaustion).

Out of scope:

- Compromise of a user's machine or their private key in
  `~/.config/haven/identity` (Haven assumes the endpoint and key are trusted).
- Confidentiality of data sent over a plaintext (non-TLS) transport — signed
  requests prevent forgery, not eavesdropping. Run the server behind TLS.
- History that was committed as plaintext *before* a file was marked secret
  (already public by then; purging requires a history rewrite).

## Important: this software has not had an external security audit

Haven implements its own cryptographic protocols (a signed policy chain, a
signed-request wire protocol) on top of vetted primitives (`filippo.io/age`
X25519, `crypto/ed25519`, SHA-256). The composition has **not** been
independently audited. Treat it accordingly: suitable for solo and
small-trusted-team use over TLS; do not rely on it as the sole control for
high-value secrets in a hostile environment until an audit has been performed.
See [`docs/threat-model.md`](docs/threat-model.md) for the full model and its
boundaries.
