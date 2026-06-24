# Chapter 1 — Why Haven Thinks Differently

Before you type a single command, it is worth understanding the itch Haven was built to scratch. Plenty of version control systems exist, and they are very good at what they do. Haven does not try to replace them at tracking history — it borrows that wholesale. What it changes is the assumption underneath two everyday acts that mainstream tools quietly leave to your willpower: keeping some work private, and keeping secrets out of your history. This chapter is the one place in the book where we slow down and build a mental model. Get this model right and the rest of Haven feels obvious; skip it and the commands will seem arbitrary.

By the end of this chapter you will be able to explain, to a colleague, exactly what "privacy" and "secrecy" mean as *separate* properties in Haven, why a public branch can safely hold an encrypted password while a private branch can hold a plaintext draft, and what an attacker who steals a full copy of your repository actually walks away with. That last question — *what does copying the repo give them?* — is the lens we will keep returning to, because it is the question that reveals whether a guarantee is real or merely polite.

## The two things mainstream VCS leaves to discipline

Think about the last time you worked on something half-baked — a spike, an experiment, a security fix you did not want public yet. In a mainstream tool, that work lives on a branch that is mechanically identical to every other branch. The only thing standing between your half-baked spike and the whole world is *you remembering not to push it*. One absent-minded `push --all`, one overly helpful IDE button, and the work is public. The tool did exactly what it was told. The safety was never in the tool; it was in your memory.

Now think about secrets. Your application needs a database password, an API key, a TLS private key. They live in a `.env` file or a `config/secrets.yaml`. The moment you commit that file, its plaintext bytes are written into history — and history is forever. It is in every clone, every backup, every fork, every CI cache. You can delete the file in a later commit, but the bytes remain in the object store, one `git show` away. Again, the tool behaved correctly. It faithfully recorded what you gave it. The problem is that "faithfully record everything in plaintext" is the wrong default when some of what you hand it is a secret.

Notice the shape these two problems share. In both cases the tool is *procedural*: it offers you a capability (push a branch, commit a file) and trusts you to use it wisely. The protection, such as it is, is a habit you maintain. Habits fail. People are tired, rushed, new to the team, or simply human. A guarantee that depends on no one ever making a mistake is not a guarantee — it is a hope.

Haven's central bet is that these two protections should be **structural** instead of procedural. A private branch should be one that the push command *physically cannot* send to a shared server, no matter how you invoke it. A secret should be a file whose bytes are *already ciphertext* by the time they reach the object store, so that "leaking" it leaks gibberish. When the protection is built into how data is stored and moved, your memory is no longer load-bearing.

## Two axes, not one

Here is the single most important idea in Haven, and the one most likely to trip you up if you carry over intuitions from other tools. Privacy and secrecy are **two independent axes**, not one slider. People new to Haven tend to collapse them — "private means encrypted, public means plaintext, right?" — and then nothing makes sense. Let's pull them apart deliberately.

The first axis is **access**: *who is allowed to fetch a given ref from a server?* This is about visibility and movement. A branch can be **public** (anyone the server lets in can fetch it), **restricted** (only named people or groups — need-to-know), or it can be a **haven**, which is private and *never travels to a team server at all*. Access is enforced by the **server**: it decides what to hand out and what to refuse.

The second axis is **secrecy**: *are a file's bytes encrypted at rest?* This is about the content itself, independent of where it lives or who can fetch the ref. A file is either stored as plaintext or stored as ciphertext. Secrecy is enforced by **cryptography**: the bytes are encrypted before they are stored, so even something that holds the bytes — a backup, a server's disk, a stolen laptop — sees only ciphertext unless it also holds a private key.

```
                      SECRECY  (is the file's content encrypted at rest?)
                      plaintext            ciphertext
                  +----------------------+----------------------+
        public    | ordinary open-source | a public repo whose  |
                  | code on a public     | .env is encrypted to |
 ACCESS           | branch               | the branch's readers |
 (who can    -----+----------------------+----------------------+
  fetch the       | a need-to-know       | a restricted branch  |
  ref?)  restricted| branch of plaintext | holding encrypted    |
                  | design docs          | deployment secrets   |
                  +----------------------+----------------------+
        haven     | a private draft —    | a private vault: not |
       (private)  | plaintext, but never | pushed AND every byte |
                  | leaves your machine  | encrypted at rest     |
                  +----------------------+----------------------+
```

