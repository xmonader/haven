# Chapter 4 — Havens and Secrets

This is the chapter where Haven stops looking like a smaller version control system and starts doing the two things no mainstream tool does for you. In Chapter 1 we drew the two axes in the abstract; here we operate them. The **access** axis gives us *havens* — private branches that the push command physically refuses to leak. The **secrecy** axis gives us encrypted *secrets* — files whose bytes are ciphertext at rest, readable only by the people who can read their branch. These are Haven's reason to exist, and by the end of this chapter you will create both, prove their guarantees with independent tools, and manage the lifecycle of a secret as people join and leave.

We will take the two pillars in turn — havens first, then secrets — and then show how they combine into the most secure cell of Chapter 1's grid: a private branch whose entire tree is also encrypted. Throughout, keep the binding rule from Chapter 1 in the front of your mind, because it is the thread that stitches access and secrecy together: *a secret's decryption recipients are exactly the people who can read its branch.* Everything about how secrets behave follows from that one sentence.

## Part 1 — Havens: branches that refuse to leak

Recall the bottom-left cell of the grid: a private branch holding plaintext. You have an experiment, a half-finished spike, a security fix you are not ready to disclose. It has no secrets in it — it is just *not for sharing yet*. In a mainstream tool the only thing keeping it off the server is your memory. In Haven, you put it in a **haven**, and the tool makes leaking it a thing that *cannot happen by accident*, no matter how you invoke `push`.

A haven is a branch on the private end of the access axis. It lives in its own namespace, separate from public branches — which is why there is a dedicated `haven` command paralleling the `branch` command you already know. Creating one looks familiar:

```sh
hv haven create wip
  created private wip
hv haven list
    wip
```

Read the creation message: `created private wip`. The word *private* is not decoration — it is a property stored with the ref that the rest of the system honors. `hv haven list` shows your havens, a list deliberately separate from `hv branch list` (which shows public branches). The separation matters: a haven named `wip` and a public branch named `wip` would be two distinct refs, because they live in different namespaces. This keeps the mental model clean — public things are in one place, private things in another — and prevents the confusion of "is this branch the private one or the public one?"

You switch onto a haven and work on it exactly as you would a branch — the daily loop from Chapter 3 is unchanged:

```sh
hv haven switch wip
echo "rough draft" > draft.txt
hv add draft.txt
hv commit -m "wip"
```

Nothing about committing changes because the privacy of a haven is about *movement*, not about how it is stored locally. On your own disk, a (non-secret) haven is plaintext, just like any branch — you can read it, diff it, merge into it. The guarantee kicks in the moment you try to send it to a shared server.

### The structural refusal

Here is the payoff that makes a haven different from "a branch I promise not to push." When you ask `push` to send a haven to a team server, it *refuses* — and it refuses by design, not by configuration:

```sh
hv push origin wip
  hv push: refusing to push private haven "wip"; use 'hv sync' to a personal
  remote, or --i-know to override
```

The command did not push, and — crucially — it exited with a *non-zero status*, so a script or CI pipeline that ran `hv push` would see a failure rather than sailing past a silent skip. This is the structural-versus-procedural distinction from Chapter 1 made concrete: the protection is the tool's behavior, not your vigilance. You could run this in a loop, wire it into a careless automation, hand it to a brand-new teammate who does not know `wip` is private — and the haven still does not leak, because `push` is built to refuse private refs to a team remote.

Notice the message offers two escape hatches, and it is worth understanding both. `hv sync` to a *personal* remote is the legitimate way to move a haven between *your own* machines — we cover it shortly. And `--i-know` is a deliberate, explicit override for the rare case where you truly mean to push a private ref and accept the consequences. The override exists because Haven respects that you are an adult who occasionally has unusual needs; but it requires you to *say so loudly*, which means it can never happen by the kind of absent-minded reflex that leaks branches in other tools. A safety feature you can override on purpose but not trip over by accident is exactly the right design.

### Graduating a haven with `publish`

A haven is for work that is not ready. When it *becomes* ready, you do not want to manually recreate it as a public branch — you want to graduate it cleanly. That is `publish`:

```sh
hv publish wip --as feature
  published wip -> branch feature at 875118cc0b
```

