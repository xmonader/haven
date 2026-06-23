# Haven User Guide

A practical tour of `hv`. For the design rationale, see [`design.md`](design.md).

## Install

```sh
make build      # produces ./hv (single static binary, no cgo)
make install    # or copy ./hv onto your PATH
```

## The mental model: two axes

Everything in Haven sits on two independent axes:

- **Access** — *who can fetch a ref from a server.* Branches are **public**, havens are **private** (never pushed to a team server), and you can mark refs **restricted** (need-to-know). Enforced by the server.
- **Secret** — *whether a file's bytes are encrypted at rest.* Marked files (`.env`, `*.pem`, …) are encrypted to a ref's readers; everything else is plaintext. Enforced by cryptography — the server only ever sees ciphertext.

A file can be any combination: a public branch can hold encrypted secrets; a private haven can hold plaintext drafts.

---

## 1. Your first repository

```sh
hv init
hv config user.name "Ada Lovelace"
hv config user.email ada@example.com

echo "hello" > README.md
hv add .
hv commit -m "first commit"

hv status        # what's staged / modified / untracked
hv log           # history
hv diff          # working-tree changes
```

## 2. Branches and private havens

```sh
hv branch create feature
hv branch switch feature
# …work, commit…
hv branch switch main
hv merge feature          # three-way, rename-aware; conflicts land in the tree

# A haven is a PRIVATE branch — it never leaves your machine on a team push.
hv haven create wip
hv haven switch wip
# When it's ready for the world:
hv publish wip            # promotes a haven to a public branch (fast-forward only)
```

`hv branch list` shows branches; `hv haven list` shows havens. They're separate namespaces.

## 3. Identity and secrets

Generate your keypair once (an age key for encryption + an ed25519 key for signing). The private key lives in `~/.config/haven/identity`, **never** in the repo.

```sh
hv key gen        # also bootstraps the signed policy; you become the founding admin
hv key show       # prints your two PUBLIC keys, to share with an admin
```

Files matching a **secret mark** are encrypted automatically on `add`:

```sh
hv secret list                 # default marks: .env, .env.*, *.pem, *.key, secrets/**, …
hv secret add "config/*.token" # add your own glob

echo "TOKEN=hunter2" > .env
hv add .env                    # stored as ciphertext, addressed by a stable plaintext hash
hv commit -m "add secret"
```

On checkout, recipients see the plaintext; everyone else sees a locked-notice placeholder. The repository database and any server hold only ciphertext.

### Secret branches (whole-tree encryption)

```sh
hv secret ref staging          # every file on 'staging' is encrypted at rest
hv haven create vault --secret # a private haven whose whole tree is encrypted
```

### Rotating after a membership change

```sh
hv secret rotate               # re-encrypt secrets to the CURRENT readers (no new commit)
hv secret status               # warns when readers drifted from the last rotation
```

## 4. Access control (signed policy)

Authorization is a **portable, ed25519-signed policy chain** stored in the repo. It verifies offline and the server can enforce it but cannot forge it.

```sh
# Add a teammate: ask them for `hv key show`, then:
hv member add bob <bob-sign-key> <bob-age-recipient>

# Capabilities: read | write | force | grant | admin (higher implies lower).
hv grant bob write "refs/branches/**"

# Groups and need-to-know refs:
hv group create deployers
hv group add deployers bob
hv restrict staging --read deployers    # removes PUBLIC access to 'staging'

# Inspect:
hv policy show          # current grants/keyring
hv policy access staging# who can read a ref
hv policy log           # version chain
hv policy verify        # verify every signature back to the root
```

Revoking: `hv member revoke bob` (then `hv secret rotate` so secrets bob could read are re-encrypted without him).

## 5. Remotes and collaboration

Two server kinds: a **team** server refuses private refs (havens); a **personal** server accepts them (for syncing between your own machines).

