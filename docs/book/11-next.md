# Chapter 11 — Where to go next

---

## What exists

Let me state the current position precisely, because the rest of this
chapter is about what is *missing* and the difference matters.

**Done:**

- A deterministic WFC solver in Go, with no dependencies outside the
  standard library, and 30 tests covering observation, propagation,
  determinism, weighting and contradiction handling.
- A hex island of 1801 cells per layer, two layers, solved from the
  ground up.
- Click-to-place, constraint removal, and a diff-driven partial rebuild
  that touches one chunk of forty-nine per placement.
- Data-driven, neighbour-aware buildings: walls, windows, arches, towers,
  spires, roofs.
- Deterministic props: trees, shrubs, flowers, lamps, barrels, rocks.
- A packaged application: icon, desktop entry, licence, and a Linux
  `.deb` that builds, validates, and runs.
- `gofmt`, `go vet`, TypeScript, ESLint, Vite and the packaging checks
  all pass.

**Not done**, honestly:

- **No save/load UI.** `SaveTown` and `LoadTown` exist and work; there is
  no button that calls them.
- **No frontend test runner.** The geometry and prop behaviour is
  covered by runtime probes and startup assertions, not committed tests.
- **No Windows or macOS packages.** The metadata and icons are
  configured and structurally checked, but never built end-to-end.
- **No audio.**
- **Roof ridges do not always line up** across adjacent cells.

---

## The four open problems, in order of value

### 1. Save and load

The backend is done. `SaveTown` writes the seed, the constraints and
their order; `LoadTown` restores them and rebuilds the town exactly. The
test `TestSaveAndLoadRoundTrip` proves it.

What is missing is the plumbing: a menu item, a file dialog, and a
status message.

Wails v3 has native dialogs:

```go
path, err := dialog.OpenFile(dialog.OpenFileOptions{
    Title: "Open a town",
    Filters: []dialog.FileFilter{{DisplayName: "ArkPeak town (*.arktown)", Pattern: "*.arktown"}},
})
```

The design question worth thinking about: **should saving be a snapshot or
a recipe?**

ArkPeak currently stores the *constraints* — the player's actual
placements — and rebuilds the town from them. That is the right choice,
and it is worth understanding why.

If the save file stored the *settled grid*, then loading would be exact
and simple. But the grid is 3602 cells of derived state, and it would
bloat the file, and it would freeze any future change to the ruleset. A
town saved with the old rules would load with the old geometry baked in.

Storing the constraints instead means the town is *re-derived*. Change the
ruleset and old towns grow new buildings. That is a feature, not a
limitation — it is the same property that makes the per-cell random
stream worth its cost (Chapter 7).

**The catch**: the rebuild must be deterministic, or a loaded town will
not match the one that was saved. That is exactly what the per-cell
random source guarantees, and it is why that decision was made in the
solver rather than in the save code.

### 2. Frontend tests

The frontend has no committed test runner. The geometry and prop logic
is the part of the codebase most likely to break silently, because
nothing about it throws when it is wrong — it just draws something
subtly off.

The obvious choice is **Vitest**. It shares Vite's configuration, so the
cost of adoption is low.

The tests worth writing, in priority order:

**Geometry purity.** The wall-vertex-count property is a perfect test:

```ts
// A wall draws one face per linked side, and each face is a box of
// 24 vertices. The vertex count must be a strict function of the
// neighbour pattern.
expect(vertexCount(wall, linksWith(1))).toBe(24 * 2);
expect(vertexCount(wall, linksWith(5))).toBe(24 * 6);
```

This catches off-by-one errors in the neighbour loop, which are
invisible in a screenshot and obvious in a number.

**Chunk partition exactness.** The check that exists at startup should
also be a test, so that a failure is a test failure rather than an
application that will not boot.

**Prop distribution.** The density check from Chapter 10, as a test with
a tolerance.

**Determinism.** The same inputs must give the same outputs, across
runs. The frontend's hash functions are the risky part here, and a test
that runs them twice is cheap.