This promotes the private haven `wip` into a new *public* branch named `feature`, pointing at the same commit (`875118cc0b`). The `--as` flag names the resulting public branch. After publishing, `feature` is an ordinary public branch — it can be pushed, fetched, and merged like any other — while the haven has served its purpose as the private incubator. The promotion is one-directional and deliberate: you are making a conscious decision that this work is now fit for the world, and `publish` records that transition rather than leaving you to reconstruct it by hand. Think of it as the moment a draft becomes a release candidate.

### Moving havens between your own machines with `sync`

There is a real tension in the haven design: a haven must never reach a *team* server, but you legitimately work across more than one machine and want your private work-in-progress to follow you. Haven resolves this with two *kinds* of remote and a separate verb. A **team** remote refuses havens (the refusal you just saw). A **personal** remote — a server you run for yourself, syncing your own laptop and desktop — *accepts* them, and you move havens to it with `sync` rather than `push`:

```sh
# (full remote setup is Chapter 5; the shape of the idea:)
hv remote add laptop http://laptop.local:8473 --kind personal
hv sync laptop          # carries branches AND havens between your machines
```

The distinction is enforced on both ends. `push` refuses havens regardless of remote kind, so you cannot accidentally `push` a private branch even to your personal server; `sync` is the verb that intentionally carries privates, and it is meant for personal remotes. A correctly configured *team* server, for its part, refuses to accept a private ref even if one is somehow offered. The result is that "private work follows me across my machines" and "private work never reaches the team" are *both* true, with no contradiction — the access axis simply has a personal lane and a team lane, and havens are allowed in the first but not the second. We wire up real remotes and servers in Chapter 5; for now, hold the concept: `push` for sharing, `sync` for your own machines.

## Part 2 — Secrets: encrypted to whoever can read the branch

Now the secrecy axis. A secret in Haven is a file whose bytes are stored as ciphertext, encrypted so that only authorized recipients can decrypt them. You met this in Chapter 2 when `.env` was encrypted automatically; here we learn how Haven decides what to encrypt, how the recipient set is determined, and how to manage secrets over time.

### Marks: how Haven knows what to encrypt

Haven does not ask you, on every commit, "is this file a secret?" — that would be procedural, and you would forget. Instead it matches filenames against a set of **secret marks**: glob patterns that designate which files are secrets. Any file matching a mark is encrypted automatically as it is staged. A fresh repository ships with sensible defaults:

```sh
hv secret list
  **/credentials.json
  *.key
  *.pem
  .env
  .env.*
  id_ed25519
  id_rsa
  secrets/**
```

Look at what those defaults capture: environment files (`.env`, `.env.*`), private keys (`*.key`, `*.pem`, `id_rsa`, `id_ed25519`), credential blobs (`**/credentials.json`), and anything under a `secrets/` directory (`secrets/**`). These are the files that most commonly leak in real-world incidents, pre-marked so that the *default* behavior is safe. The `**` in some patterns means "at any depth," so `**/credentials.json` matches that filename anywhere in the tree, while `secrets/**` matches everything under a `secrets/` directory. This is the secrecy axis defaulting to protection: out of the box, the usual suspects are encrypted without you doing anything.

When your project has secrets the defaults do not anticipate — a custom token file, say — you add your own mark:

```sh
hv secret add "config/*.token"
  marked "config/*.token" as secret (matching files will be encrypted on add)
```

From now on, any file matching `config/*.token` is encrypted on `add`, exactly like the built-in marks. The message states the contract plainly: *matching files will be encrypted on add*. Marks are stored in the repository, so they travel with it — a teammate who clones gets the same marks and the same automatic protection, which means the safety is a property of the project, not of each person's discipline.

### The binding rule in action

Here is where the two axes meet. When Haven encrypts a secret, *to whom* does it encrypt it? The binding rule answers: to exactly the people who can read the secret's branch. You never hand Haven a recipient list; it derives one from the access policy. On a solo repo, that recipient is just you, which you can see when you rotate (more on rotation below):

```sh
hv secret rotate
  rotated 1 secret object(s) on main to 1 recipient(s)
```

The phrase `to 1 recipient(s)` is the rule made visible: there is one person who can read `main` (you), so the secret is encrypted to one recipient (you). When a teammate is granted read access to `main` in Chapter 5, that number becomes two, automatically, because the recipient set is *computed from access*, not maintained by hand. This is the entire reason Haven is pleasant to operate at scale: you think about *who can read the branch*, and encryption recipients follow mechanically. The two-list synchronization nightmare from Chapter 1 simply does not exist.

### What a non-recipient sees

