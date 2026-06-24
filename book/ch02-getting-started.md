# Chapter 2 — Getting Started

In Chapter 1 we stayed in the world of ideas: two axes, a binding rule, and an attacker we kept interrogating. Now we make it real. By the end of this chapter you will have built the `hv` binary, created an actual repository, generated the identity that anchors all of Haven's cryptography, recorded your first commit — and, most importantly, watched a secret turn into ciphertext on disk using nothing but a generic text-search tool. That last demonstration is the moment the abstract guarantee from Chapter 1 becomes something you have seen with your own eyes.

We are going to move slowly and explain every artifact Haven creates, because the *layout* of a Haven repository — what lives where, and especially what deliberately lives *nowhere near* the repo — is itself part of the security model. If you rush past "where does the private key go?" you will later struggle to reason about backups, clones, and stolen laptops. So treat this chapter as a guided tour, not just a setup script.

## From a model to a machine

Everything in Chapter 1 was a promise. A promise is only worth as much as the machinery that keeps it, and Haven's machinery is a single small program called `hv`. There is no daemon humming in the background, no service to register, no central account. A Haven repository is a self-contained thing on your disk, and `hv` is the tool that reads and writes it. That simplicity is intentional: the fewer moving parts a security tool has, the fewer places a guarantee can quietly break.

Because `hv` is the only piece of software involved, getting it onto your machine is the entire installation. There is nothing to configure globally, no system service, no root access required for normal use. We build it from source, which has the pleasant side effect of proving your toolchain works and giving you a binary you fully control.

## Building `hv`

Haven is written in Go and compiles to a single statically linked binary with no C dependencies. That phrase — "static, no cgo" — is worth unpacking, because it is a deliberate engineering choice with practical consequences for you. A static binary carries everything it needs inside one file; you can copy it to another machine of the same OS and architecture and it simply runs, with no shared libraries to install and no version-skew surprises. "No cgo" means the build does not reach out to a system C compiler or link against system C libraries, which is what makes the static, portable result possible in the first place.

The project ships a `Makefile` so you never have to remember the exact compiler invocation. Building is one command, run from the repository's source tree:

```sh
make build      # produces ./hv in the current directory
```

When that finishes you have an executable named `hv` sitting in the directory. You can run it in place as `./hv`, but for daily use you will want it on your `PATH` so you can type `hv` from any project. Copy it somewhere your shell already looks for programs — the conventional choices are a personal `~/.local/bin` or a system `/usr/local/bin` — or use the project's install target if one is provided. Once it is on your `PATH`, confirm the install answered by asking for its version:

```sh
hv version
  hv v0.1.0-205-gc31dc35-dirty (linux/amd64, go1.25.11)
```

Read that line, because it tells you three useful things. The leading `v0.1.0-…` is the version derived from the source's git history, so you can always trace a binary back to the exact commit it was built from. The `linux/amd64` is the platform the binary targets — a static binary is portable *within* an OS and architecture, not across them, so a Linux build will not run on macOS. And the `go1.25.11` records the compiler version, which matters because security-relevant fixes in Go's standard library (TLS, certificate handling) ride along with the toolchain. If your output differs in the version string, that is fine; what matters is that the command ran and printed a coherent line rather than "command not found."

A common first stumble here is building successfully but then typing `hv` and getting "command not found." That means the binary built fine but is not on your `PATH` — you are likely still in a different directory than the one holding `hv`. Either run it as `./hv` from the directory where it was built, or finish the install by moving it onto your `PATH`. The fix is never to rebuild; the build already succeeded.

## Creating your first repository

With `hv` available, pick or create an empty directory for a throwaway project — we will make a small one to learn with. Inside it, the very first command turns an ordinary directory into a Haven repository:

```sh
hv init
  initialized empty haven repository in /home/you/demo/.haven
```

What just happened is more interesting than the one line of output suggests. Haven created a hidden subdirectory called `.haven`, and *that directory is the entire repository*. Unlike systems that scatter state, a Haven repo's history, refs, configuration, staged files, and objects all live inside a **single SQLite database file**. Let's look:

```sh
ls -la .haven
  -rw-r--r-- HEAD          # which branch you're on
  -rw-r--r-- haven.db      # the entire repository: objects, refs, config, staging
```

