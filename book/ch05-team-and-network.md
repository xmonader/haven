# Chapter 5 — Working as a Team

Everything so far has been solo. One identity, one reader, recipient counts that stubbornly stayed at one. That was on purpose — you needed to own the mechanics before adding the complication of *other people*. Now we add them. This chapter takes Haven onto the network and introduces the cast: a server that hands out refs, teammates who clone and push, and the **signed policy** that decides who may read and write. The grand payoff is watching the binding rule from Chapter 1 finally do something visible — a teammate is granted access, a secret rotates, the recipient count climbs from one to two, and that teammate clones over the network and decrypts a secret you encrypted before they existed in the policy.

This is also the chapter where the honest limits matter most, because the moment a server and a network are involved, new failure modes appear that cryptography alone does not cover — transport security, server configuration, key handling. We will end with a clear-eyed operating guide, the same one you sketched as an "attacker's report" back in Chapter 1. By the close you will be able to run a two-person Haven setup with confidence and know exactly where its guarantees stop.

## Serving a repository

A Haven repository is a self-contained file, so "sharing" it means running a small server that other people's `hv` clients can talk to over HTTP (or HTTPS). There is no central service to sign up for; you run `hv serve` on a machine others can reach, pointed at a repository, and it answers requests. The most important decision you make when serving is the server's **kind**, because that single flag determines whether the server will accept private havens:

```sh
hv serve --addr 127.0.0.1:18475 --kind team
```

The `--addr` flag sets the address and port to listen on, and `--kind team` declares this a *team* server — one that **refuses private refs**. This pairing of the two halves of the haven guarantee is what makes it airtight: the client's `push` refuses to *send* a haven (Chapter 4), and the team server refuses to *accept* one, so a private branch cannot reach the team even if a client were misconfigured or malicious. The other kind, `--kind personal`, *accepts* havens and is meant only for syncing your own machines; running a personal server is how you let your laptop and desktop share private work, and it is emphatically not what you expose to a team.

For any real deployment you serve over TLS, because plain HTTP sends request bodies in the clear — and while your *secrets* are encrypted at rest and therefore still ciphertext on the wire, plenty of other request content is not. You supply a certificate and key:

```sh
hv serve --kind team --addr :8473 --tls-cert cert.pem --tls-key key.pem
```

With `--tls-cert` and `--tls-key`, the server speaks HTTPS, and clients connect with `https://` URLs. The certificate and key here are the server's TLS material — entirely separate from anyone's Haven identity — and they are what give you confidentiality and authentication of the *transport*. Treat plain `http://` as a thing for local experiments only; the operating guide at the end of this chapter returns to why TLS is non-negotiable in production.

One behavior to fix in your mind now: an **anonymous** client — one that presents no identity — receives only **public** refs from the server. It can clone your public branches (getting plaintext code and ciphertext secrets), but it cannot see restricted refs, cannot fetch havens (which are not there anyway on a team server), and cannot write anything. Access for anything beyond public-read requires a signed, recognized identity, which is what the policy is all about.

## Remotes and the verbs that move data

On the client side, you tell `hv` about a server by adding a **remote** — a named URL with a kind that mirrors the server's:

```sh
hv remote add origin http://127.0.0.1:18475 --kind team
  added team remote origin -> http://127.0.0.1:18475
```

The name `origin` is conventional for "the main place I push to"; the `--kind team` records that this remote is a team server (so the client knows havens must not go there). With a remote defined, the data-moving verbs become available, and they divide cleanly by direction and by what they carry. To send your public branches up to the server:

```sh
hv push origin main
  pushed main -> origin (b061f8f9d7)
```

`push` sends the named branch (or your current branch if you name none) along with the objects it needs and the signed policy, and — as you proved in Chapter 4 — it *refuses* to send any private haven. The reciprocal verbs bring data down: `fetch` downloads refs and objects into local tracking refs without touching your working tree, `pull` does a fetch and then merges into your current branch, and `clone` creates a whole new local repository from a remote. Each respects the policy: a client only receives the refs its identity is allowed to read.

The cleanest way to see the *whole* picture — what travels and what does not — is a clone performed by a real teammate, which is exactly the scenario we build in the next section. But first we need that teammate to exist in the eyes of the system, and that is the job of the policy.