The flip side of "recipients see plaintext" is "non-recipients see nothing useful." When someone without an authorized key checks out a secret, Haven does not hand them garbage bytes that might be mistaken for corruption, nor does it error out — it materializes a clear placeholder so the situation is unmistakable:

```
<haven: encrypted secret; you are not a recipient>
```

You saw this exact line in Chapter 2 when a wrong identity tried to read a secret. It is a deliberate, honest signal: the file *exists* and is *encrypted*, and *you* are not among those it was encrypted for. This is far better than either alternative — silent garbage would look like a bug, and a hard error would break tools that just want to walk the tree. The placeholder lets a non-recipient clone, build, and work with the non-secret parts of a project while being told, in plain language, exactly which files they cannot read and why. We will watch this happen to a real second person in Chapter 5; for now, recognize the placeholder as the secrecy axis speaking clearly.

### Whole-branch secrecy: secret refs and secret havens

Marking individual files is right when most of a branch is public and a few files are secret. Sometimes you want the opposite: a branch where *everything* is encrypted — a staging branch full of deployment artifacts, or a private vault of sensitive notes. Marking every file individually would be error-prone, so Haven lets you declare an entire ref secret:

```sh
hv secret ref staging
  marked staging secret (whole tree encrypted at rest)
```

Now every file committed on `staging` is encrypted at rest, regardless of its name — the mark is the *branch*, not a filename pattern. This is the secrecy axis applied wholesale. Combine it with a haven and you get the most protected cell of Chapter 1's grid — a private branch that is *also* entirely encrypted — in a single command:

```sh
hv haven create vault --secret
  created private vault
  marked vault secret (whole tree encrypted at rest)
```

The `vault` haven is doubly protected: it is private (never pushed to a team server — the access axis) *and* its entire tree is ciphertext at rest (the secrecy axis). An attacker who somehow obtained the database still finds only ciphertext for `vault`'s contents, and a misconfigured push still cannot send it. This is belt-and-suspenders for genuinely sensitive material, and the fact that it is one command — `haven create … --secret` — reflects how cleanly the two independent axes compose.

### Rotation: keeping recipients correct over time

A secret's recipients are "whoever can read the branch" — but readership *changes*. Someone joins the team and should now be able to decrypt; someone leaves and should not. The act of re-encrypting a secret to the *current* set of readers is **rotation**, and it is a single command:

```sh
hv secret rotate
  rotated 1 secret object(s) on main to 1 recipient(s)
```

Rotation re-encrypts every secret on the current branch to whoever can currently read it, *without creating a new commit* — it updates the at-rest ciphertext to match the present access policy. You run it after a membership change: grant a teammate read access, then `rotate` so the existing secrets become readable to them; revoke someone, then `rotate` so the secrets are re-encrypted without them. (The grant/revoke commands themselves are Chapter 5; here we are learning the rotation half of the dance.)

Because forgetting to rotate would silently leave your secrets encrypted to a *stale* recipient set, Haven helps you notice. The `secret status` command tells you whether the secrets' recipients have drifted from the current readership:

```sh
hv secret status
  no secret drift
```

`no secret drift` means every secret is currently encrypted to exactly the people who should be able to read it — recipients and readers agree. If they had drifted (say you added a member but had not rotated yet), `status` would warn you, turning "did I remember to rotate?" from a nagging worry into a checkable fact. This is the same philosophy as the rest of Haven: replace a discipline you must remember with an observable property you can verify.

### A footgun worth knowing: your name is your identity

One sharp edge deserves a direct warning, because it can lock you out of your own policy in a way that is confusing if you do not understand it. In Haven, an *actor* is identified by a **name** (the `user.name` you configured), and your authority — your admin grant, your membership in the keyring — is recorded under that name when you run `hv key gen`. If you later *change* `user.name`, you become, as far as the policy is concerned, a *different person*: your signatures now claim a name that is not in the keyring, and policy-mutating commands fail with an error like `policy v1 signer "NewName" not in parent keyring`.

The practical rule is simple: **set your `user.name` once, before `hv key gen`, and leave it.** If you saw that error while experimenting, it almost certainly means your current `user.name` differs from the name under which the policy was bootstrapped — fix it by setting `user.name` back to the founding name. This is a consequence of access being modeled around human-readable actor names (which makes policies legible — you read "Ada may write" not "key a3f2… may write"), and the trade-off is that the name is load-bearing. Treat your actor name like a username on a system: chosen once, then stable.

