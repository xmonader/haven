# Contributing to Haven

Thanks for your interest. Haven is a security-sensitive tool (it handles secrets
and access control), so the bar for changes on the crypto/data path is high.

## Before you start

- **Security issues:** do **not** open a public issue or PR. Follow
  [`SECURITY.md`](SECURITY.md) for private disclosure.
- For features or larger changes, open an issue first to discuss scope.

## Development

```sh
make build    # build ./hv (version is injected from git describe)
make test     # all tests
make race     # race detector, uncached — the gate before merging
make cover    # coverage summary
make fmt      # gofmt -w
make vet      # go vet
```

Requires Go 1.25+ and `git` on PATH.

## Expectations for a PR

1. **It builds and `make race` is green.** CI runs build, `go vet`, a gofmt
   check, `-race` tests on Linux and macOS, coverage, and a short parser fuzz.
2. **gofmt-clean.** Run `make fmt`. CI fails on unformatted code.
3. **Tests for behavior changes.** Anything touching the secret, crypto, policy,
   or write path needs a test that would fail without the change. A test that
   can't fail is worse than none.
4. **Small, focused commits** in `type(scope): subject` form, each one building
   and passing on its own.
5. **No new dependencies** without discussion — Haven is deliberately a single
   static binary (pure-Go SQLite, age, stdlib crypto).

## Especially careful areas

- `internal/secret`, `internal/workspace` — decryption and writing plaintext to
  disk. Get permissions, integrity checks, and atomicity right.
- `internal/policy`, `internal/protocol` — access decisions, signatures, replay.
- Anything that walks untrusted input (objects from a remote): bound it.

When in doubt, write the adversarial test first.