## The signed policy: where authority lives

Here is a question mainstream tools answer badly: *where does "who can read and write what" actually live?* In most hosted systems, the answer is "in the vendor's database." That has two consequences you may never have questioned. First, the rules do not travel with the repository — clone it elsewhere and the access list is gone, because it was never in the repo. Second, you cannot verify the rules offline; you must ask the vendor. Haven takes a different path: the access rules live *in the repository itself*, as a **portable, cryptographically signed policy** that anyone can verify offline and that a server can *enforce* but cannot *forge*.

Think about what "cannot forge" buys you. The server that hands out your refs is also a place an attacker might compromise. If the server could invent access rules, compromising it would mean compromising your access control. Because the policy is signed by admins' private keys — which the server does not hold — a compromised server can refuse to serve, or serve stale data, but it cannot *grant itself* or anyone else new access. The server is an enforcer, not an authority. That separation is the heart of Haven's access model, and the rest of this section is the mechanics of operating it.

### Members: the keyring

Before anyone can be granted anything, they must be a **member** — an actor whose public keys are recorded in the policy's keyring. Adding a member is how you say "this person, identified by these public keys, exists in this repository's world." You need their two public keys, which they obtain with `hv key show` (Chapter 2) and send you:

```sh
# bob runs `hv key show` and sends you his two public keys; then you:
hv member add bob <bob-sign-key> <bob-age-recipient>
  added member bob
hv member list
  alice	active	age1q3r6dm5yjxhe9egnfy26ycmlr69fznqxhdks8mdr0dtrx6gy2y9s66870c
  bob	active	age1grjwl7ecr8gaaau9qtty7cnds428vfmlaaugfdv48d6ukqgm63dq4dpf9y
```

The two keys play the two roles you learned in Chapter 2: the *sign* key lets the server verify that requests claiming to be from bob really are, and the *age recipient* lets secrets be encrypted *to* bob. `member list` shows the keyring — each actor, their status (`active`), and their recipient. Adding bob to the keyring does not yet let him *do* anything; it only makes him a known actor. Authority comes from grants.

### Grants: capabilities on ref patterns

A **grant** says "this subject may perform this verb on refs matching this pattern." The verbs form a hierarchy of increasing power — `read`, `write`, `force`, `grant`, `admin` — where each higher verb implies the ones below it. Granting bob read access to all branches looks like this:

```sh
hv grant bob read "refs/branches/**"
  granted bob read on refs/branches/**
```

Now bob may read any ref under `refs/branches/` — which, by the binding rule, also means secrets on those branches will be encrypted to include him after the next rotation. The verb hierarchy is worth internalizing: `read` fetches; `write` updates a ref normally; `force` allows non-fast-forward updates (rewriting history on the server); `grant` lets the holder give others access up to their own level; and `admin` is full control, including changing the policy itself. You give each person the least powerful verb that lets them do their job — bob, a contributor who should not rewrite history or change the policy, gets `write` on the branches he works and `read` elsewhere, never `admin`.

### Groups and need-to-know with `restrict`

Granting capabilities to individuals does not scale past a few people, so Haven has **groups** — named sets of actors you grant to collectively:

```sh
hv group create deployers
  created group deployers
hv group add deployers bob
  added [bob] to deployers
```

Now you can grant a capability to `deployers` once and manage membership by adding and removing people from the group, rather than editing grants one by one. Groups pair naturally with **restriction**, which is how you implement *need-to-know*: by default branches are publicly readable (recall the `public-read` grant on `refs/branches/**`), but you can mark a specific ref as restricted so that *only* named subjects reach it:

```sh
hv restrict staging --read deployers
  restricted staging to group deployers (public access removed)
```

The message says exactly what happened: the `staging` branch's *public* read access was removed, and now only the `deployers` group can read it. This is the **restricted** cell of Chapter 1's access axis — a ref that is neither fully public nor a private haven, but visible to a named circle. Combined with secrets, a restricted branch becomes a need-to-know channel whose contents are encrypted to exactly that circle, with the recipient set derived (binding rule again) from the restriction.

### Inspecting the policy

Because the policy is the authority, you must be able to read and verify it. Four sub-commands of `policy` cover this. `policy show` prints the current state — members, groups, grants, and which refs are restricted:

```sh
hv policy show
  policy v5 (signed by alice)
  members:
    alice [active]
    bob [active]
  groups:
    deployers: [bob]
  grants:
    [root-admin] alice admin refs/**
    [public-read] * read refs/branches/**
    [bob:read:2] bob read refs/branches/**
    [restrict-read:staging] deployers read refs/branches/staging
    [restrict-write:staging] deployers write refs/branches/staging
  restricted: [refs/branches/staging]
```

Read that as a snapshot of authority. It is `policy v5` because the policy is *versioned* — every change (adding bob, granting, grouping, restricting) produced a new version, each signed by an admin (`signed by alice`). The grants list shows the founding `root-admin` (alice has `admin` on everything), the default `public-read`, bob's read grant, and the two grants the `restrict` produced for `staging`. To ask the focused question "who can read this particular ref?", use `policy access`:

```sh
hv policy access refs/branches/main
  refs/branches/main:
    read: everyone (public)
    read: alice
    read: bob
```

This resolves the policy down to a concrete answer for one ref — `main` is publicly readable, and alice and bob are named readers. The last two sub-commands are about trust in the chain itself. `policy log` lists the chain of policy versions (each a signed object linked to its parent), and `policy verify` walks that chain and checks every signature back to the root:

```sh
hv policy verify
  policy chain valid: 6 version(s)
```

`policy chain valid: 6 version(s)` is the system certifying that every version in the policy's history is properly signed by an authorized admin and correctly linked to its parent — the offline, unforgeable verification that is the whole point of putting authority in a signed chain rather than a vendor's database. You can run `policy verify` on any clone, with no server, and know the access rules are authentic.

## The binding rule, across the network

Now the demonstration the whole book has been building toward. We have a repository with an encrypted `.env`, originally readable only by alice (recipient count: one). We add bob as a member and grant him read access. Then we **rotate**, and watch the recipient count change:

```sh
hv member add bob <bob-keys>
hv grant bob read "refs/branches/**"
hv secret rotate
  rotated 1 secret object(s) on main to 2 recipient(s)
```

There it is: `to 2 recipient(s)`. You did not tell Haven "encrypt to bob"; you told it "bob may read the branch," and the recipient set *followed* — the binding rule, finally visible, climbing from one to two. The secret is now encrypted to both alice and bob. The proof that this is real comes when bob, on his own machine with his own private key, clones the repository over the network and reads the secret:

```sh
# on bob's machine, with bob's identity:
hv clone http://127.0.0.1:18475 bobrepo
  fetched refs/policy
  cloned into bobrepo
cat bobrepo/.env
  DB_PASSWORD=hunter2-team
```

Bob sees the plaintext. Not because the server decrypted it for him — the server only ever held ciphertext — but because the secret was rotated to include bob's recipient key, and bob holds the matching private key on his own machine. Trace the whole arc: alice encrypted a secret to herself; she granted bob read access; rotation re-encrypted the secret to include bob; bob cloned the ciphertext over the network; bob's private key decrypted it locally. Access flowed into decryption automatically, exactly as the binding rule promises, and at no point did the server or the network see plaintext. This is Haven's central claim, demonstrated end to end with two real identities.

Revocation runs the same machinery backward. To remove bob, you revoke his access and rotate so the secret is re-encrypted without him: `hv member revoke bob` (or revoking the specific grant by its id with `hv revoke <id>`), then `hv secret rotate`. After that, bob's key is no longer a recipient, so future fetches give him only the placeholder. The standing caution from Chapter 1 still applies: rotation stops bob from decrypting *future* ciphertext, but if he ever saw the plaintext he still knows it — so when someone leaves under less-than-friendly terms, rotate Haven's encryption *and* change the underlying credential at its source.

## How requests are kept honest

Putting authority in a signed policy is only half the network story; the *requests* to the server must also be unforgeable, or an attacker could impersonate bob regardless of how well-signed the policy is. Every authenticated request to a Haven server is signed by the client's identity over a canonical description of the request: the method, the path, the time, a hash of the body, and a one-time **nonce**. That bundle of properties defeats a family of attacks at once. Signing the method and path stops an attacker from redirecting a valid signature to a different operation; signing the body hash stops tampering with the payload; signing the time bounds how long a captured request stays valid; and the one-time nonce, recorded by the server, stops a captured request from being *replayed* even within that time window.

