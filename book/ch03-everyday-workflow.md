# Chapter 3 — The Everyday Workflow

The security model that drew you to Haven is, paradoxically, something you want to *stop thinking about* most of the time. A tool whose safety features demand constant attention is a tool you will eventually fight. Haven's privacy and secrecy are designed to fade into the background — automatic, structural — so that day to day you can just be a developer: write code, record snapshots, branch to try ideas, merge them back, and undo the inevitable mistakes. This chapter is about that daily loop. It is the most "ordinary" chapter in the book, and that ordinariness is the point.

If you have used a version control system before, much here will rhyme with what you know, and that is deliberate — Haven borrows the history model wholesale rather than reinventing it. But we are not going to assume you know it; we are going to build the loop up from its pieces and explain *why* each piece exists. By the end you will move fluidly between staging, committing, inspecting, branching, merging, and undoing, and you will understand the small but important ways Haven's commands differ from the tools you may have used.

## The loop you will run a thousand times

Every working session in Haven is a cycle: you change files, you decide which changes belong together, you record them as a commit, and occasionally you look back or step sideways onto a branch. Drawn as a picture, the core of it is a flow between three places your files can be:

```
   working tree   --- hv add --->   index (staging)   --- hv commit --->   history
   (files you      (the snapshot you're            (a permanent, hashed
    edit on disk)   assembling for the next         point you can return to)
                    commit)
```

The three boxes are worth naming precisely because the *index* — the middle box — is the part newcomers underestimate. Your **working tree** is just the files in your project directory as you edit them. **History** is the permanent record of commits. Between them sits the **index** (or "staging area"): a deliberate holding pen where you assemble exactly the snapshot you want to commit. The arrows are the two verbs you will type most: `hv add` moves changes from the working tree into the index, and `hv commit` seals the index into history.

Why have a middle box at all? Because "what you changed" and "what you want to record as one logical commit" are often different. You might have edited five files but only two of them belong to the bugfix you are about to commit; the other three are an unrelated experiment. The index lets you stage just the two, commit them as a clean, self-contained change, and deal with the rest separately. This is the difference between a history that reads like a clear story and one that reads like a pile of "misc changes." The staging step is Haven giving you editorial control over your own history.

## Staging and committing in earnest

Let's run the loop on a real file. We will create a small text file, record it, then change it and record the change — watching how `status` reports each state along the way. Start from a fresh repo with your identity already set up (Chapter 2):

```sh
printf 'line one\nline two\nline three\n' > poem.txt
hv add .
hv commit -m "add poem"
```

That is one full turn of the loop: the file went from working tree, into the index via `add`, into history via `commit`. The `-m` flag supplies the commit message inline — a short, present-tense description of *what this change does*, which your future self will thank you for. Now edit the file and ask Haven what it sees before staging:

```sh
printf 'line one\nline TWO changed\nline three\n' > poem.txt
hv diff
  diff modified poem.txt
  --- a/poem.txt
  +++ b/poem.txt
  @@ -1,3 +1,3 @@
   line one
  -line two
  +line TWO changed
   line three
```

The `diff` command, with no arguments, shows the difference between your working tree and the last commit (`HEAD`). Read the output the way you would read any unified diff: lines prefixed with `-` were removed, lines with `+` were added, and unprefixed lines are unchanged context shown so you can locate the change. Here, `line two` became `line TWO changed`, and the surrounding lines are shown for orientation. The `@@ -1,3 +1,3 @@` is a *hunk header* telling you the change touches the region starting at line 1, spanning 3 lines on each side. You will read thousands of these; the grammar is worth knowing cold.

Seeing the change, you decide to keep it. Stage and commit, and you have completed a second turn of the loop:

```sh
hv add poem.txt
hv commit -m "edit line two"
```

Notice we staged a specific file (`poem.txt`) this time rather than `.` (everything). Both are valid: `hv add .` stages all changes in the current directory tree, while naming files stages just those. Reach for the specific form when you want only part of your working changes in this commit — that editorial control again.

## Reading history

A commit is only useful if you can find it again. The `log` command walks backward from your current branch tip, newest first, showing each commit's hash, author, date, and message:

```sh
hv log
  commit b89018c65dc5e377aac90dfee4c3e0f2fee9ea657873ff315f7fcbfaaf9e228f
  Author: Ada <a@e.com>
  Date:   Wed, 24 Jun 2026 12:32:33 EEST

      edit line two

  commit a3adbc13b08786bd5424ca32ea9c806c3c0d6edff7d1914c68789cdf1880b634
  Author: Ada <a@e.com>
  Date:   Wed, 24 Jun 2026 12:32:33 EEST

      add poem
```

Two commits, most recent on top, each identified by a full SHA-256 hash. That hash is how you refer to a commit anywhere a "revision" is expected — though you rarely type the whole thing, since a unique prefix (the first ten or so characters) is enough, and `HEAD` always names the current tip. The author and date come from the `user.name`/`user.email` config you set and the moment of the commit. This is the readable story of your project, and the cleaner your commit messages, the more valuable `log` becomes as documentation you got for free.

You can also diff *between* two points in history, not just against the working tree. `hv diff <rev>` compares your working tree to that revision, and `hv diff <revA> <revB>` compares two commits to each other. This is how you answer "what changed between the release and now?" without leaving the terminal. The unified-diff grammar is identical to what you read above; only the endpoints change.

## Branches: trying things without fear

A branch is a movable name for a line of development. The reason branches matter is psychological as much as technical: they let you try an idea on a separate line, knowing that if it goes nowhere you can abandon it and `main` is untouched. In Haven you create and switch branches through the `branch` command's subverbs:

```sh
hv branch create feature
  created public feature
hv branch list
    feature
  * main
```

Two things to read here. `branch create feature` made a new public branch named `feature` starting from where you are now, and `branch list` shows all branches with an asterisk marking the one you are currently on (`main`). The word "public" in the creation message is a quiet but important detail: ordinary branches sit on the *public* end of the access axis from Chapter 1 — they are the kind of branch that *can* be pushed to a team server. (Their private cousins, havens, get their own command and their own chapter; hold that thought.)

Creating a branch does not move you onto it. To start working there, switch:

```sh
hv branch switch feature
# …edit poem.txt, add a fourth line, then…
hv add poem.txt
hv commit -m "add line four"
```

Now `feature` has a commit that `main` does not. The two branches have diverged: each points at a different tip, sharing the history up to where `feature` was created. This is the normal, healthy state of parallel work — and it sets up the operation that makes branches useful rather than just a way to lose track of things: merging.

A note on namespaces that will matter later: `hv branch list` shows *public branches*, and there is a separate `hv haven list` for private havens. They live in different namespaces deliberately, so a branch named `feature` and a haven named `feature` would be distinct things. For now you only have public branches; just file away that "branch" and "haven" are siblings, not synonyms.

## Three-way merge

Merging combines two diverged lines of work back into one. The naive way to merge would be to compare "your version" and "their version" line by line — but that cannot tell the difference between *you added a line* and *they deleted a line you both started with*. To resolve that ambiguity, Haven (like other serious tools) does a **three-way merge**: it looks at three versions of each file — the common **base** (the shared ancestor where the branches diverged), **ours** (your current branch), and **theirs** (the branch being merged in) — and uses the base to understand what each side actually *changed*.

```
              base (common ancestor)
              "a / b / c"
             /            \
        ours               theirs
   "ZERO / a / b / c"   "a / b / c / FOUR"
   (main prepended ZERO) (feature appended FOUR)
             \            /
              three-way merge
          "ZERO / a / b / c / FOUR"   <- both changes kept, no conflict
```

The diagram shows the case where the two sides changed *different* regions: one prepended a line, the other appended a line. Because the base lets Haven see that these are two non-overlapping edits, it can keep both automatically. Let's do exactly that. With `main` having prepended a line and `feature` having appended one, switch to `main` and merge `feature` in:

```sh
hv merge feature
  merged feature into main (ddf3c56121)

cat poem.txt
  ZERO
  line one
  line TWO changed
  line three
  line four
```

Both edits survived: `main`'s prepended `ZERO` and `feature`'s appended `line four` are both present, with no intervention from you. That is the three-way merge earning its keep — it understood that the two branches touched different parts of the file and combined them cleanly. The `(ddf3c56121)` in the output is the hash of the new *merge commit* that ties the two histories together.

### When edits collide: conflicts