Spend a moment reading the grid above, because every cell is legal and useful. The top-right cell is the one that surprises people: a **public** branch holding an **encrypted** secret. That is not a contradiction — it is the normal case. Your open-source project is public; its production `.env` is encrypted to the handful of maintainers who can read it. Anyone can clone the repo and read the code; only those maintainers can decrypt the `.env`. Access says "everyone may fetch this branch"; secrecy says "but the bytes of this one file are ciphertext to all but a few." The two axes do not fight; they compose.

The bottom-left cell is the mirror surprise: a **private** branch holding **plaintext**. Your work-in-progress spike has no secrets in it — it is just embarrassing and unfinished. You do not need to encrypt it. You need it to *not leave your machine*. So it lives in a haven: plaintext on your disk, but structurally un-pushable to the team server. The secrecy axis is irrelevant here; the access axis is doing all the work.

Once you see access and secrecy as two dials you can set independently, Haven's commands stop being a grab-bag and become a small, orthogonal toolkit. Commands like `haven` and `restrict` and `serve` move the access dial. Commands like `secret` and `key` move the secrecy dial. They were designed to be combined.

## The rule that ties the axes together

If the two axes were *completely* independent you would face a miserable bookkeeping problem: one list of who can read a branch, and a *separate* list of who can decrypt its secrets, kept in sync by hand forever. That is exactly the kind of procedural trap Haven exists to avoid, so it imposes a single binding rule:

> **A secret's decryption recipients are exactly the people who can read its branch.**

Read that twice. It means you never manage two lists. When you grant someone read access to a branch, the ability to decrypt that branch's secrets comes with it, automatically. When you revoke their access, you re-encrypt so they can no longer decrypt (a one-command operation we will meet in Chapter 4). Access *is* the key to secrecy. The "who can see this" question has one answer, not two, and Haven derives the cryptographic recipient set from the access policy rather than asking you to maintain it.

This rule is why the two axes, despite being independent in *what* they control, are linked in *how you operate them*. You think in terms of access — "the deployers can read the staging branch" — and secrecy follows. It is the quiet design decision that makes the whole system humane to use, and we will see it pay off concretely when we rotate secrets after a teammate leaves.

## The attacker's-eye view

The cleanest way to test whether a privacy guarantee is real is to imagine the strongest reasonable attacker and ask what they get. For Haven, the canonical attacker has done the worst plausible thing: they have obtained a *complete copy of your repository* — every object, every ref, the whole `.haven/haven.db` file. Maybe they cloned a public server, maybe they grabbed a backup, maybe they stole a laptop with the repo on it (but, crucially, not the private key, which lives elsewhere — more on that in Chapter 2).

```
   What's in the clone           Stored as              What the attacker gets
   --------------------------    -------------------    ----------------------------
   ordinary code                 plaintext              the code (same as Git)
   policy: keyring + grants       public keys + sigs     nothing to unlock anything with
   secret files / secret refs     ciphertext             garbage, without a private key
   private havens                 (absent on a team      not present at all — they were
                                   server)               never pushed
```

Walk down that table, because it is the whole value proposition in four rows. The ordinary code is plaintext, exactly as it would be in any VCS — Haven is not trying to hide your source from people you handed the repo to. The access policy is present, but it consists of *public* keys and signatures: it tells the attacker who the members are and what they are allowed to do, but it contains nothing they can decrypt with. The secret files are ciphertext; without an authorized private key they are noise. And the private havens are not even there — on a properly configured team server they were never accepted in the first place, so a thief of that server's disk finds no trace of them.

This is what "built in, not bolted on" actually cashes out to. The guarantee does not depend on the attacker being nice, or on a feature being toggled on, or on you having remembered something. It depends on math (the secrets are ciphertext) and on storage rules (havens were never sent). Those hold whether or not anyone was paying attention.

There is an honest footnote to all this, and a book that respects you must state it: Haven has not had an external security audit. The design is sound and the implementation is tested, but "trust this with the nuclear codes against a nation-state" is a claim no un-audited system should make. Haven is built for solo work and small trusted teams running over TLS. Keep that scope in mind; we will revisit the precise threat boundaries in Chapter 5.

## Proving it to yourself