You do not operate any of this directly — it happens whenever your client talks to a server with your identity attached — but understanding it tells you what the transport does and does not protect. It guarantees that a request genuinely came from the claimed member, unmodified, and is not a replay. It does *not*, by itself, hide the request from a network eavesdropper; that is TLS's job, which is why a production server runs over HTTPS. Signed requests give you *authenticity and integrity*; TLS gives you *confidentiality in transit*; encryption-at-rest gives you *confidentiality on disk*. Three different protections for three different threats, and a serious deployment uses all three.

## Keeping a shared repository healthy

A repository many people push to accumulates cruft — objects no longer reachable from any ref, and storage that could be packed more tightly. Haven gives you three maintenance commands, and a healthy shared repo runs them periodically. The first and most important is `fsck`, which verifies integrity:

```sh
hv fsck
  policy chain: 3 version(s) verified
  checked 7 objects (1 encrypted secrets), 2 refs
  ok: no corruption detected
```

`fsck` does three things worth noting: it verifies the *policy chain* (every signature back to root, as `policy verify` does), it checks that every *object*'s content still hashes to its address (catching any corruption, since the address is a checksum), and it confirms every *ref* resolves to a present commit graph. The `1 encrypted secrets` is `fsck` telling you it counted a secret it could not re-hash (secrets are addressed by their plaintext hash, which it cannot see) but otherwise accounted for. `ok: no corruption detected` is the all-clear. Run `fsck` after anything alarming — a crash, a disk scare, a suspicious clone — and after the other two maintenance commands.

The second command, `gc`, reclaims space by deleting objects unreachable from any ref:

```sh
hv gc
  removed 0 unreachable object(s); 7 kept
```

`gc` walks every ref, marks everything reachable, and deletes the rest — the objects left behind by resets, abandoned branches, and amended commits. It is safe by design: it takes a non-blocking lock so it will not run concurrently with a checkout or merge, and it applies a *grace period*, sparing objects written in the last ten minutes so a concurrent `commit` or `push` cannot have its just-written objects swept before its ref update lands. Use `hv gc --prune=now` to reclaim everything immediately, or `--prune=2h` for a custom window. The third command, `repack`, shrinks the database by storing similar objects as compact deltas against a base, then compacting the file:

```sh
hv repack
  repacked 0 object(s), reclaimed 0 bytes; 1 candidate(s) left whole
```

`repack` finds objects that closely resemble a recent one (the way successive versions of a file share most bytes) and stores the later one as a delta — the difference — rather than a full copy, verifying byte-for-byte that each delta reconstructs the original before committing the rewrite. It only ever shrinks the store, is safe to interrupt or re-run, and leaves secrets and the signed policy chain whole. On a long-lived repo, a periodic `gc` then `repack` then `fsck` keeps the database lean and proven-intact. (On the small demo repo above, the counts are zero because there is nothing yet to collect or pack — the commands report honestly that there was no work to do.)

A word on durability, because it underpins all of this: Haven's writes are crash-safe. Objects are written before the refs that point to them, and the database uses a write-ahead log, so a process killed mid-operation — power loss, `kill -9`, a crash — leaves the repository at either the old state or the new one, never a torn one. At worst a crash orphans some freshly-written objects, which `gc` later collects; it never dangles a ref at a missing object. This is not a hope; it is a tested property. You can pull the plug during a commit and reopen to a consistent repo.

## Operating safely: where the guarantees stop

A book that respects you names the edges. Haven's cryptographic guarantees are strong, but a real deployment lives in an operational context where the failures are usually *not* cryptographic. Here is the honest operating guide, and it is the same list of residual risks you wrote as an attacker back in Chapter 1.

First, **run team servers over TLS.** Encryption-at-rest protects your secrets on disk, and signed requests protect authenticity, but only TLS protects the *confidentiality of requests in transit* on an untrusted network. Plain `http://` is for localhost experiments. Second, **guard private keys above all.** The entire model rests on `~/.config/haven/identity` staying secret; a stolen private key makes that person's secrets readable and their actions forgeable. Back keys up encrypted, never paste them, and rotate the policy (revoke, re-add with a new key) if one is exposed. Third, **configure server kind correctly.** A server accidentally run with `--kind personal` will *accept* havens it should refuse — verify your team servers are `--kind team`. Fourth, and most important to say plainly: **Haven has not had an external security audit.** It is well-engineered and tested, suitable for solo and small-trusted-team use, but it is not the thing to bet a nation-state-grade secret on without professional review. Match the tool to the stakes.