Automatic merging works when the two sides changed different regions. When they changed *the same region in different ways*, no tool can guess which you meant — so Haven stops and asks you, by writing **conflict markers** into the file. Suppose both branches changed the same middle line, one to `FROM_MAIN` and the other to `FROM_B`:

```sh
hv merge b
  merge conflicts in 1 file(s):
    f.txt
  fix conflicts, then 'hv add' and 'hv commit'
```

Haven tells you exactly which files conflicted and what to do next, and it exits with a non-zero status (so a script would notice the failure). Open the conflicted file and you will find both versions, fenced by markers:

```
a
<<<<<<< ours
FROM_MAIN
=======
FROM_B
>>>>>>> theirs
c
```

The markers are a map of the disagreement. Everything between `<<<<<<< ours` and `=======` is *your* branch's version of the contested region; everything between `=======` and `>>>>>>> theirs` is the incoming branch's version. The unconflicted lines (`a` and `c`) are left alone because the sides agreed on them. Your job is to edit this region into the single correct result — keeping one side, the other, or a hand-written combination — and then *delete the three marker lines entirely*. A frequent beginner mistake is resolving the content but leaving a stray `=======` in the file; the markers are not magic, they are literal text, and anything you leave behind ships in your commit.

Once you have edited the file to its resolved form, you finish the merge the same way you finish any change — stage and commit:

```sh
# …edit f.txt to the resolved content, removing all <<<<, ====, >>>> lines…
hv add f.txt
hv commit -m "merge b, keeping main's wording"
```

The resulting commit is the merge commit, recording that these two lines of history joined here with your chosen resolution. Conflicts feel alarming the first time, but they are simply Haven refusing to guess about something it genuinely cannot know — which is exactly the behavior you want from a tool that records permanent history.

## Undoing mistakes

You will make mistakes — wrong edits, premature commits, work started on the wrong branch. Haven gives you a small family of undo tools, and the art is choosing the right one. They differ along one crucial dimension: *do they rewrite history, or add to it?* Rewriting is fine for local work you have not shared, but dangerous for anything others have already seen. Keep that axis in mind as we meet each.

The gentlest tool is `restore`, which brings a file back to how it looked at some revision *without touching your commit history at all*. Use it when you have messed up a file in your working tree and just want it back:

```sh
# poem.txt got mangled; bring it back to its committed state
hv restore --source HEAD poem.txt
  restored 1 file(s) from HEAD
```

`--source HEAD` says "the version as of the last commit"; you could name any revision. Only the named files in your working tree change — history is inviolate. This is your everyday "undo my uncommitted edits to this file" command, and because it changes nothing permanent, it is always safe.

When you want to move a whole branch back to an earlier commit, `reset` is the tool — and `--hard` makes it also overwrite your working tree to match:

```sh
hv reset --hard <rev>
  reset main to c87d849a5e (working tree reset)
```

This is powerful and partly destructive: `--hard` discards working-tree changes and moves the branch pointer, so commits after `<rev>` are no longer on the branch and uncommitted edits are gone. It is the right tool to abandon a local dead-end ("forget the last three commits, I'm starting over"), but precisely *because* it rewrites the branch, you must not `reset` away commits that others have already pulled — you would be erasing shared history out from under them. Reset on private, unshared work; never on what the team has seen.

For undoing a change that has *already been shared*, the safe tool is `revert`, which does not rewrite history at all. Instead it computes the inverse of a commit and applies it as a *new* commit on top:

```sh
hv revert HEAD
  reverted 3d751a49fe
```

After this, the effect of the named commit is undone, but the original commit and the new "undo" commit both remain in history — a faithful, append-only record that says "we did X, then we deliberately un-did X." This is exactly what you want for shared branches: everyone's history stays consistent because nothing was erased, only added. The rule of thumb writes itself: **reset rewrites (private only); revert appends (safe to share).**

Finally, `stash` is for the interruption case: you are mid-edit, not ready to commit, and you need a clean working tree *right now* (to switch branches, say). Stash shelves your uncommitted changes and gives you back a clean tree; later you reapply them:

```sh
hv stash
  saved working changes to stash (9934959d49)
hv status
  On main
  nothing to commit, working tree clean

# …do the urgent thing, then bring your changes back…
hv stash pop
  popped stash 9934959d49
```