Models are cheap; demonstrations are convincing. You have not installed Haven yet — that is Chapter 2 — but it is worth previewing the single most reassuring demonstration so you know what you are working toward. Once you have a repo with an encrypted `.env`, you can ask the operating system, not Haven, whether the secret is really ciphertext on disk:

```sh
# (Chapter 2 sets this up; shown here so you know the payoff.)
echo "API_KEY=sk-LEAKME-12345" > .env
hv add . && hv commit -m "app with secret"

# Now grep the raw database for the plaintext. It must NOT be there.
grep -a "sk-LEAKME-12345" .haven/haven.db && echo "LEAK" || echo "encrypted: not found in db"
```

The reason this matters is that it bypasses Haven entirely. You are not asking the tool "did you encrypt it?" and trusting the answer — you are searching the bytes of the database file with a generic text tool. If `grep` cannot find your secret in the database, then a backup of that database does not contain your secret either, because it is the same bytes. This is the difference between a feature and a guarantee: a guarantee survives you being skeptical of the thing making the claim. When you reach the end of Chapter 2 you will run exactly this and watch it print `encrypted: not found in db`.

## What you can do after this book

It helps to know where the path leads. After Chapter 2 you will have a working `hv`, an identity, and a first encrypted commit. After Chapter 3 the daily loop — stage, commit, branch, merge, undo — will be second nature. After Chapter 4 you will keep experiments in havens that cannot leak and manage secrets that re-key themselves around your team. After Chapter 5 you will run a server two people share, with a signed policy that the server enforces but cannot forge. None of that requires you to become a cryptographer. It requires only the mental model you just built: two axes, one binding rule, and the discipline of always asking *what does copying the repo give them?*

---

## Exercises

These first exercises are deliberately conceptual — Haven is not installed yet. They check that the *model* has landed, which is what the rest of the book depends on. Write your answers down before reading the solutions; articulating them is how the model sticks.

### Exercise 1.1 — Name the axes

**Problem:** In one sentence each, define Haven's two axes and say who enforces each.

**Solution:** The **access** axis controls *who can fetch a ref from a server* (public, restricted, or a never-pushed haven), and it is enforced by the **server**. The **secrecy** axis controls *whether a file's bytes are encrypted at rest*, and it is enforced by **cryptography**.

**Explanation:** The two halves people most often get wrong are the enforcers. Access is a *runtime* decision made by a server handing out (or refusing) data, which is why it only bites when you involve a server. Secrecy is a *storage* property baked into the bytes before they are written, which is why it holds even with no server in sight — on your own disk, in a backup, anywhere the bytes go. Keeping the enforcers straight is what lets you reason about offline copies versus server requests, a distinction that returns in every later chapter.

### Exercise 1.2 — Fill the grid

**Problem:** Give a concrete, realistic example for each of these three cells: (a) public + ciphertext, (b) haven + plaintext, (c) restricted + ciphertext.

**Solution:** (a) An open-source web app on a public `main` branch whose `.env` (database URL, API keys) is encrypted to the three maintainers. (b) A private `wip-redesign` haven holding rough, unfinished UI code with no secrets in it — it just is not ready to show anyone. (c) A `staging` branch restricted to the `deployers` group, carrying encrypted deployment credentials only those deployers can read.

**Explanation:** The point of the exercise is to feel that all three are *ordinary*, not exotic. Case (a) is most software with secrets; case (b) is every spike you have ever been embarrassed by; case (c) is a deployment branch. If you found yourself reaching for contrived examples, re-read the grid: the cells correspond to real situations you already encounter, which is precisely why Haven gives each its own dial.

### Exercise 1.3 — The two-list trap

**Problem:** Explain, in your own words, what bookkeeping problem the rule "a secret's recipients are exactly the branch's readers" eliminates.

**Solution:** It eliminates having to maintain two separate, hand-synchronized lists — one for "who may fetch this branch" and another for "who may decrypt this branch's secrets." With the rule, there is a single source of truth (the access policy), and the cryptographic recipient set is derived from it automatically.

**Explanation:** Two lists kept in sync by hand is a classic source of security bugs: the lists drift, someone is removed from access but still in the decryption set (or vice versa), and the gap is invisible until exploited. By making access the single input from which recipients are derived, Haven removes the entire category of "the two lists disagreed" failures. This is the same philosophy as the rest of the system — replace a discipline you must maintain with a structural guarantee.

### Exercise 1.4 — Attacker triage