None of these caveats undermine the core guarantees — they delineate them. Copying the repository still gives an attacker only ciphertext and an unforgeable policy; havens still cannot leak; the binding rule still ties decryption to access. The operating guide is about the *context* those guarantees live in, and using Haven well means tending that context as carefully as you tend your commits.

---

## Exercises

These need two identities. Simulate a second person by pointing `HOME` at a different directory for "bob," exactly as the chapter's captures did. Run a local `hv serve` in the background for the network exercises.

### Exercise 5.1 — Stand up a team server and clone anonymously

**Problem:** Serve a repo with a public branch and an encrypted secret, then clone it *anonymously* (a fresh `HOME` with no identity). What do you get for the code, and what for the secret?

**Solution:**
```sh
hv serve --addr 127.0.0.1:18500 --kind team &     # in alice's repo
# anonymous client (fresh empty HOME, no key gen):
hv clone http://127.0.0.1:18500 anon
cat anon/app.go    # plaintext code
cat anon/.env      # <haven: encrypted secret; you are not a recipient>
```

**Explanation:** The anonymous clone gets the public branch's code in plaintext — Haven never claimed to hide public content — but the `.env` materializes as the locked-notice placeholder because the anonymous client is not a recipient. This is the access and secrecy axes acting together over the network: public access lets you fetch the branch, but secrecy keeps the secret's bytes ciphertext to anyone without a key. It directly demonstrates the chapter's claim that anonymous clients receive public refs and nothing decryptable, and it is the network version of the Chapter 2 `grep` proof.

### Exercise 5.2 — Add a member from their public keys

**Problem:** Generate a second identity for "bob" in a separate `HOME`, get his public keys, and add him as a member in alice's repo. What two pieces of information do you need, and why two?

**Solution:**
```sh
# as bob (HOME=/tmp/bob): hv key gen; hv key show  -> sign + enc keys
# as alice:
hv member add bob <bob-sign-key> <bob-age-recipient>
  added member bob
```

**Explanation:** You need both of bob's public keys because they serve the two distinct cryptographic roles from Chapter 2: the *sign* key lets the server verify bob's requests are authentically his, and the *age recipient* lets secrets be encrypted *to* bob. Adding him to the keyring makes him a known actor but grants no power yet — membership is identity, not authority. The exercise also rehearses the real onboarding handshake: the newcomer runs `key show` and sends *public* keys (never the private file) to an admin, who records them.

### Exercise 5.3 — Watch the recipient count climb

**Problem:** With bob a member, grant him read on the branches, rotate, and observe the recipient count in the rotate output. Explain why you never specified bob as a recipient.

**Solution:**
```sh
hv grant bob read "refs/branches/**"
  granted bob read on refs/branches/**
hv secret rotate
  rotated N secret object(s) on main to 2 recipient(s)
```

**Explanation:** The count is two because alice and bob can now both read the branch, and the binding rule makes the recipient set *equal* the reader set — so granting read access is, transitively, granting decryption. You never named bob as a recipient because recipients are *derived* from access, not maintained separately; that derivation is the entire ergonomic win of Haven's model. This is the single most important behavior in the system to understand: you manage *who can read the branch*, and encryption recipients follow automatically, eliminating the two-list synchronization bug class from Chapter 1.

### Exercise 5.4 — Prove a recipient can decrypt over the network

**Problem:** Have bob clone the repo (with his identity) and read the secret. Why can he decrypt it when the server only ever stored ciphertext?

**Solution:**
```sh
# as bob:
hv clone http://127.0.0.1:18500 bobrepo
cat bobrepo/.env
  DB_PASSWORD=hunter2-team
```