The presence of just two entries is the lesson. `HEAD` is a tiny file naming your current branch (it starts on `main`). `haven.db` is everything else — every commit you ever make, every file version, every ref, the access policy, all of it — in one self-describing database file. This single-file design is why a Haven repository is so easy to reason about, back up, and copy: there is one file to think about. It is also why, throughout this book, we can *grep the database* to inspect what is really stored. When someone asks "where is my repository?", the honest answer is "that one file."

You will sometimes notice two extra files appear briefly during writes, `haven.db-wal` and `haven.db-shm`. These are SQLite's write-ahead log and shared-memory files; they are part of how the database stays consistent even if the process is killed mid-write (a property we will lean on heavily). They come and go on their own — you never manage them, and they are not separate parts of your repo so much as the database's own scratch space.

## Telling Haven who you are

Commits record *who* made them, so before committing you set your name and email. These are stored in the repository's config — they describe the author of commits, and are separate from the cryptographic identity we set up next. Set them once per repository:

```sh
hv config user.name "Ada Lovelace"
hv config user.email "ada@example.com"

hv config user.name        # read a value back
  Ada Lovelace
```

It is worth being clear about what these are *not*. `user.name` and `user.email` are descriptive metadata — they label your commits so history is readable, exactly as in other version control systems. They are not credentials and they grant no authority; anyone can set any name. The thing that actually proves *you are you* to a server, and that lets you decrypt secrets, is the cryptographic identity we create in the next section. Keeping that distinction sharp now will save confusion in Chapter 5, where authority comes from a signing key, never from a name.

## Your identity: the key that anchors everything

Here is the step that makes Haven *Haven* rather than a smaller VCS. You generate a cryptographic identity — a pair of keypairs, actually — that does two jobs: it lets you decrypt secrets you are entitled to, and it lets you sign your actions so a server can verify them. You do this once, and it is stored *outside* any repository, in your user config:

```sh
hv key gen
  generated identity at /home/you/.config/haven/identity
  recipient: age19z5785jw0gm83qn2p4avc5jnx55penht05dgkh8t0yvr2v7649kql3ksnl
  bootstrapped policy: you ("Ada Lovelace") are the founding admin
```

Three things happened, and each deserves attention. First, a file was written to `~/.config/haven/identity` — note the location, *outside* the repo. This file holds your **private** keys, and it is the single most sensitive artifact in the whole system. Haven writes it with owner-only permissions (mode `0600`) inside a directory it locks down to `0700`, and it fails closed if it cannot secure those permissions — it would rather refuse than leave your private key readable by others. This is the structural reason the attacker from Chapter 1 cannot decrypt your secrets from a repo copy: the key is simply not in the repo, and never will be.

Second, the output printed your **recipient** — the long string starting `age1…`. This is your *public* encryption key. The word "recipient" is the right mental image: when someone encrypts a secret so that you can read it, you are a recipient, identified by this string. It is safe to share; it is how others address encryption to you. Under the hood Haven uses `age` (a modern, well-regarded encryption format) with X25519 keys, but you do not need to know the cryptography — you need to know that this public string is shareable and the private file is not.

Third, and easy to miss: `bootstrapped policy: you are the founding admin`. Because this is a brand-new repository, the act of generating your identity also created the repository's **access policy** with you as its first administrator. The policy is the signed record of who may do what — we devote much of Chapter 5 to it — and every repository needs a root of authority to start from. By generating your key in a fresh repo, you became that root. There is a second keypair involved too, an Ed25519 *signing* key, which is what makes your policy entries unforgeable; `key gen` created it alongside the encryption key.

You can see your public half of both keys at any time, which is exactly what you hand to a teammate's admin when you want to be added to *their* repository:

```sh
hv key show
  sign: 1e24fa4ccace0b6d0868185c90adba818a816c6d9f77a0c98ba2d6012c364617
  enc:  age19z5785jw0gm83qn2p4avc5jnx55penht05dgkh8t0yvr2v7649kql3ksnl
```

The two lines are your two *public* keys: `sign` is the Ed25519 verification key (proves your signatures are yours), and `enc` is the `age` recipient (lets others encrypt to you). Both are public and shareable — that is their entire purpose. Notice what is *not* shown and never will be by this command: the private halves. There is no `hv key export-private` you might run by accident; the private material stays in the protected file and the tooling gives you no easy way to spill it. That asymmetry — public keys are trivially viewable, private keys are deliberately awkward to extract — is a small design decision that prevents a large class of accidents.