Between the `stash` and the `stash pop`, your working tree is clean as if you had committed, but your in-progress changes are safe on the side, not lost and not committed. `stash pop` reapplies them and removes them from the stash. This is the tool for "I need to put this down for a second without losing it" — the developer equivalent of sweeping your half-finished work into a drawer to clear the desk, then taking it back out.

## Choosing the right undo

Because the undo family is where new users hesitate most, here is the whole decision in one place:

```
   situation                                        tool
   ----------------------------------------------   --------------------------
   "I mangled a file, want it as last committed"    hv restore --source HEAD <file>
   "abandon my last few LOCAL commits entirely"     hv reset --hard <rev>   (rewrites!)
   "undo a SHARED commit, keep history honest"      hv revert <rev>         (appends)
   "set work aside to switch tasks, don't commit"   hv stash  /  hv stash pop
   "save a named point to return to later"          hv tag <name>
```

The single question that selects the tool is: *has this been shared, and do I want history to remember the undo?* If nothing is shared and you want the mistake gone, `reset` is fine. If it is shared, or you want an honest record, `revert`. If you are not undoing at all but just clearing your desk, `stash`. And if you simply want to bookmark a commit by a friendly name — a release, a known-good point — `tag` records it: `hv tag v1.0` creates the tag and `hv tag list` shows them. Internalize this table and the anxiety around "did I just lose my work?" largely disappears.

---

## Exercises

Run these in a throwaway repo. History tools are best learned by deliberately making messes and cleaning them up; you cannot truly hurt anything in a practice repo.

### Exercise 3.1 — Stage selectively

**Problem:** You have edited three files but only want two of them in your next commit. How do you commit exactly those two, leaving the third's changes in your working tree?

**Solution:**
```sh
hv add file1.go file2.go      # stage only the two
hv commit -m "the focused change"
hv status                     # file3.go still shows as modified, uncommitted
```

**Explanation:** This is the entire reason the index exists, exercised directly. By naming files to `add` rather than using `.`, you assemble a commit that contains one logical change instead of a grab-bag, and `file3.go`'s edits stay in the working tree for a later, separate commit. A clean history is not an accident; it is the product of this small editorial discipline at staging time. Anyone reading your `log` later — including future-you bisecting a bug — benefits from commits that each do one thing.

### Exercise 3.2 — Read a diff hunk

**Problem:** Given the hunk header `@@ -10,4 +10,5 @@`, describe what region of the file changed and how its size changed.

**Solution:** The change is in the region beginning at line 10. The old version had 4 lines there (`-10,4`); the new version has 5 (`+10,5`). So one net line was added within that region.

**Explanation:** Hunk headers are a compact coordinate system, and reading them fluently saves you from miscounting when you apply or review changes by hand. The two pairs are `start,length` for the old side (after `-`) and the new side (after `+`). When the lengths differ, lines were net added or removed; when they match, it was a pure substitution. You will rely on this when resolving conflicts and when reviewing what a `revert` or `restore` is about to do.

### Exercise 3.3 — Clean three-way merge

**Problem:** Create a base file, branch off, change *different* lines on each branch, and merge. Why does this succeed without conflict?

**Solution:**
```sh
printf 'a\nb\nc\n' > f.txt; hv add .; hv commit -m base
hv branch create side; hv branch switch side
printf 'a\nb\nC-side\n' > f.txt; hv add f.txt; hv commit -m side   # changed line 3
hv branch switch main
printf 'A-main\nb\nc\n' > f.txt; hv add f.txt; hv commit -m main   # changed line 1
hv merge side
  merged side into main (…)
cat f.txt   # -> A-main / b / C-side
```

**Explanation:** The merge succeeds because the two branches edited *non-overlapping* regions (line 1 versus line 3), and the three-way merge uses the common base to recognize that. Each side's change is unambiguous relative to the shared ancestor, so both can be applied without contradiction. This is the common, happy case of merging, and seeing it work cleanly builds the intuition you need to understand *why* the next exercise's overlap cannot be auto-resolved.

### Exercise 3.4 — Force and resolve a conflict

**Problem:** Make two branches change the *same* line differently, merge, and resolve the conflict so the file keeps your branch's wording. What must you remember to delete?