**Explanation:** Bob decrypts because rotation re-encrypted the secret to include his recipient key, and bob holds the matching *private* key locally — the server never decrypted anything, it merely served ciphertext that happened to be decryptable by bob. This closes the loop the whole book has been building: access (the grant) flowed into decryption (the rotation included bob) which bob realized locally (his private key), with no plaintext ever touching the server or the wire. It is the end-to-end demonstration that Haven's confidentiality is real and survives the network, not a claim you have to take on faith.

### Exercise 5.5 — Least privilege with the verb hierarchy

**Problem:** Bob should be able to push to feature branches but must not rewrite history or change the policy. Which verb do you grant, and which do you withhold?

**Solution:** Grant `write` on the feature branches (`hv grant bob write "refs/branches/feature/**"`). Withhold `force` (which would allow history-rewriting non-fast-forward updates), `grant`, and `admin`.

**Explanation:** The verb hierarchy — read < write < force < grant < admin — exists precisely so you can hand out the *least* power that does the job. `write` lets bob make normal fast-forward updates to his branches, which is all a contributor needs; `force` would let him rewrite shared history, `grant` would let him hand access to others, and `admin` would let him alter the policy itself — none of which a regular contributor should have. Practicing least privilege contains the blast radius if bob's key is ever compromised: an attacker with bob's `write` can push commits, but cannot rewrite history or escalate access, because those capabilities were never granted.

### Exercise 5.6 — Need-to-know with groups and restrict

**Problem:** Make a `staging` branch readable only by a `deployers` group containing bob, removing public access. Confirm with `policy access` that the public can no longer read it.

**Solution:**
```sh
hv group create deployers && hv group add deployers bob
hv restrict staging --read deployers
  restricted staging to group deployers (public access removed)
hv policy access refs/branches/staging   # lists only deployers/members, not "everyone (public)"
```

**Explanation:** `restrict` implements the *restricted* cell of the access axis — a ref that is neither fully public nor a private haven, but visible to a named circle. Routing the grant through a *group* rather than naming bob directly means you manage the circle by editing group membership, which scales as the team grows. Confirming with `policy access` that "everyone (public)" no longer appears for `staging` is the verification habit applied to access: do not assume the restriction took, *check* who the policy now resolves as able to read. Combined with secrets, this is how you build a need-to-know channel encrypted to exactly the right people.

### Exercise 5.7 — Verify the policy chain offline

**Problem:** On a clone with no server running, verify that the entire access policy is authentic. What does a successful verification prove?

**Solution:**
```sh
hv policy verify
  policy chain valid: N version(s)
```

**Explanation:** A successful `policy verify` proves that every version of the policy is signed by an authorized admin and correctly linked to its parent, all the way back to the root — and it proves this *offline*, with no server to ask. This is the concrete payoff of putting authority in a signed chain in the repository rather than in a vendor's database: the rules travel with the repo and their authenticity is independently checkable by anyone holding a clone. It is also the basis for trusting a server you do not control — even a compromised server cannot forge a policy that passes `verify`, so you can detect tampering rather than having to trust the host.

### Exercise 5.8 — Maintain a repo and confirm integrity

**Problem:** Run the maintenance trio on a repo and interpret each output, then explain the right order to run them in.

**Solution:**
```sh
hv gc        # removed K unreachable object(s); M kept
hv repack    # repacked R object(s), reclaimed B bytes; …
hv fsck      # checked … objects, … refs; ok: no corruption detected
```
Order: `gc` (drop unreachable), then `repack` (delta-compress what remains), then `fsck` (verify the result).

**Explanation:** The order matters because each step prepares for the next: `gc` first removes objects you do not want to spend effort packing, `repack` then compresses the survivors, and `fsck` last confirms that all the rewriting left every object hashing correctly and every ref resolving. Running `fsck` at the end is the verification habit one more time — after any operation that rewrites storage, prove the result is intact rather than assume it. On a long-lived shared repo this trio, run periodically, keeps the single `haven.db` file lean and certified consistent, and `fsck`'s `ok: no corruption detected` is the receipt.

---

## Mini-projects

### Mini-project 5.A — A complete two-person onboarding

**Description:** Run the full lifecycle of bringing a second person onto a repository with a shared secret: exchange keys, add the member, grant access, rotate, serve, and have the newcomer clone and decrypt. This is *the* canonical Haven team workflow, end to end.