```sh
# Serve a repo (HTTP, or HTTPS with a cert):
hv serve --kind team --addr :8473
hv serve --kind personal --tls-cert cert.pem --tls-key key.pem

# On a client:
hv remote add origin http://host:8473 team
hv push origin            # pushes branches (refuses havens), carries the signed policy
hv fetch origin
hv pull origin            # fetch + merge
hv clone http://host:8473 myrepo

# Between YOUR machines (carries havens too):
hv remote add laptop http://laptop:8473 personal
hv sync laptop
```

Every request to a server is signed over method + path + time + body + a one-time nonce, so requests can't be forged, tampered, or replayed. Anonymous clients get only public refs.

## 6. Everyday history tools

```sh
hv reset --hard <rev>           # move the branch (and working tree) to a revision
hv restore --source <rev> path  # restore a file from a revision
hv tag v1.0                     # lightweight tag; 'hv tag list' / 'hv tag delete v1.0'

hv cherry-pick <rev>            # replay one commit onto HEAD
hv revert <rev>                 # apply the inverse of a commit
hv rebase main                  # replay current branch onto main (aborts cleanly on conflict)

hv stash                        # shelve working changes; 'hv stash list' / 'hv stash pop'

hv bisect start                 # find the commit that introduced a bug
hv bisect bad ; hv bisect good <old>
# …test the checked-out commit, mark good/bad until found…
hv bisect reset
```

## 7. Maintenance

```sh
hv fsck    # verify object integrity and the policy chain
hv gc      # drop unreachable objects (keeps everything reachable from refs/tags/stash/policy)
```

---

## Conflict handling

`merge`, `cherry-pick`, `revert`, and `stash pop` write standard conflict markers into the working tree:

```
<<<<<<< ours
your version
=======
their version
>>>>>>> theirs
```

Edit, `hv add` the resolved files, then `hv commit`. `rebase` is all-or-nothing: a conflict aborts the whole rebase and restores your branch untouched.

## Quick reference

| Area | Commands |
|------|----------|
| Local | `init` `config` `add` `commit` `status` `log` `diff` |
| Branches | `branch` `haven` `publish` `merge` |
| History | `reset` `restore` `tag` `cherry-pick` `revert` `rebase` `stash` `bisect` |
| Identity/secrets | `key` `secret` |
| Access | `member` `group` `grant` `revoke` `restrict` `policy` |
| Remotes | `serve` `remote` `push` `fetch` `pull` `clone` `sync` |
| Maintenance | `fsck` `gc` |

Run `hv help` for the full list.

---

## Known limitations

Haven is solid for solo and small-trusted-team use. Be aware of these by-design or not-yet boundaries:

- **Storage compresses but doesn't delta/pack.** Each object is zlib-compressed at rest (incompressible content is stored raw, never enlarged), which cuts on-disk size for source trees. But there is no delta encoding or packfile, and each object is still loaded whole into memory per operation. Great for source trees; not suited to very large repos or large binary files.
- **Unix only.** The working-copy lock uses `flock(2)`; Windows is unsupported.
- **Shared-plaintext secrets across refs.** A secret's identity is its plaintext hash, so the *same* secret bytes on two refs share one ciphertext. This keeps merges of a shared `.env` clean, but if two refs need that identical secret encrypted to *different* readers, only one recipient set is stored. It is not a disclosure risk (identical plaintext means identical knowledge); at worst a reader is temporarily locked out until `hv secret rotate`. Use distinct secret values per trust boundary.
- **Replay nonces are evicted after 5 minutes.** Within that window replay is blocked by a durable nonce table; the window itself is the clock-skew bound.
- **Single-process server assumptions.** The reachability cache is per-process; the nonce table is shared, but run one `hv serve` per repo.

For multi-writer teams, concurrent ref updates are safe (atomic compare-and-swap) and rotated secrets propagate on push. Run the server behind TLS (or a tunnel) for confidentiality on untrusted networks.