**Solution:**
```sh
# both branches change line 2; on main it's FROM_MAIN, on b it's FROM_B
hv merge b
  merge conflicts in 1 file(s): f.txt
# edit f.txt: keep FROM_MAIN, delete the <<<<<<<, =======, and >>>>>>> lines
hv add f.txt
hv commit -m "merge b, keep main wording"
```
You must delete all three marker lines (`<<<<<<< ours`, `=======`, `>>>>>>> theirs`).

**Explanation:** The conflict arises because both sides changed the same region, so Haven cannot know which you intended and refuses to guess — the correct behavior for a tool recording permanent history. The markers are literal text, not metadata, which is why forgetting to remove a `=======` line leaves garbage in your commit. Resolving means editing the contested region to its final form *and* removing the fences, then staging and committing as normal. Doing this once removes the fear; conflicts are just Haven asking you a question it genuinely cannot answer itself.

### Exercise 3.5 — Restore versus reset

**Problem:** You mangled one file but your commits are fine. Which undo tool do you use, and why *not* the other one?

**Solution:** Use `hv restore --source HEAD <file>`. Do not use `hv reset --hard`, because reset moves the whole branch pointer and resets the *entire* working tree — far more than you want, and it would discard unrelated good work.

**Explanation:** The instinct to reach for the biggest hammer is exactly what gets people into trouble. `restore` is surgical: it touches only the files you name and leaves history and other files alone, which is precisely right when the problem is one mangled file. `reset --hard` is a branch-level operation that rewrites where the branch points and flattens the working tree to match — appropriate for abandoning whole lines of work, catastrophic for "oops, one file." Matching the scope of the tool to the scope of the mistake is the core skill of safe undoing.

### Exercise 3.6 — Revert a shared commit

**Problem:** A commit that teammates have already pulled introduced a bug. Why is `revert` the correct tool rather than `reset`, and what will `log` look like afterward?

**Solution:** Use `hv revert <bad-rev>`. Afterward `log` shows *both* the original buggy commit and a new commit that undoes it — history grew, nothing was erased. `reset` would be wrong because it rewrites the branch, erasing the commit your teammates already have and desynchronizing everyone.

**Explanation:** The deciding factor is that the commit is *shared*. Rewriting shared history is one of the few genuinely painful mistakes in version control: other people's repositories still contain the commit you erased, and reconciling that is miserable. `revert` sidesteps the whole problem by being append-only — it adds an "inverse" commit, leaving the timeline consistent for everyone and producing an honest record that the change was made and then deliberately undone. "Append, don't rewrite" is the golden rule for anything others can see.

### Exercise 3.7 — Stash across a branch switch

**Problem:** You are mid-edit on `main` when you must urgently fix something on `feature`. You are not ready to commit your `main` work. What sequence keeps your work safe?

**Solution:**
```sh
hv stash                 # shelve main's uncommitted work; tree is now clean
hv branch switch feature # safe, because the tree is clean
# …make and commit the urgent fix…
hv branch switch main
hv stash pop             # bring your in-progress main work back
```

**Explanation:** You cannot cleanly switch branches with a dirty working tree of unrelated changes, and you do not want to make a junk commit just to move. `stash` resolves the bind by setting your changes aside and giving you a clean tree, which makes the switch safe; `stash pop` later restores them exactly. This is the canonical "interruption" workflow, and it teaches the broader lesson that not every "I need a clean tree" situation calls for a commit — sometimes you just need a drawer to sweep things into temporarily.

### Exercise 3.8 — Tag a known-good point

**Problem:** You have a commit you want to find easily later (a release). Create a friendly name for it and list your tags.

**Solution:**
```sh
hv tag v1.0
  tagged v1.0 -> ddf3c56121
hv tag list
  v1.0
```

**Explanation:** A tag is a stable, human-meaningful name pinned to a specific commit, so you never have to remember that `v1.0` was hash `ddf3c5…`. Unlike a branch, a tag is not expected to move — it marks a moment (a release, a demo, a known-good baseline) you may want to return to or diff against. Tags cost nothing and make `log`, `diff`, and `reset` far more pleasant to use, since you can say `hv diff v1.0` instead of copying hashes around. Naming the points that matter is a small habit with outsized payoff on a long-lived project.