**Upper-storey visibility.** That a storey is drawn only over solid
ground, and only for the cells the rules say.

The reason this was not done earlier is worth recording: the frontend
work happened alongside the Go work, and the Go side had a test suite from
the start. Retrofitting a test runner into a codebase that grew without
one is more work than starting with one. If you are reading this before
writing your own frontend, set up Vitest on day one.

### 3. Windows and macOS packaging

The Linux package is verified end to end: it builds, `nfpm` validates it,
and the extracted binary runs without errors.

The other two platforms are configured but untested, and there is a
specific reason to be careful here.

```json
// build/windows/info.json
{
  "productName": "ArkPeak",
  "companyName": "ArkPeak",
  "fileDescription": "A cozy procedural town-builder",
  "productVersion": "0.1.0",
  "copyright": "Copyright © 2026 ArkPeak",
  "comments": "..."
}
```

These files are structurally correct, but structural correctness is not
the same as a working installer. The things that actually break:

- **Icon format.** Windows wants `.ico` with multiple sizes including
  256×256. macOS wants `.icns`. The generated icons exist, but the
  *conversion* is what breaks, and it only breaks on the target platform.
- **Code signing.** An unsigned Windows binary triggers SmartScreen. An
  unsigned macOS binary will not launch at all on Apple Silicon — it
  needs at least an ad-hoc signature, which the build task handles.
- **The webview runtime.** Windows needs the WebView2 runtime, which is
  usually present but must be handled if it is not.

**The instruction is the same as for any cross-platform work: build it on
the target platform.** A Linux CI runner cannot produce a signed macOS
build.

### 4. Roof ridge continuity

This is the most interesting remaining rendering problem, and it is worth
explaining because it is a genuine *design* question rather than a bug.

A roof ridge is the line where two roof slopes meet. In a real building,
the ridge runs in a straight line along the top of the building. In
ArkPeak, each cell draws its own roof, and each cell decides its own
ridge direction from its connector pattern.

Where two roof cells meet along a ridge, the two ridges should line up.
Sometimes they do; sometimes they do not, because the two cells can
disagree about which way the ridge runs.

There are two ways to fix it.

**Make the ridge direction a rule.** Add a connector like `ridge-ns` and
`ridge-ew` to the roof tiles, and require the two cells' ridge connectors
to match. The solver would then guarantee continuity, because it is
exactly the kind of constraint the solver is good at.

**Make the ridge direction derived.** Compute it from the *neighbour
pattern* — if the roof continues north and south, the ridge runs
north-south. This is the approach the rest of the renderer takes, and it
keeps the rules simple.

The first is more correct and the second is more consistent with the
architecture. Which one you pick is a design decision, and it is a good
exercise, because it forces you to decide whether roof continuity is a
*rule* or a *rendering consequence*.

---

## Some things that would be interesting to try

These are not on the roadmap. They are the ideas that this book has
provoked while writing it, and they are presented as exercises.

### Implement true Shannon entropy

Chapter 4 explained the difference between entropy and candidate count.
Implementing the real thing is a good exercise because it touches the
performance-sensitive path:

```go
// The reference implementation maintains running sums so that the
// entropy of a cell is O(1) to read:
//
//     H = log(sumOfWeights) - sumOfWeightLogWeights / sumOfWeights
```

You would need to add two `[]float64` to the cell struct and update them
in `ban`. Then you could measure whether the towns look better, and
whether the solve time changes.

### Oskar's local-minimum heuristic

The WFC repository says Oskar's island generator "uses a custom
observation heuristic that doesn't produce local minimums". Finding out
what that heuristic is — and implementing it — would be the single most
interesting exercise in this list, because it is the difference between
"a valid town" and "a town someone wanted to draw".

A good first approximation: prefer cells whose *collapse* would affect
the most neighbours. A cell with six possible tiles and many
constrained neighbours is more valuable to settle early than one in open
space.

### A procedural coastline

The island is a perfect hexagon. Townscaper's has a real coastline with
bays and promontories.