## Your first commit — and an encrypted secret

Now the satisfying part. We will create two files: an ordinary `README.md` and a `.env` that, because it matches Haven's built-in secret marks, will be encrypted automatically. Then we stage, commit, and inspect — paying close attention to the moment encryption happens.

```sh
echo "hello haven" > README.md
echo "API_KEY=sk-LEAKME-12345" > .env
```

Before staging anything, ask Haven what it sees. The `status` command is your situational awareness — it tells you what is staged, modified, and untracked, and you will run it constantly:

```sh
hv status
  On main
  No commits yet

  Untracked files:
    .env
    README.md
```

Both files are "untracked," meaning Haven knows they exist but is not yet recording them. "No commits yet" confirms this is a virgin repository on the `main` branch. Nothing is encrypted yet, because nothing has been staged — encryption happens as part of staging a marked file, not the instant the file appears on disk. Now stage both files with `add`, and watch the output carefully:

```sh
hv add .
  staged 2 file(s) (1 encrypted)
```

That parenthetical — `(1 encrypted)` — is Haven telling you, unprompted, that one of the two files was encrypted as it was staged. You did not ask for encryption; you did not pick which file. Haven recognized that `.env` matches a secret mark (we will list and customize those marks in Chapter 4) and encrypted its contents on the way into the object store, while `README.md`, matching no mark, went in as plaintext. This is the secrecy axis working automatically, exactly as Chapter 1 described: the protection is structural, triggered by the file's name rather than your memory.

Staging changed the picture; `status` now reflects it:

```sh
hv status
  On main
  No commits yet

  Changes to be committed:
    .env
    README.md
```

Both files have moved from "untracked" to "changes to be committed." They are staged — recorded in the index, ready to be sealed into a commit. The final step turns that staged snapshot into a permanent, addressable point in history:

```sh
hv commit -m "first commit"
  [main 38d03891d7] first commit
```

The output names the branch (`main`) and the start of the new commit's hash (`38d03891d7…`). That hash is not arbitrary — it is the SHA-256 of the commit's content, which is what makes Haven *content-addressed*: identical content always produces the identical hash, and any tampering changes the hash, so the address of an object is also a checksum of it. You can see your commit in the history:

```sh
hv log
  commit 38d03891d7055c0c18483d11f440f7de079215e907069509c461352fdb227399
  Author: Ada Lovelace <ada@example.com>
  Date:   Wed, 24 Jun 2026 12:28:39 EEST

      first commit
```

There is your name and email (the config you set earlier), the full commit hash, and your message. This is ordinary version-control history — the part Haven borrows wholesale. What is *not* ordinary is sitting silently in the database: the `.env` you just committed is in there as ciphertext. Let's prove it.

## The payoff: watch the secret become ciphertext

This is the demonstration promised at the end of Chapter 1, and it is the single most important thing you will do in this chapter. We will search the raw bytes of the repository database for the plaintext of our secret, using `grep` — a tool that knows nothing about Haven and has no incentive to lie:

```sh
grep -a "sk-LEAKME-12345" .haven/haven.db && echo "LEAK" || echo "encrypted: not found in db"
  encrypted: not found in db
```

Sit with that result for a moment. We wrote `API_KEY=sk-LEAKME-12345` into `.env`, staged it, committed it — and the literal string `sk-LEAKME-12345` does **not** appear anywhere in the database file. The `-a` flag told `grep` to search the binary file as text, so it would have found the plaintext if it were there in the clear. It is not there because Haven encrypted it before storing it. And because the database file *is* the repository, this means a backup of that file, a copy sent to a server, or a clone handed to a colleague all equally lack the plaintext. The secret is ciphertext at rest, full stop.

Now confirm the other half of the story — that *you*, the authorized owner, still see plaintext in your working tree:

```sh
cat .env
  API_KEY=sk-LEAKME-12345
```

Your working copy of `.env` is plaintext, because you hold the private key and Haven decrypts the file for you on checkout. This is the secrecy axis in full: ciphertext in the store, plaintext in the hands of a recipient. The file is simultaneously "encrypted" (in the database, to anyone without the key) and "readable" (in your working tree, because you have the key). Those are not in tension — they are the whole point, and now you have observed both ends of it directly rather than taking anyone's word.

