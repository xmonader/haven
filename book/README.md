# The Haven Handbook

*A practical guide to version control where privacy and secrets are built in, not bolted on.*

This is a hands-on book about **using** Haven (`hv`) — not building it. By the end you will be able to keep unfinished work private by construction, ship code whose `.env` is encrypted to exactly the people who can read its branch, and run a small team server that enforces who-can-see-what with a signature it can check but cannot forge.

Haven is a from-scratch VCS. If you know Git, most of the verbs will feel familiar (`init`, `add`, `commit`, `branch`, `merge`, `push`). What is *new* is that two ideas mainstream tools leave to discipline — "don't push the private branch" and "don't commit the secret" — are enforced here by storage and cryptography. This book teaches you to lean on that enforcement instead of remembering rules.

## Who this book is for

You are comfortable on a command line and have used a version control system before. You do not need to know any cryptography — we explain every concept (keys, recipients, signatures) the first time it matters, in plain language. The target reader is a solo developer or a member of a small trusted team who wants private-by-default history and safe secrets without standing up a vault service.

## How to read it

The chapters build on each other; read them in order the first time. Every chapter ends with **exercises** (do these at a real terminal — Haven is small and fast, and muscle memory is the point) and **mini-projects** (realistic end-to-end scenarios). Solutions and explanations are included; resist peeking until you have tried.

Throughout, when you see a command, read the prose around it first. The commands are short; the *why* is the part worth your time.

## Table of contents

| Chapter | What you will learn |
|---|---|
| **[1. Why Haven Thinks Differently](ch01-why-haven.md)** | The two problems mainstream VCS gets wrong, the two-axis mental model (access × secrecy), and the one rule that ties them together. The foundation everything else rests on. |
| **[2. Getting Started](ch02-getting-started.md)** | Build `hv`, create your first repository, generate your identity, make a commit, and learn where Haven keeps things (and where it never does). |
| **[3. The Everyday Workflow](ch03-everyday-workflow.md)** | Staging, committing, inspecting history, branching, three-way merges, and undoing mistakes — the daily loop you will run hundreds of times. |
| **[4. Havens and Secrets](ch04-havens-and-secrets.md)** | Private branches that physically refuse to leak, encrypted secrets tied to branch readers, and rotation after someone joins or leaves. Haven's two signature features. |
| **[5. Working as a Team](ch05-team-and-network.md)** | Serving a repository, pushing and cloning, the signed access policy (members, groups, grants), what travels and what stays home, and keeping a shared repo healthy. |

## Conventions

- Commands you type start with `hv`. Lines beginning with `#` are comments, not commands.
- Output is shown indented under the command when it matters to the lesson.
- `~/.config/haven/identity` is your private key file. It is mentioned often because keeping it off the repo and off the server is the whole game.

A companion quick-reference for every command lives in the project's [`docs/userguide.md`](../docs/userguide.md); this book is the teaching path that gives those commands meaning.

Let's begin.