**Concepts practiced:** `key show` exchange, `member add`, `grant`, `secret rotate`, `serve`, `clone`, the binding rule over the network.

**Requirements:** End with the newcomer reading the shared secret from their own clone, and with `secret rotate` having reported two recipients.

**Walkthrough:** Set up alice's repo with an encrypted `.env`. In a separate `HOME`, generate bob's identity and capture his two public keys with `key show`. Back as alice, add bob as a member with those keys, grant him read on the branches, and rotate — confirming the output now says two recipients. Serve the repo, and as bob (his `HOME`, his key) clone it and `cat` the `.env`. The newcomer reading the plaintext is the proof that access flowed all the way through to decryption.

**Solution:**
```sh
# alice
hv init; hv config user.name alice; hv config user.email a@e; hv key gen
echo "DB_PASSWORD=hunter2-team" > .env; echo code > app.go
hv add . && hv commit -m "app + secret"

# bob (separate HOME) generates identity, shares PUBLIC keys
#   hv key gen ; hv key show   -> <bob-sign> <bob-enc>

# alice onboards bob
hv member add bob <bob-sign> <bob-enc>
hv grant bob read "refs/branches/**"
hv secret rotate
  rotated 1 secret object(s) on main to 2 recipient(s)
hv serve --addr 127.0.0.1:18475 --kind team &

# bob clones with his identity and reads the secret
hv clone http://127.0.0.1:18475 bobrepo
cat bobrepo/.env
  DB_PASSWORD=hunter2-team
```

**Explanation:** This mini-project is the spine of the chapter performed as one continuous story, and every step maps to a guarantee. The key *exchange* moves only public material, so nothing sensitive crosses the wire during onboarding. `member add` plus `grant` makes bob both known and authorized; `secret rotate` then re-encrypts to two recipients, and seeing that count is your confirmation the binding rule fired. The clinching step is bob reading the plaintext from *his own* clone with *his own* private key — proving the server held only ciphertext and that decryption happened locally, exactly as the model promises. Run this once and you can onboard a real teammate with confidence, because you have watched access turn into decryption with your own eyes.

### Mini-project 5.B — Offboard a member safely

**Description:** Reverse the previous project: remove bob's access, rotate so secrets exclude him, and confirm he can no longer decrypt — while reasoning explicitly about what rotation can and cannot undo.

**Concepts practiced:** `revoke`/`member revoke`, `secret rotate`, the limits of rotation, credential rotation at the source.

**Requirements:** Show that after revocation and rotation, a fresh clone by bob yields the placeholder, and state in writing why you would also change the underlying credential.

**Walkthrough:** Starting from the onboarded state (bob a recipient), revoke bob's access and rotate. The rotate should now report one recipient again. Have bob attempt a fresh clone and read the secret — he gets the placeholder, because the current ciphertext is no longer encrypted to him. Then write the crucial caveat: if bob ever actually saw the plaintext password, rotation does not unsee it, so for a real departure you also change the database password at its source.

**Solution:**
```sh
# alice removes bob
hv member revoke bob        # (or: hv revoke <grant-id>)
hv secret rotate
  rotated 1 secret object(s) on main to 1 recipient(s)

# bob tries again with a fresh clone
hv clone http://127.0.0.1:18475 bob2
cat bob2/.env
  <haven: encrypted secret; you are not a recipient>
```
Caveat: also rotate the real credential (issue a new DB password) if bob ever saw the plaintext.

**Explanation:** The recipient count dropping back to one, and bob's fresh clone yielding the placeholder, prove that revocation plus rotation genuinely cuts off *future* decryption — the current ciphertext is re-encrypted to exclude him. But the written caveat is the mature part: rotation controls who can decrypt the *stored* secret going forward; it cannot retract knowledge bob already gained if he previously decrypted and read the value. So a complete offboarding has two halves — Haven's (revoke and rotate, which you just did) and the upstream system's (invalidate the actual credential). Conflating "they can no longer decrypt our copy" with "the secret is safe" is a real-world mistake; this project trains you to do both.

### Mini-project 5.C — Tamper, detect, and recover

**Description:** Play the integrity story end to end: corrupt an object in the database, watch `fsck` catch it, and reason about how a healthy clone lets you recover — tying together content-addressing, `fsck`, and the value of distributed copies.