## Proving both pillars at once

Before the exercises, let's connect the two pillars with a single demonstration that exercises everything. Create a secret haven, put a secret in it, and verify both guarantees — that the haven refuses to push *and* that its contents are ciphertext at rest:

```sh
hv haven create vault --secret
echo "MASTER_KEY=do-not-leak-7777" > vault-notes.txt
hv haven switch vault && hv add vault-notes.txt && hv commit -m "vault"

# Secrecy: the master key is ciphertext in the database
grep -a "do-not-leak-7777" .haven/haven.db && echo LEAK || echo "encrypted at rest"
  encrypted at rest

# Access: pushing the private vault to a team remote is refused
hv push origin vault
  hv push: refusing to push private haven "vault"; …
```

The two checks together are the whole chapter in miniature. The `grep` confirms the secrecy axis: the master key is not in the database in plaintext, so any copy of that database lacks it. The refused `push` confirms the access axis: the vault cannot be sent to a team server even when you ask directly. One file, both protections, each verifiable with an independent observation — secrecy with a generic text search, access with the tool's own refusal and exit code. This is what "private *and* encrypted" means operationally, and you now know how to produce and verify it.

---

## Exercises

Work these in a throwaway repo whose `user.name` you set *before* `hv key gen` (heed the footgun). Each builds intuition for one slice of the two pillars.

### Exercise 4.1 — Create and confirm a haven is private

**Problem:** Create a haven, commit to it, and confirm via the right list command that it is a private ref and not a public branch.

**Solution:**
```sh
hv haven create spike
  created private spike
hv haven switch spike
echo x > x.txt; hv add x.txt; hv commit -m spike
hv haven list      # shows: spike
hv branch list     # does NOT show spike
```

**Explanation:** The two list commands are the proof: `spike` appears under `hv haven list` (private namespace) and is absent from `hv branch list` (public namespace). This confirms that a haven is genuinely a different *kind* of ref, not just a branch with a label, and that the two namespaces are separate as the chapter described. Internalizing which command shows which kind prevents the common confusion of "where did my branch go?" — havens and branches are siblings in different drawers, and you open the right drawer with the right command.

### Exercise 4.2 — Watch the push refusal exit non-zero

**Problem:** Attempt to push a haven to a team remote and capture the command's *exit status* (not just its message). Why does the non-zero exit matter?

**Solution:**
```sh
hv push origin spike   # prints the refusal
echo "exit=$?"         # exit=1  (non-zero)
```

**Explanation:** The non-zero exit is what makes the refusal *safe in automation*, not just at an interactive prompt. A script doing `hv push origin spike && deploy` would stop, because the `&&` sees the failure; a tool that returned zero on a refusal would let the script believe it had pushed and march on. (Be careful measuring this through a pipe — `cmd | tail` reports the pipe's exit, not `cmd`'s; capture `$?` directly.) The lesson generalizes: a protection that only prints a warning is weaker than one that also signals failure through the exit code, because only the latter is honored by the machinery built on top of it.

### Exercise 4.3 — Publish a haven

**Problem:** Graduate a haven into a public branch and confirm the new branch is now pushable (conceptually — full push is Chapter 5).

**Solution:**
```sh
hv publish spike --as ready
  published spike -> branch ready at <hash>
hv branch list     # now shows: ready
```

**Explanation:** `publish` is the one-way door from "not for sharing" to "fit for the world," and confirming `ready` appears under `hv branch list` shows it became a genuine public branch, not a relabeled haven. The deliberateness of the step is the point: you do not drift from private to public by accident, you *decide* to publish, and the command records that decision. This mirrors a real workflow — a spike incubates privately until you judge it ready, then graduates in one explicit move — and it keeps the access axis honest, since nothing becomes public without a conscious act.

### Exercise 4.4 — List and extend the secret marks

**Problem:** Show the default secret marks, then add a mark for a custom secret file your project uses, and explain how the mark will affect future `add`s.

**Solution:**
```sh
hv secret list                  # shows the defaults (.env, *.pem, secrets/**, …)
hv secret add "config/*.token"
  marked "config/*.token" as secret (matching files will be encrypted on add)
```