## A map of where Haven keeps things

Let's consolidate the tour into a single mental map, because knowing what lives where is how you reason about every later operation. There are exactly two locations that matter, and the separation between them is load-bearing.

```
  ~/.config/haven/identity            <project>/.haven/
  --------------------------          ------------------------------------
  YOUR PRIVATE KEYS                   THE REPOSITORY
  (age + ed25519 secret halves)       HEAD        -> current branch name
  mode 0600, dir 0700                 haven.db    -> ALL history, refs,
  never in a repo                                     config, staging, and
  never pushed to a server                            objects (secrets as
  one per user, all repos share it                    ciphertext)
```

The left box is *you*; the right box is *the project*. The private keys on the left are never copied into the right — that is the structural fact underpinning the entire attacker's-eye guarantee from Chapter 1. Everything on the right can be copied, backed up, pushed, and cloned freely, because everything sensitive on the right is already ciphertext, and the only thing that could decrypt it lives on the left, outside the repo. When you back up a project, you back up the right box and the secrets stay safe. When you set up a new machine, you bring the left box (carefully) so that *you* can still decrypt. Keeping these two boxes mentally distinct is most of what it takes to use Haven safely.

---

## Exercises

Do these at a real terminal in a throwaway directory. The whole point of Haven being a small, fast, single-file tool is that experimenting is cheap — make repos, break them, delete them. Each exercise has a complete solution and an explanation; try first, then compare.

### Exercise 2.1 — Build and verify

**Problem:** Build `hv` from source and confirm it runs. What does the version string tell you about the binary?

**Solution:**
```sh
make build
hv version
  hv v0.1.0-…-… (linux/amd64, go1.25.11)
```
The version string encodes the source commit (the `v0.1.0-…` derived from git), the target platform (`linux/amd64` — OS and architecture), and the Go toolchain version (`go1.25.11`).

**Explanation:** Verifying the build by running `version` rather than just trusting that `make` exited zero is the same skepticism we applied to the `grep` test — observe the program working rather than assume it. The platform field matters more than beginners expect: a static binary is portable *within* an OS/architecture pair but not across them, so a colleague on macOS needs their own build. The toolchain field matters because Haven's network security partly rides on the Go standard library's TLS and certificate code, which is patched by upgrading the toolchain — so "which Go built this" is a security-relevant fact, not trivia.

### Exercise 2.2 — Anatomy of a fresh repo

**Problem:** Run `hv init` in an empty directory and list the contents of `.haven`. Explain what each entry is.

**Solution:**
```sh
hv init
ls -la .haven
  HEAD        # a small file naming the current branch (starts as "main")
  haven.db    # a single SQLite database holding the entire repository
```

**Explanation:** The surprise for newcomers is how *little* there is — two files, one of them tiny. Almost everything you think of as "the repo" (commits, file versions, refs, config, the access policy, staged changes) is inside the one `haven.db` file. This single-file design is what makes a Haven repository trivial to copy or back up (one file) and is the reason this book can keep grepping the database to show you what is really stored. `HEAD` is just a pointer to which branch you are on; it changes when you switch branches.

### Exercise 2.3 — Public versus private keys

**Problem:** After `hv key gen`, run `hv key show`. Which of the printed values are safe to post publicly, and which value is *not* printed at all — and why does that matter?

**Solution:** Both printed values — `sign` (Ed25519 public key) and `enc` (the `age` recipient) — are public and safe to share. The private halves are *not* printed by any normal command; they stay in `~/.config/haven/identity` with `0600` permissions.

**Explanation:** The asymmetry is the security property. Public keys exist to be shared — `enc` so others can encrypt secrets to you, `sign` so others can verify your signatures — so making them easy to view is correct. The private keys are the crown jewels; if they leak, an attacker can decrypt your secrets and forge your actions. By giving you an easy command to show the *public* keys and no easy command to extract the *private* ones, the tool nudges you toward safe behavior and away from accidentally pasting your private key into a chat window. The location and permissions of the identity file are the structural backstop behind that nudge.

### Exercise 2.4 — Make the secret appear and disappear

**Problem:** Create a `.env` with a memorable fake secret, commit it, and prove with `grep` that the plaintext is absent from `haven.db` while present in your working tree. Then explain why both facts are true at once.