---

## Mini-projects

### Mini-project 3.A — A clean feature-branch lifecycle

**Description:** Take a feature from idea to merged, the way you would on a real project: branch, make a couple of focused commits, keep `main` moving in parallel, then merge cleanly. The goal is to feel the full rhythm rather than isolated commands.

**Concepts practiced:** branching, selective staging, parallel development, three-way merge.

**Requirements:** End with `main` containing both its own independent commit and the feature's commits, merged without conflict, and a `log` that tells a coherent story.

**Walkthrough:** Start on `main` with a committed baseline. Create and switch to a `feature` branch and make two small, focused commits there — focused because each should do one thing, per Exercise 3.1's lesson. Switch back to `main` and make an *unrelated* commit that touches a different file, so the branches genuinely diverge on non-overlapping work. Finally merge `feature` into `main`; because the changes do not overlap, it merges cleanly, and `log` now shows main's commit, the feature's commits, and the merge commit joining them.

**Solution:**
```sh
# baseline on main
printf 'v1\n' > app.txt; hv add .; hv commit -m "app v1"

# feature work
hv branch create feature && hv branch switch feature
printf 'helper\n' > helper.txt; hv add helper.txt; hv commit -m "add helper"
printf 'helper\nmore\n' > helper.txt; hv add helper.txt; hv commit -m "extend helper"

# parallel, unrelated work on main
hv branch switch main
printf 'v1\nREADME\n' > app.txt; hv add app.txt; hv commit -m "document app"

# merge — clean, because feature only touched helper.txt
hv merge feature
  merged feature into main (…)
hv log    # shows: document app, extend helper, add helper, app v1, + merge
```

**Explanation:** This is the everyday loop at full scale, and the reason it merges cleanly is the same as Exercise 3.3: the feature touched only `helper.txt` while main touched only `app.txt`, so the three-way merge sees no overlap. The deeper lesson is workflow shape — keeping a feature's work on its own branch means `main` stays releasable and the feature can be reviewed or abandoned as a unit. When you read the resulting `log`, notice it tells a story: a baseline, a documented main, a feature built in two steps, and the moment they joined. That legibility is the dividend of branching plus focused commits, and it is what makes a project navigable months later.

### Mini-project 3.B — Deliberately break and recover

**Description:** Practice the full undo family on purpose: mangle a file and `restore` it, make a bad local commit and `reset` it away, then simulate a shared bad commit and `revert` it. The point is to build reflexes for the moment something goes wrong for real.

**Concepts practiced:** `restore`, `reset --hard`, `revert`, and the shared-vs-private distinction.

**Requirements:** Demonstrate each recovery and articulate, for each, why that tool and not another.

**Walkthrough:** Begin with a couple of good commits. First, scribble nonsense into a tracked file and `restore --source HEAD` it — history untouched, file fixed. Second, make a genuinely bad commit (say it deletes something important), confirm it is local and unshared, and `reset --hard` to the commit before it, erasing the mistake. Third, to rehearse the shared case, make another bad commit but this time *pretend* it has been pushed, and use `revert` so the fix is an append rather than a rewrite. Compare the `log` after the reset (mistake gone) versus after the revert (mistake and its undo both present).

**Solution:**
```sh
printf 'good\n' > f.txt; hv add .; hv commit -m good1
printf 'also good\n' >> f.txt; hv add f.txt; hv commit -m good2
GOOD2=$(hv log | grep ^commit | head -1 | awk '{print $2}')

# 1) mangled working file -> restore (no history change)
printf 'GARBAGE\n' > f.txt
hv restore --source HEAD f.txt        # f.txt back to good2 content

# 2) bad LOCAL commit -> reset --hard (rewrite, since unshared)
printf '' > f.txt; hv add f.txt; hv commit -m "oops emptied file"
hv reset --hard "$GOOD2"              # the bad commit is gone from history

# 3) bad SHARED commit -> revert (append, keep history honest)
printf 'bad\n' >> f.txt; hv add f.txt; hv commit -m "bad change"
BAD=$(hv log | grep ^commit | head -1 | awk '{print $2}')
hv revert "$BAD"                      # adds an inverse commit; both remain in log
```