**Explanation:** Marks are the mechanism that makes encryption *selective and automatic* rather than manual. By matching filenames against globs, Haven encrypts the files most likely to be sensitive without you flagging each one, and adding a mark extends that automatic protection to your project's particular secrets. The crucial property is that the mark is stored in the repo and so travels to everyone who clones — protection becomes a project-level fact rather than something each contributor must remember, which is the whole structural-not-procedural thesis applied to secrecy.

### Exercise 4.5 — Prove a marked file is ciphertext at rest

**Problem:** Add a file matching a mark, commit it, and demonstrate with `grep` that its contents are not in the database in plaintext.

**Solution:**
```sh
echo "API_TOKEN=zzz-SECRET-42" > config/app.token
hv add . && hv commit -m "token"
grep -a "zzz-SECRET-42" .haven/haven.db && echo LEAK || echo "encrypted"   # -> encrypted
```

**Explanation:** This is the Chapter 2 verification applied to a *custom-marked* file, proving the mark you added actually triggers encryption. The `grep` routes around Haven entirely, so a clean miss means the plaintext is genuinely absent from the stored bytes — and therefore from any backup or push of those bytes. Re-running this verification whenever you add a new kind of secret is a cheap, high-confidence habit: it confirms the mark matched and the encryption fired, rather than trusting that the glob was written correctly. A `LEAK` result would mean your glob did not match the file, sending it to the store in plaintext — exactly the mistake the check catches.

### Exercise 4.6 — Make a whole branch secret

**Problem:** Declare an entire ref secret so that every file on it is encrypted, and explain when you would choose this over per-file marks.

**Solution:**
```sh
hv secret ref staging
  marked staging secret (whole tree encrypted at rest)
```

**Explanation:** A secret ref shifts the unit of secrecy from "files matching a name pattern" to "this entire branch," which is the right tool when most or all of a branch is sensitive — a deployment branch of artifacts, a vault of notes — and naming every file would be tedious and error-prone. Per-file marks suit a mostly-public branch with a few secrets (the typical app repo); secret refs suit a branch that is sensitive in its entirety. Choosing the coarser tool when it fits avoids the failure mode of forgetting to mark one file among many, because there is nothing to forget — the branch itself is the mark.

### Exercise 4.7 — Compose both axes into a secret haven

**Problem:** Create a single ref that is both private (never pushed) and fully encrypted at rest, and name the two independent guarantees it gives you.

**Solution:**
```sh
hv haven create vault --secret
  created private vault
  marked vault secret (whole tree encrypted at rest)
```
Guarantees: (1) *access* — `vault` is a haven, so `push` refuses to send it to a team server; (2) *secrecy* — its whole tree is ciphertext at rest, unreadable without an authorized key.

**Explanation:** This exercise is the grid from Chapter 1 collapsed into one command, and the value is feeling that the two axes truly are independent yet composable. Privacy (an access property, enforced by the server/push refusal) and encryption (a secrecy property, enforced by cryptography) are layered without interfering — remove either and the other still holds. For genuinely sensitive material you want both, because they defend against different failures: the haven defends against *exfiltration via a server*, the encryption defends against *theft of the at-rest bytes*. One command, two orthogonal defenses.

### Exercise 4.8 — Rotate and check for drift

**Problem:** With a secret committed, run `secret status`, then `secret rotate`, and interpret both outputs. What does "drift" mean?

**Solution:**
```sh
hv secret status
  no secret drift
hv secret rotate
  rotated N secret object(s) on main to M recipient(s)
```
"Drift" means the set of recipients a secret is *currently encrypted to* has diverged from the set of people who *currently can read* its branch.

**Explanation:** `status` turns the worry "are my secrets encrypted to the right people?" into a checkable fact, and `no secret drift` is the all-clear: recipients equal readers. Rotation is the corrective action that re-encrypts secrets to the *current* readership, which you run after any membership change so the at-rest ciphertext keeps matching the access policy. The reason drift detection exists is that the binding rule (recipients = readers) is only maintained if you rotate after access changes — `status` is the safety net that flags the gap before it bites, embodying Haven's habit of replacing "remember to do X" with "you can verify whether X is needed."

### Exercise 4.9 — Trigger and diagnose the name footgun

**Problem:** In a fresh repo, run `hv key gen`, *then* change `user.name`, then attempt `hv secret ref staging`. Explain the error and the fix.

**Solution:**
```sh
hv key gen                       # bootstraps policy under the current name, e.g. "olduser"
hv config user.name "newname"
hv secret ref staging
  hv secret: policy v1 signer "newname" not in parent keyring
# Fix:
hv config user.name "olduser"    # back to the bootstrapped name
hv secret ref staging            # now succeeds
```