**Solution:**
```sh
echo "DB_PASSWORD=hunter2-XYZZY" > .env
hv add . && hv commit -m "add secret"
grep -a "hunter2-XYZZY" .haven/haven.db && echo LEAK || echo "encrypted"   # -> encrypted
cat .env                                                                    # -> DB_PASSWORD=hunter2-XYZZY
```

**Explanation:** The two results are not contradictory because they observe two different things. The `grep` reads the bytes *at rest* in the database, which are ciphertext, so the plaintext is genuinely not there. The `cat` reads your *working tree*, where Haven decrypted the file for you on checkout because you hold the authorized private key. "Encrypted at rest, plaintext to a recipient" is one coherent state, and seeing both halves with two generic tools (`grep`, `cat`) is what turns the claim from a feature into an observed fact. This is the single most important demonstration in the book; if it ever printed `LEAK`, you would have found a serious bug.

### Exercise 2.5 — `add` tells you what it encrypted

**Problem:** Stage a directory containing one `.env` and two `.go` files in a single `hv add .`. Predict the exact count in the output before running it.

**Solution:**
```sh
hv add .
  staged 3 file(s) (1 encrypted)
```

**Explanation:** Predicting the output first is a small way to test whether you have internalized the secrecy rule. Three files were staged; exactly one (`.env`) matched a secret mark and was encrypted, while the two `.go` files did not match and went in as plaintext. The `(1 encrypted)` annotation is Haven proactively reporting its security-relevant action so you are never guessing whether a sensitive file was protected. If your prediction had been `(0 encrypted)` or `(3 encrypted)`, re-read the secrecy axis: encryption is selective and driven by marks, not all-or-nothing.

### Exercise 2.6 — Name is not authority

**Problem:** Set `hv config user.name` to "Grace Hopper" in a repo that you created. Does this give "Grace Hopper" any special powers? What actually carries authority?

**Solution:** No. `user.name` is descriptive metadata that labels commits; it grants nothing. Authority comes from your cryptographic *signing* key (the Ed25519 key in your identity), which is what a server verifies and what the access policy is bound to.

**Explanation:** Conflating a human-readable name with authority is a classic security mistake, and Haven deliberately separates them. Anyone can type any name into `user.name` — it is a label, like the "From" line on an envelope, which is why it can be forged trivially and therefore must never be trusted for access decisions. The thing that cannot be forged is a signature made with your private signing key, and that is what Chapter 5's policy actually checks. Keeping "who the commit says made it" separate from "who cryptographically proved they made it" is essential once a server is involved.

### Exercise 2.7 — The two boxes

**Problem:** From memory, draw the two-location map: where do your private keys live, and where does the repository live? For each, state whether it is safe to copy to a backup server.

**Solution:** Private keys live in `~/.config/haven/identity` (mode `0600`); the repository lives in `<project>/.haven/` (mostly the `haven.db` file). The repository is safe to back up freely because its secrets are ciphertext. The identity file is *not* safe to copy to an untrusted backup, because it contains the private keys that would unlock those secrets.

**Explanation:** This map is the operational core of using Haven safely, which is why it is worth being able to reproduce from memory. The asymmetry in backup safety follows directly from Chapter 1's attacker view: the repo is a lock without a key, so backing it up exposes nothing; the identity file *is* the key, so it demands the same care as any private key — encrypted backup, a password manager, or a hardware token, never a plaintext cloud folder. Most real-world Haven incidents would come from mishandling the left box, not the right one.

### Exercise 2.8 — Reproduce a repo's identity location on a second "machine"

**Problem:** Simulate a second machine by pointing `HOME` at a fresh empty directory, then run `hv key gen` and observe where the identity is written. What does this tell you about how the identity file is found?

**Solution:**
```sh
HOME=/tmp/machine2 hv key gen
  generated identity at /tmp/machine2/.config/haven/identity
  …
```
The identity path is derived from `HOME` (`$HOME/.config/haven/identity`), so each user/home gets its own identity, independent of any repository.

**Explanation:** This exercise reveals that the identity is a property of *you* (your home directory), not of any single repo — one identity serves all the repositories you work in. It also explains how you would set up a genuine second machine: you do not regenerate a key there (that would be a different identity, unable to decrypt your existing secrets), you securely *copy* your existing `~/.config/haven/identity` to the new machine. Understanding that the file is located via `HOME` is also handy for the throwaway experiments in this book, where isolating `HOME` gives each test its own clean identity without touching your real one.

