# ArkPeak — A Companion Book

A long-form guide to ArkPeak, written for someone who is new to both
**Wave Function Collapse** and **Wails**. It also works as an introduction
to *Townscaper* and the ideas behind it.

Every chapter refers to real files in this repository, so you can read
alongside the code.

---

## Contents

### Part I — The original game

| | |
|---|---|
| [0 — Preface](00-preface.md) | What this book is, how the code is laid out, and the short version |
| [1 — Townscaper: the game that is not a game](01-townscaper.md) | What Oskar Stålberg made, and why the algorithm is the whole point |
| [2 — Before Wave Function Collapse](02-before-wfc.md) | *Island* (2013), *CityGenerator* (2014), *Polygonal Planet* (2015) |

### Part II — The algorithm

| | |
|---|---|
| [3 — Wave Function Collapse from the ground up](03-wfc.md) | The quantum-flavored names, the real mechanics, a worked example |
| [4 — Entropy, and why the algorithm feels like drawing](04-entropy.md) | The one heuristic that matters, and the failure mode it has |
| [5 — Propagation, and the rules that make a town](05-propagation.md) | Connectors, the wildcard, and the rules language |

### Part III — Building it

| | |
|---|---|
| [6 — ArkPeak's architecture](06-architecture.md) | Three layers, strict boundaries, and why the rules never appear in TypeScript |
| [7 — Writing the solver in Go](07-solver.md) | Bitmasks, the observation/propagation loop, and determinism |
| [8 — Wails: a Go program with a web page inside](08-wails.md) | The bridge, the generated bindings, and the thread rules |
| [9 — Drawing a town in Three.js](09-rendering.md) | The hex grid, neighbour-aware geometry, chunked rebuilds, props |

### Part IV — Being careful

| | |
|---|---|
| [10 — The bugs worth learning from](10-bugs.md) | Five real bugs, what each taught, and how to spot the same class |
| [11 — Where to go next](11-next.md) | What is not built yet, and the interesting problems waiting in it |

---

## Reading paths

**If you have never written a procedural generator.** Read Chapters 3, 4
and 5 in order. They are self-contained and explain the algorithm without
requiring any Go or JavaScript knowledge.

**If you know WFC but not Wails.** Read Chapters 1 and 2 for the context,
skip to Chapter 6, then read 8 carefully.

**If you know Wails but not WFC.** Read Chapters 3 and 4 first — they are
the ones that matter. Chapter 8 assumes you are comfortable with a
frontend/backend split.

**If you are here to hack on ArkPeak.** Chapter 5 (the rules language),
Chapter 7 (the solver) and Chapter 9 (the renderer) are the three that
map onto code you will change.

**If you are here to learn how the bugs work.** Chapter 10 stands alone.

---

## A note on attribution

Townscaper was made by **Oskar Stålberg**. This project is not
Townscaper and is not endorsed by him; it is a study of the same ideas,
built from scratch as a teaching exercise.

This book quotes and paraphrases several primary sources, and says so
wherever it does:

- **Oskar Stålberg's own Tumblr archive**, including the pre-Townscaper
  prototype posts (Chapter 2).
- **Maxim Gumin's WaveFunctionCollapse repository** — the reference
  implementation and the canonical description of the algorithm (Chapters
  3 and 4).
- **Oskar's still-hosted prototypes** at `oskarstalberg.com/game/`.

Everything describing ArkPeak's own code is verified against the
repository.