**Explanation:** Running all three back to back makes the distinctions visceral. After step 2's `reset`, the bad commit is *absent* from `log` — appropriate only because it was never shared. After step 3's `revert`, both the bad commit and its inverse are *present* — appropriate because we imagined it shared, and erasing shared history desynchronizes everyone who has it. The `restore` in step 1 sits outside the history question entirely: it is a working-tree fix that touches no commits at all. The reflex to build is the triage from this chapter's decision table: ask "is it shared? do I want a record?" and the tool selects itself. Practicing on a disposable repo means that when the real "oh no" moment arrives, your hands already know what to do.

### Mini-project 3.C — A conflict you resolve three ways

**Description:** Construct a single conflict and resolve it three different ways — keep ours, keep theirs, and hand-merge both — committing each as a separate experiment, to internalize that conflict resolution is *your editorial decision*, not a mechanical one.

**Concepts practiced:** conflict markers, deliberate resolution, the meaning of "ours" and "theirs."

**Requirements:** Produce three resolved versions of the same conflicting region and explain when each resolution would be the right real-world choice.

**Walkthrough:** Set up two branches that change the same line — `main` to `FROM_MAIN`, `b` to `FROM_B` — and merge to produce the conflict. Rather than resolving once, do it three times in scratch copies of the file: once keeping only the `ours` text, once keeping only the `theirs` text, once writing a line that combines both intents. For each, remember to strip the three marker lines. Reflect on which choice fits which real situation: keep-ours when your branch's change supersedes, keep-theirs when the incoming change does, hand-merge when both carry information that must survive.

**Solution:**
```sh
# produce the conflict (both changed line 2)
hv merge b
  merge conflicts in 1 file(s): f.txt
# the contested region in f.txt:
#   <<<<<<< ours
#   FROM_MAIN
#   =======
#   FROM_B
#   >>>>>>> theirs

# resolution A — keep ours:        f.txt line 2 = "FROM_MAIN"
# resolution B — keep theirs:      f.txt line 2 = "FROM_B"
# resolution C — hand-merge both:  f.txt line 2 = "FROM_MAIN and FROM_B"
# (in each case delete the <<<<, ====, >>>> lines, then:)
hv add f.txt && hv commit -m "resolve: <which strategy>"
```

**Explanation:** The exercise drives home that the markers do not tell you the answer — they tell you the *question*, and the answer is a judgment only you can make with knowledge of what the two changes meant. Keep-ours is right when your side's edit intentionally replaces the other (you rewrote a function the other branch merely tweaked); keep-theirs when the incoming work is the authoritative version; hand-merge when both edits add real information (two people added different valid entries to a list). Because the resolution is editorial, the same conflict can correctly resolve three different ways depending on intent — which is exactly why a tool must never auto-resolve an overlap and silently pick one. Understanding "ours" as your current branch and "theirs" as the incoming one also prevents the common error of keeping the wrong side under time pressure.

---

## Summary

This chapter put the security model on the shelf and taught the loop you actually live in. You learned the three-box model — working tree, index, history — and why the index's editorial step lets you craft commits that each tell one clear story via selective `hv add`. You read changes with `hv diff` and its unified-diff grammar, walked the past with `hv log`, and used branches (`branch create`/`switch`/`list`) to develop ideas in parallel without endangering `main`. You saw the three-way merge combine non-overlapping edits automatically by reasoning from a common base, and you learned that overlapping edits produce conflict markers — a question, not a failure — that you resolve by editing the contested region and deleting the fences before committing.

Most valuably, you built a triage instinct for undoing mistakes: `restore` for a mangled file (history untouched), `reset --hard` to abandon *local* commits (rewrites history — never on shared work), `revert` to undo *shared* commits honestly (appends an inverse), and `stash` to clear your desk without committing. The one question that selects the tool — *is it shared, and do I want a record of the undo?* — is worth more than memorizing the commands, because it generalizes to every version-control decision you will ever make.

You can now work in Haven the way you work in any capable VCS. That fluency is the platform for the next chapter, where the features unique to Haven take center stage: private havens that physically refuse to leak, and encrypted secrets whose readers you manage by managing access. Everything you just learned — branching, committing, the working-tree loop — still applies; Chapter 4 simply adds the privacy and secrecy dials back on top of a workflow you now own.