---

## Mini-projects

### Mini-project 2.A — A reproducible "first repo" script

**Description:** Write a shell script that, from an empty directory, sets up a complete starter Haven repo — identity, config, a normal file, and an encrypted secret — and ends by proving the secret is encrypted at rest. This is the demonstration you will use to show a colleague what Haven does in sixty seconds.

**Concepts practiced:** `init`, `key gen`, `config`, `add`, `commit`, the `grep` verification.

**Requirements:** The script must be idempotent enough to run in a fresh temp dir, must not touch your real identity, and must print a clear PASS/FAIL for the encryption check.

**Walkthrough:** The trick to not touching your real identity is to isolate `HOME`, exactly as Exercise 2.8 showed — point it at a temp directory so `key gen` writes a throwaway identity. From there the script is just the chapter's session in order: init, set name/email, generate the key, write a normal file and a `.env`, add and commit, then run the `grep` check and translate its result into a human-readable verdict. End by also `cat`-ing the working-tree `.env` so the demo shows both halves: ciphertext in the db, plaintext for the owner.

**Solution:**
```sh
#!/bin/sh
set -e
work=$(mktemp -d)
export HOME="$work/home"; mkdir -p "$HOME"     # isolate identity from your real one
cd "$work" && mkdir demo && cd demo

hv init
hv config user.name "Demo User"
hv config user.email "demo@example.com"
hv key gen >/dev/null

echo "hello haven"            > README.md
echo "API_KEY=sk-DEMO-99999"  > .env
hv add . && hv commit -m "starter repo"

echo "--- proof ---"
if grep -aq "sk-DEMO-99999" .haven/haven.db; then
  echo "FAIL: secret plaintext found in database"
else
  echo "PASS: secret is ciphertext at rest"
fi
echo "working tree still readable to the owner:"
cat .env
```

**Explanation:** The script's value is that it is *self-contained and honest*: it isolates `HOME` so a curious colleague running it cannot clobber their real key, and it ends with an independent check rather than a reassuring message from Haven itself. The `grep -aq` is the same routed-around-the-system test from the chapter, with `-q` for a quiet exit status the `if` can branch on. Showing the `cat .env` afterward is what makes the demo land — without it, a skeptic might think the secret was simply lost or corrupted; with it, they see the file is perfectly readable to the authorized owner and gibberish to everyone else. Keep this script; it is the fastest possible pitch for the secrecy axis.

### Mini-project 2.B — Inventory what a clone would expose

**Description:** Build a small repo with a mix of public and secret files, then make a *copy* of just the `.haven/haven.db` file (simulating what an attacker who stole the repo would have) and catalog what is and is not recoverable from that copy alone, without your identity.

**Concepts practiced:** the attacker's-eye view made concrete, content-addressing, the role of the identity file's location.

**Requirements:** Produce a short report listing (a) what plaintext you can find in the copied database with `grep`, and (b) what you cannot, tying each result back to Chapter 1's model.

**Walkthrough:** Create a repo with `README.md`, a `notes.txt` of ordinary text, and a `.env` secret; commit them. Now copy `haven.db` to a separate location to stand in for the stolen artifact — crucially, do *not* copy your identity file, because the attacker does not have it. Then `grep` the copy for known strings: a phrase from `notes.txt`, and the secret value from `.env`. Catalog which searches hit and which miss, and write one sentence per result explaining it with the model.

**Solution:**
```sh
# in your demo repo, with README.md, notes.txt ("project kickoff notes"), .env (SECRET=zzz-TOKEN)
hv add . && hv commit -m "mixed content"
cp .haven/haven.db /tmp/stolen.db          # the attacker has THIS, not your identity

grep -a "project kickoff" /tmp/stolen.db   # HIT  -> plaintext file is readable
grep -a "zzz-TOKEN"       /tmp/stolen.db   # MISS -> secret is ciphertext
```
Report: plaintext files (`notes.txt`, `README.md`) are fully recoverable; the `.env` secret is not, because it was encrypted to recipients and the copied database contains no private key.