This could be done with WFC itself — a second solver run, on a different
layer, deciding which cells are land and which are sea. It is a
beautiful demonstration of the algorithm's generality, and the
constraints are easy: a coastline must be a closed curve, and the
interior must be connected.

### More tile vocabulary

The tile set has eleven tiles. The interesting direction is not more
tiles but *more expressive connectors*:

- A `corner` connector that only matches another `corner` at 120°,
  making curved walls.
- A `stairs` connector that only matches `stairs`, making spiral
  staircases.
- A `bridge` connector that matches only `bridge` and `bridge-rail`.

Each of these creates a new kind of structure that the solver discovers
on its own, with no special-case code anywhere.

### A second observer

Wails events are unused in ArkPeak, on purpose, because there is only one
window. But a second window — a minimap, say — would want to follow the
first. The event system is there for exactly that:

```go
app.Event.Emit("town-updated", diff)
```

and the second window would apply the same diff to its own mirror. Since
the diff is a *value*, not an event-with-payload-semantics, the second
window cannot drift from the first.

### Seed sharing

A town is fully determined by its seed and its constraints. That means a
seed is a *shareable town*. A URL like:

```
arkpeak://town/8a3f2b1c
```

would open the same island with the same buildings. This is the kind of
feature that procedural games can do cheaply and that players love.

---

## Closing thoughts

Three things are worth taking away from this book.

**First: the algorithm is the easy part.** Wave Function Collapse is
understood, well-documented, and fits in 800 lines. What makes a
procedural town *good* is everything around it: the tile vocabulary, the
tightness of the rules, the layering, the renderer's handling of
neighbours, and the stability of re-solves. None of that is in the
algorithm.

**Second: the boundaries are what make it work.** ArkPeak's solver knows
nothing about towns, and its renderer knows nothing about rules. The
rules travel in the snapshot, so the frontend cannot restate them and get
them wrong. Every bug in Chapter 10 was a boundary violation — two things
that should have agreed, disagreeing.

**Third: procedural generation is a design discipline, not a technical
one.** The question is never "can I generate a town?" It is always "what
should the rules say, and what should the player be allowed to do?" Oskar
Stålberg answered that question for Townscaper, and the answer was a
tile vocabulary of about a dozen abstract shapes, and a player who places
constraints rather than commands. ArkPeak is a study of that answer.

If you take one thing from this book, take the habit of asking what the
rules should say before asking how to solve them. The solver will always
be the easy part.

---

## Further reading

These are the sources this book drew on.

**Maxim Gumin, *WaveFunctionCollapse*** —
<https://github.com/mxgmn/WaveFunctionCollapse>

The reference implementation and the canonical description of the
algorithm. The README's "Algorithm", "Tilemap generation" and "Comments"
sections are the best short explanation of WFC in existence. The
`Model.cs` file is the algorithm in 255 lines of readable C#.

**Oskar Stålberg's Tumblr** —
<https://oskarstalberg.tumblr.com>

The archive of his own work, including the pre-Townscaper experiments
quoted in Chapter 2: *Island* (2013), *CityGenerator* (2014),
*Polygonal Planet* (2015) and *Brick Block*.

**Oskar's prototypes** — <http://oskarstalberg.com/game/>

Still hosted: the *Island* demo, the *CityGenerator* demo and the
*Polygonal Planet* demo, all playable in a browser.

**Townscaper** — <https://www.townscapergame.com>

The game itself. Play it before reading Chapter 1 again; it makes
everything else make sense.

**Wails v3 documentation** — <https://v3.wails.io>

The framework. The concepts section covers the service, bindings and
events that Chapter 8 uses.

**Three.js** — <https://threejs.org/docs>

The renderer. The `examples/jsm/utils/BufferGeometryUtils.js` merge
helper is what makes Chapter 9's geometry cache possible.

**Wave Function Collapse, the original paper** — by Alexandre Dinaburg
and Ivan Leibbrandt, 2016. Available through the WFC repository's
research links.

---

*Previous: [Chapter 10 — The bugs worth learning from](10-bugs.md) · Back to the [Preface](00-preface.md)*