**Explanation:** The error appears because authority is bound to your *actor name*, recorded at `key gen` time; signing a policy change under a different name makes you an unrecognized actor whose entry is not in the keyring. The fix is to restore the original name — your key did not change, only the label you were presenting. Understanding this prevents a genuinely baffling lockout, and it teaches the underlying model: Haven's access policy is written in terms of human names for legibility, which makes the name a stable identifier you should choose once and keep. Treat changing `user.name` after bootstrap the way you would treat changing your system username — something with consequences, not a cosmetic tweak.

---

## Mini-projects

### Mini-project 4.A — The incubator: haven to public branch

**Description:** Model the real lifecycle of a risky feature: develop it privately in a haven where it cannot leak, confirm the leak-proofing, then graduate it to a public branch when it is ready. This is the everyday use of the access axis.

**Concepts practiced:** `haven create`/`switch`, the push refusal, `publish`.

**Requirements:** Show the haven refusing to push while private, then show the published branch as an ordinary public branch.

**Walkthrough:** Create a haven for the risky work and make a couple of commits on it, treating it exactly like a normal branch. Before publishing, attempt to push it to a (team) remote and observe the refusal and non-zero exit — this is the safety you are relying on during the private phase. Once the feature is ready, `publish --as` it into a public branch, and confirm with `hv branch list` that the result is a genuine public branch that *would* push. The arc — private incubation, verified leak-proofing, deliberate graduation — is the canonical haven workflow.

**Solution:**
```sh
hv haven create risky && hv haven switch risky
echo "experimental" > feature.txt; hv add feature.txt; hv commit -m "spike"
echo "more"        >> feature.txt; hv add feature.txt; hv commit -m "iterate"

# during the private phase, leaking is structurally prevented:
hv push origin risky ; echo "exit=$?"
  hv push: refusing to push private haven "risky"; …
  exit=1

# when ready, graduate it:
hv publish risky --as risky-feature
  published risky -> branch risky-feature at <hash>
hv branch list      # risky-feature now present as a public branch
```

**Explanation:** This mini-project is the access axis across a feature's whole life, and the instructive moment is the contrast between the two phases. While `risky` is a haven, `push` refuses it and exits non-zero — so no automation, teammate, or reflex can leak the unfinished work; the protection is the tool, not your memory. After `publish`, `risky-feature` is an ordinary branch with none of that refusal, because you made the conscious decision that it is ready. The workflow encodes a healthy discipline structurally: experiments stay private by default and become public only by an explicit act, which is exactly inverted from mainstream tools where everything is publishable by default and privacy is the thing you must remember to protect.

### Mini-project 4.B — A secret with a verified lifecycle

**Description:** Take a secret through its whole life in a solo repo: mark it, commit it encrypted, verify it is ciphertext at rest, check for drift, and rotate. This rehearses every secret command except the multi-person grant/revoke, which Chapter 5 adds.

**Concepts practiced:** marks, automatic encryption, `grep` verification, `secret status`, `secret rotate`.

**Requirements:** End with a proof that the secret is ciphertext at rest and a `secret status` of `no secret drift`.

**Walkthrough:** Add a custom mark for your secret file so you exercise `secret add`, then create and commit the file, watching `add` report `(1 encrypted)`. Verify at rest with `grep`. Run `secret status` to confirm recipients match readers (`no secret drift` on a solo repo, since the only reader is you). Finally `rotate` and read the `to N recipient(s)` line as the binding rule made visible. The sequence is exactly what you will do for real, minus the membership changes that make rotation interesting — which you will add in Chapter 5.

**Solution:**
```sh
hv secret add "secrets/*.txt"
  marked "secrets/*.txt" as secret …
mkdir secrets
echo "ROOT_PW=correct-horse" > secrets/db.txt
hv add . && hv commit -m "db secret"
  staged 1 file(s) (1 encrypted)

grep -a "correct-horse" .haven/haven.db && echo LEAK || echo "encrypted"   # -> encrypted
hv secret status
  no secret drift
hv secret rotate
  rotated 1 secret object(s) on main to 1 recipient(s)
```