**Concepts practiced:** content-addressing as a checksum, `fsck` detection and exit status, recovery from a good clone.

**Requirements:** Demonstrate `fsck` detecting a deliberately corrupted object and exiting non-zero, and describe the recovery path.

**Walkthrough:** In a repo with a few commits, run `fsck` to confirm health. Then corrupt one object's stored bytes directly in the database (flipping a byte), so its content no longer matches its hash-address. Run `fsck` again: it reports the specific object as corrupt — stored under one hash but hashing to another — and exits non-zero. Finally, reason about recovery: because objects are content-addressed and the repo is distributed, a healthy clone (or the server) still has the correct object under that hash, so re-fetching or restoring from a good copy repairs the damage.

**Solution:**
```sh
hv fsck
  ok: no corruption detected

# corrupt one blob's stored content in the database (flip a byte):
#   (using any sqlite tool) UPDATE objects SET content=<altered> WHERE hash=<some-blob>;

hv fsck ; echo "exit=$?"
  corrupt blob object: stored as <h> but hashes to <h2>
  hv fsck: 1 problem(s) found
  exit=1
# Recovery: re-fetch the object from the server or a healthy clone, which still
# holds the correct bytes under hash <h> (content-addressing makes it verifiable).
```

**Explanation:** This project makes content-addressing's payoff tangible: because an object's address *is* the hash of its content, any corruption changes the hash and `fsck` catches it by simply recomputing — `stored as <h> but hashes to <h2>` is the smoking gun, and the non-zero exit means automation notices too. The recovery insight is why distributed version control is resilient: the correct object still exists under hash `<h>` in every other copy, so a damaged repo is repaired by fetching the authentic bytes from a healthy peer, with content-addressing guaranteeing you got the *right* bytes back. Integrity is thus not just detected but recoverable, which is exactly the property you want from the store underneath your team's shared history. (The earlier crash-safety discussion is the prevention side of the same coin; this is the detection-and-cure side.)

---

## Summary, and where you have arrived

This final chapter put Haven on the network and gave it a cast. You learned to `serve` a repository, choosing `--kind team` (refuses havens) or `--kind personal` (accepts them, for your own machines), over TLS for any real use. You added remotes and moved data with `push`, `fetch`, `pull`, and `clone`, each respecting the policy. And you met the heart of Haven's access model — the **signed policy**: members in a keyring (`member add`), capabilities granted on ref patterns through a least-privilege verb hierarchy (`grant` read < write < force < grant < admin), groups and `restrict` for need-to-know, and `policy show`/`access`/`log`/`verify` to inspect and *offline-verify* a chain that a server can enforce but never forge. The chapter's climax was the binding rule made visible across the network: grant bob read, `rotate`, watch the recipient count go from one to two, and watch bob clone over the wire and decrypt locally with his own key — access flowing into decryption, no plaintext ever touching the server. You learned how signed requests keep the network honest, how `fsck`/`gc`/`repack` keep a shared repo lean and proven-intact, that writes are crash-safe by tested design, and exactly where the guarantees stop: use TLS, guard private keys, configure server kind, and remember the system is unaudited.

Step back and see the whole book. You began with a mental model — two independent axes, access and secrecy, tied by one rule that recipients equal readers — and the discipline of asking *what does copying the repo give an attacker?* You built `hv`, created an identity that lives outside every repo, and watched a secret become ciphertext under a generic `grep`. You learned the daily loop — stage, commit, branch, merge, undo — until the security faded into the background. You operated the two pillars: havens that structurally refuse to leak, and secrets encrypted to a branch's readers, composing into doubly-protected secret havens. And now you have taken it to a team, where the policy decides who reads and writes and the binding rule turns that decision into decryption automatically.

The single thread through all of it is that Haven replaces *discipline* with *structure*. Where another tool asks you to remember not to push the private branch and not to commit the secret, Haven makes the private branch un-pushable and the secret ciphertext-by-default — guarantees that hold whether or not anyone is paying attention, and that you can verify with tools that have no stake in the answer. That is what "privacy and secrets, built in, not bolted on" means, and you now know not just the slogan but the commands, the model, and the proofs behind it. Go build something you are not ready to show the world yet — Haven will keep it that way until you decide otherwise.