**Problem:** An attacker steals a complete copy of a team server's repository. For each of these, state whether they obtain it and in what form: (a) the source code on `main`, (b) the production database password (a secret on `main`), (c) a developer's private `experiment` haven, (d) the list of who the team members are.

**Solution:** (a) They get the source code, in plaintext. (b) They get the password, but as ciphertext — useless without an authorized private key. (c) They do not get it at all; havens are never pushed to a team server, so it is not on that disk. (d) They get the member list (public keys and grants), in the clear — the policy is not secret, only unforgeable.

**Explanation:** Each answer maps to a row of the attacker's-eye table, and getting them right means the model is doing its job. The subtle ones are (c) and (d). For (c), the protection is *absence*, not encryption — the strongest possible defense is that the data was never there. For (d), recognize that Haven does not treat the membership roster as a secret; knowing *who* the deployers are does not let you *become* one, because that requires a private key you do not have. Confidentiality (you cannot read the secrets) and authenticity (you cannot forge the policy) are different goals, and Haven gives you the second without pretending to give you the first for the roster.

### Exercise 1.5 — Why grep the database?

**Problem:** Why is grepping `.haven/haven.db` for a secret's plaintext a more convincing test than running a Haven command that reports "this file is encrypted"?

**Solution:** Because `grep` is a generic tool that has no stake in the answer. It reads the literal bytes of the database file. A Haven command that *says* "encrypted" is making a claim you would have to trust; `grep` finding nothing proves the plaintext is not in those bytes, so any copy of those bytes (a backup, the server's disk) is equally free of the plaintext.

**Explanation:** This is the essence of verifying a security property rather than accepting an assurance. The trustworthy tests are the ones that route around the system under test. Throughout this book we favor demonstrations of this kind — kill a process and check the repo survived, clone with the wrong key and check you get gibberish — over taking a success message at face value. Train yourself to ask, "what independent thing can I observe that would be false if the guarantee were broken?"

### Exercise 1.6 — Public yet encrypted

**Problem:** A teammate says, "If the branch is public, encrypting the `.env` on it is pointless — anyone can clone it." What is wrong with this reasoning?

**Solution:** Cloning the branch gets them the *ciphertext* of the `.env`, not its contents. "Public" (an access property) means anyone allowed by the server may fetch the ref; it says nothing about whether the bytes they fetch are readable. Without an authorized private key, the cloned `.env` is gibberish. So encrypting it is the entire point: it lets the code be open while the credentials stay closed.

**Explanation:** The teammate has collapsed the two axes into one — the exact mistake this chapter warns against. Their intuition comes from tools where "in the repo" equals "in plaintext," so "anyone can clone" equals "anyone can read everything." Haven breaks that equivalence: being in the repo and being readable are separate facts. This is also why the top-right grid cell is the common case, not a curiosity — open code with closed secrets is what most real projects need.

### Exercise 1.7 — Where the key is not

**Problem:** The attacker stole a laptop with a full repo clone on it but did *not* get the contents of `~/.config/haven/identity`. Can they read the encrypted secrets? Why does the location of that file matter?

**Solution:** No. The private key needed to decrypt the secrets lives in `~/.config/haven/identity`, which is deliberately *outside* the repository. A repo clone — however complete — never contains the private key, so it never contains the means to decrypt. The location matters because it is what makes "copying the repo" insufficient: the repo and the key are separate artifacts, and the secret stays safe as long as they stay separate.

**Explanation:** This is the structural reason the attacker's-eye guarantee holds. If the private key lived in the repo, every clone would carry the means to unlock itself and the encryption would be theater. By keeping the key in a per-user config location that is never committed and never pushed, Haven ensures the repository is a *lock without a key*. We will see in Chapter 2 exactly when and where that file is created, and why Haven tightens its permissions defensively.

### Exercise 1.8 — Procedural vs. structural

**Problem:** Classify each as a *procedural* protection (depends on a person remembering) or a *structural* one (enforced by the system): (a) "we agreed never to push the `secret-fix` branch," (b) "the `secret-fix` haven cannot be pushed to the team server," (c) "we always remember to delete `.env` before committing," (d) "files matching `.env` are encrypted automatically on commit."

**Solution:** (a) procedural, (b) structural, (c) procedural, (d) structural.

**Explanation:** The (a)/(b) and (c)/(d) pairs are the same goal achieved the two different ways, and seeing them side by side is the whole thesis of the chapter. The procedural versions ((a), (c)) are agreements — they work right up until the moment someone forgets, and they fail silently. The structural versions ((b), (d)) are properties of the tool — they work whether or not anyone is thinking about them, and a slip-up produces a refusal or an automatic encryption rather than a leak. Haven's job is to convert the procedural column into the structural column wherever it can.

---

## Mini-projects

Because Haven is not installed yet, these mini-projects are *design exercises* — you produce a short written artifact, not running code. They rehearse the planning you will do for real in later chapters, and each has a worked solution you can compare against.

### Mini-project 1.A — Map your own repository

**Description:** Take a real project you work on and place every sensitive part of it onto Haven's two-axis grid. The goal is to translate your existing, implicit security habits into the explicit model.

**Concepts practiced:** the access axis, the secrecy axis, the binding rule.

**Requirements:** List at least five distinct artifacts from a real project (branches, config files, credentials, docs). For each, state its access setting (public / restricted / haven) and its secrecy setting (plaintext / ciphertext), and one sentence of justification.

**Walkthrough:** Start by listing artifacts without thinking about Haven at all — just "what's in this project that I care about?" A typical list: the main source tree, a production `.env`, a `docs/internal-roadmap.md`, an experimental `perf-rewrite` branch, and a `deploy/keys/prod.pem`. Now take each in turn and ask the two questions separately. *Who should be able to fetch this?* sets the access dial. *Should its bytes be encrypted at rest?* sets the secrecy dial. Resist setting both from a single gut feeling; force yourself to answer them independently, because that is the skill.

**Solution:**

```
artifact                  access       secrecy      justification
------------------------  -----------  -----------  ----------------------------------
src/ (main branch)        public       plaintext    it's the product; teammates read it
.env (on main)            public       ciphertext   credentials; readable only to maintainers
docs/internal-roadmap.md  restricted   plaintext    leadership-only, but not a secret value
perf-rewrite branch       haven        plaintext    unfinished; must not leak; no secrets in it
deploy/keys/prod.pem      restricted   ciphertext   deploy key; deployers only, encrypted
```

**Explanation:** The instructive part of this artifact is that no two rows have the same pair of settings, which proves the axes are genuinely independent for real work. The `.env` and the roadmap differ on *secrecy* but could share *access*; the roadmap and the haven differ on *access* but share *secrecy* (both plaintext). If your own map collapsed everything into just "public+plaintext" and "private+ciphertext," look again — you have probably mislabeled a public-but-secret credential or a private-but-not-secret draft. Doing this mapping once for a real project is the fastest way to internalize that "private" and "encrypted" are different words for different things.

### Mini-project 1.B — Write the attacker's report

**Description:** Adopt the role of an attacker who has stolen a complete copy of a team's Haven repository, and write the short, honest report you would deliver to whoever hired you: what you obtained, what you did not, and why.

**Concepts practiced:** the attacker's-eye view, plaintext vs. ciphertext, the absence of havens, key location.

**Requirements:** Three paragraphs — "what I got," "what I couldn't get," and "what would change the answer." Be specific and unflinching; the value is in the honesty.

**Walkthrough:** Begin from the attacker's-eye table and turn each row into a sentence of plunder or frustration. The first paragraph is the loot: the source code and the membership roster, both readable. The second is the wall you hit: the secrets are ciphertext and the private keys were never in the repo, and the developers' havens were never on this server to begin with. The third is the most useful for your "client": under what conditions would the picture change — and that is where you name the real risks (a stolen *private key*, a server misconfigured as `personal` so it accepts havens, secrets sent over plaintext HTTP instead of TLS).

**Solution:**

> **What I got.** A complete, readable copy of the source on every public branch, identical to what a normal clone yields. I also recovered the full access policy: the list of members, their public keys, the groups, and every grant. I know exactly who the admins and deployers are.
>
> **What I couldn't get.** Every secret file is ciphertext. I have the encrypted `.env` and the encrypted deploy key, but not a single private key to decrypt them — those live in each user's `~/.config/haven/identity`, which is never in the repository. I found no private development branches; the team's havens were never pushed to this server, so they are simply not here. Knowing who the deployers are does not let me become one, because I cannot forge the signed policy without an admin's signing key.
>
> **What would change the answer.** Steal a *developer's private key* and their encrypted secrets become readable. Find a server accidentally run with `--kind personal` and it may be holding havens it should have refused. Catch a `push` going over plaintext HTTP rather than TLS and I could read secrets in transit even though they are encrypted at rest only against the wrong threat — so confirm TLS everywhere.

**Explanation:** This report is valuable precisely because it refuses to overclaim. It names real, plaintext-readable loot (code, roster) so you do not fool yourself into thinking Haven hides everything — it does not, and it does not try to. It then identifies the genuine residual risks, all three of which are *operational* rather than cryptographic: protect private keys, configure servers correctly, use TLS. That is the honest security posture of the system, and writing it from the attacker's chair fixes it in your mind better than any reassuring paragraph could. Keep this report in mind in Chapter 5, where we configure the server and remote that these risks hinge on.

### Mini-project 1.C — Design a secret-sharing plan for a two-person team

**Description:** On paper, design how you and one collaborator will share a single production `.env` on a public `main` branch such that both of you can decrypt it and no one else can, and sketch what must happen when a third person joins and later leaves.

**Concepts practiced:** the binding rule (recipients = readers), rotation, the lifecycle of access.

**Requirements:** Describe the initial two-person setup, the change when a third member joins, and the change when they later leave — in terms of *access* decisions, letting secrecy follow from the rule.

**Walkthrough:** Anchor everything to the binding rule so you never reason about encryption directly. Initially, two people can read `main`, so the `.env` is encrypted to exactly those two — you do not choose recipients, the readership chooses them. When a third person is granted read access to `main`, the readership grows to three, so the secret must be re-encrypted to all three; that re-encryption is the "rotate" step. When they leave, you revoke their access (readership shrinks back to two) and rotate again so the now-removed person can no longer decrypt — note that they may still possess the *old* ciphertext from before, which is why rotation matters and why you also rotate the underlying credential if it was ever truly exposed.

**Solution:**

```
phase            access on main        recipients of .env       action needed
---------------  --------------------  -----------------------  -----------------------
start (2 people) you, collaborator     you, collaborator        encrypt on first commit
joiner added     +newcomer (3 total)   you, collaborator, new   grant read, then rotate
joiner leaves    revoke newcomer (2)   you, collaborator        revoke, rotate, and
                                                                 change the credential
                                                                 itself if it was exposed
```

**Explanation:** The plan reads entirely in the *access* column, with the *recipients* column following mechanically — which is the binding rule doing its job and the whole reason Haven is pleasant to operate at this. The one piece of real-world judgment the rule cannot make for you sits in the last cell: rotation re-encrypts so the departed person cannot decrypt *future* fetches, but if they ever held the plaintext, they still know it. So when someone leaves under anything but the friendliest terms, you rotate Haven's encryption *and* rotate the actual secret value (issue a new database password, revoke the old API key) at the source. Haven controls who can decrypt the stored ciphertext; only the upstream system can invalidate a credential a human has already seen. We will perform the `grant`, `revoke`, and `rotate` commands for real in Chapter 4 — you are designing now what you will execute then.

---

## Summary

This chapter built the mental model that the rest of the book stands on, so let's restate it as a single connected thought rather than a list. Mainstream version control leaves two protections — keeping work private and keeping secrets out of history — to your discipline, and discipline fails because people are human. Haven makes both protections structural by separating them onto two independent axes: **access**, which controls who can fetch a ref and is enforced by the server, and **secrecy**, which controls whether a file's bytes are encrypted at rest and is enforced by cryptography. These axes compose freely — public code with encrypted secrets is the everyday case, and a private plaintext draft is just as legitimate — and they are tied together by one humane rule: a secret's decryption recipients are exactly the people who can read its branch, so you never maintain two lists.

The test we will keep applying is the attacker's-eye view: a thief of a complete repository copy walks away with plaintext code and a readable-but-unforgeable policy, but only ciphertext for secrets and nothing at all for havens, because the private keys live outside the repo and havens are never pushed. That is what "built in, not bolted on" means in practice — guarantees that survive your skepticism and other people's mistakes.

If you take one thing from this chapter into the next, let it be the instinct to ask, of any operation, *what does this give someone who copies the repository?* Hold that question. In Chapter 2 we stop theorizing, install `hv`, create a real repository and identity, and run the very `grep` demonstration previewed above — watching, with our own eyes and a generic tool, a secret become ciphertext on disk.