**Explanation:** Running the full sequence makes the secret lifecycle concrete and shows how the commands fit together: the mark makes encryption automatic, `add` reports it, `grep` proves it, `status` confirms the recipients are correct, and `rotate` re-establishes that correctness on demand. On a solo repo every number is one, which is deliberately boring — the point is to learn the *shape* of the operations in a setting with no other variables, so that when Chapter 5 introduces a second member and the recipient count jumps to two, nothing about the mechanics surprises you. The `no secret drift` line is the one to cherish: it is Haven certifying that your secrets are encrypted to exactly the right people, a fact you can check any time rather than hope for.

### Mini-project 4.C — Build the most-protected ref and attack it

**Description:** Create a secret haven (private *and* fully encrypted), put genuinely sensitive content in it, then play attacker: try to push it, and try to read its contents from the raw database. Document that both axes hold independently.

**Concepts practiced:** composing both axes, the attacker's-eye view from Chapter 1, independent verification of each guarantee.

**Requirements:** Two attacks, two defenses — a refused push (access) and a failed `grep` (secrecy) — each tied back to which axis defends it.

**Walkthrough:** Make a `vault` haven with `--secret`, switch to it, and commit a file with an unmistakable secret string. Then attack it two ways. First, attempt `push origin vault` to a team remote — the haven refuses (access axis). Second, `grep` the database for the secret string — it is absent because the whole tree is encrypted (secrecy axis). Write one line per attack naming the axis that stopped it, so you end with a tidy demonstration that the two protections are genuinely separate and both active.

**Solution:**
```sh
hv haven create vault --secret && hv haven switch vault
echo "MASTER_KEY=do-not-leak-7777" > vault/master.txt
hv add vault/master.txt && hv commit -m "vault master key"

# Attack 1 — exfiltrate via a server (defended by the ACCESS axis):
hv push origin vault ; echo "exit=$?"
  hv push: refusing to push private haven "vault"; …
  exit=1

# Attack 2 — read the at-rest bytes (defended by the SECRECY axis):
grep -a "do-not-leak-7777" .haven/haven.db && echo LEAK || echo "encrypted at rest"
  encrypted at rest
```

**Explanation:** This is Chapter 1's attacker's-eye view performed with your own hands on the most-protected kind of ref. Attack 1 is stopped by the access axis — the haven is private, so `push` refuses and exits non-zero, meaning the vault cannot be exfiltrated through a team server even on a direct request. Attack 2 is stopped by the secrecy axis — the tree is encrypted, so the master key is not in the database bytes and thus not in any copy of them. The two defenses are independent: one is about *movement* (enforced by the push refusal), the other about *content* (enforced by encryption), and you just confirmed each separately. Composing them gives a ref that resists both a server-side exfiltration and a raw-bytes theft, which is precisely the belt-and-suspenders posture you want for a master key — and you proved it rather than assumed it.

---

## Summary

This chapter operated Haven's two reasons to exist. On the **access** axis, you created *havens* — private branches living in their own namespace, developed exactly like ordinary branches, but which `push` structurally *refuses* to send to a team server (exiting non-zero so automation honors the refusal). You graduated a haven to a public branch with `publish`, and learned that `sync` to a *personal* remote is the legitimate lane for carrying privates between your own machines, keeping "follows me across my laptops" and "never reaches the team" simultaneously true. On the **secrecy** axis, you saw that *marks* (with safe defaults like `.env` and `*.pem`, extensible via `secret add`) make encryption automatic and selective; that the binding rule encrypts each secret to exactly its branch's readers; that non-recipients get an honest `<haven: encrypted secret; you are not a recipient>` placeholder; and that `secret ref` and `haven --secret` extend secrecy to a whole branch. You learned rotation (`secret rotate`) and drift detection (`secret status`) as the way recipients stay correct over time, and you met the one real footgun — authority is bound to your actor *name*, so set `user.name` before `key gen` and keep it.

The single idea to carry forward is that access and secrecy are *independent yet composable*: a private haven, an encrypted file, or both at once in a secret haven, each verifiable by an independent observation — a refused push for access, a failed `grep` for secrecy. You proved both guarantees with your own hands rather than trusting a success message, which is the habit this whole book is training.

So far everything has been solo: one identity, one reader, recipient counts of one. Chapter 5 adds the other people. We will stand up a server, push and clone over the network, and meet the **signed policy** — the members, groups, and grants that decide who can read and write — watching the recipient count climb from one to two as a teammate joins, secrets rotate to include them, and the server enforce a policy it can check but cannot forge. Everything you learned here about secrets and havens is about to gain a cast of characters.