**Explanation:** This mini-project turns Chapter 1's table from an assertion into something you executed. The `notes.txt` hit confirms that Haven does *not* hide ordinary content — it never claimed to, and pretending otherwise would be the kind of overclaim this book avoids. The `.env` miss confirms the secrecy guarantee holds against the exact attacker we care about: someone with the complete repository but not your identity. The reason the miss happens is the separation of the two boxes from the chapter's map — the stolen database is the right box, the unstolen identity is the left box, and decryption requires both. Run this once and the phrase "ciphertext at rest" stops being jargon and becomes something you have personally verified.

### Mini-project 2.C — Set up a believable second machine

**Description:** Simulate working from two machines by creating one identity, building a repo with a secret, then on a "second machine" (a different `HOME`) demonstrate that copying the *identity* — not regenerating it — is what lets the second machine decrypt the secret.

**Concepts practiced:** the identity as a per-user artifact, why regeneration breaks decryption, safe key transport.

**Requirements:** Show that a freshly-generated *different* identity cannot decrypt the secret, but the *copied* original identity can.

**Walkthrough:** On "machine 1" (HOME `m1`), generate an identity and commit a `.env`. Copy the repo to a "machine 2" working directory. First, point machine 2 at its *own* fresh `HOME` and generate a brand-new identity — then try to read the secret; it should come back as a locked placeholder, because the new identity is not a recipient. Then replace machine 2's identity with a copy of machine 1's identity file and try again — now the secret decrypts. The contrast is the lesson.

**Solution:**
```sh
# machine 1
export HOME=/tmp/m1; mkdir -p "$HOME"
cd /tmp && rm -rf proj && mkdir proj && cd proj
hv init && hv key gen >/dev/null
echo "SECRET=alpha-omega" > .env && hv add . && hv commit -m s

# machine 2, WRONG identity (freshly generated, different keys)
export HOME=/tmp/m2; mkdir -p "$HOME"
hv key gen >/dev/null                 # a DIFFERENT identity
hv restore --source HEAD .env && cat .env
  restored 1 file(s) from HEAD
  <haven: encrypted secret; you are not a recipient>

# machine 2, RIGHT identity (copy machine 1's private key over)
cp -r /tmp/m1/.config /tmp/m2/.config
hv restore --source HEAD .env && cat .env
  restored 1 file(s) from HEAD
  SECRET=alpha-omega
```

**Explanation:** The placeholder line `<haven: encrypted secret; you are not a recipient>` is the system telling you, plainly, that the working identity is not on the recipient list — the secret is present as ciphertext but undecryptable. This is exactly what should happen with the wrong key, and it is why you cannot set up a second machine by simply running `key gen` there: that produces a *new person* as far as the cryptography is concerned, with no claim to your existing secrets. The fix — copying the identity file — underscores that your identity is the portable, precious thing, and that transporting it is a security-sensitive act deserving an encrypted channel (a password manager, an encrypted USB key, `scp` over SSH), never a public paste. This rehearses the real procedure you would follow to onboard your own laptop, and it foreshadows Chapter 4, where being or not being a "recipient" becomes the central idea of secret sharing.

---

## Summary

You arrived with a mental model and now you have a working system that embodies it. You built `hv` into a single static binary and confirmed it with `hv version`; you turned a directory into a repository with `hv init` and learned that the entire repo is essentially one SQLite file, `haven.db`, alongside a tiny `HEAD` pointer. You set descriptive authorship with `hv config`, and — the pivotal step — you generated your cryptographic identity with `hv key gen`, which wrote your private keys to `~/.config/haven/identity` (mode `0600`, outside any repo), printed your shareable public recipient, and made you the founding admin of the repository's signed policy. You then made a first commit containing an automatically encrypted `.env`, and you *proved* the encryption two independent ways: `grep` could not find the plaintext in the database, while `cat` showed it readable in your working tree.

The most durable takeaway is the two-box map: your private keys live with *you* (`~/.config/haven/identity`) and never enter a repository, while the repository (`.haven/haven.db`) holds everything else with its secrets already encrypted. That separation is not an implementation detail — it is the structural fact that makes every guarantee from Chapter 1 hold up, and it is the thing to protect above all else. If you internalize one habit from this chapter, let it be guarding the left box as carefully as the right box is carefree.

With a real repository and identity in hand, Chapter 3 turns to the daily loop you will run more than any other: staging and committing in earnest, reading history, branching, merging two lines of work with Haven's three-way merge, and — because everyone makes mistakes — undoing changes safely. The security model now fades into the background where it belongs, and we get to work like developers.
