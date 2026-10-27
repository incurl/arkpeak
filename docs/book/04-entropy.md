# Chapter 4 — Entropy, and why the algorithm feels like drawing

---

## The one heuristic that matters

There is exactly one interesting decision in WFC: **which cell do you
collapse next?**

Almost everything else is forced. If cell 1 is a wall, and walls may only
touch walls, then cell 2 cannot be anything but a wall. There is no choice
there, no judgement, no strategy. It is arithmetic.

But *which* cell to collapse first is a genuine choice, and it is where
the character of the output comes from.

Gumin's reference implementation offers the choice like this:

```csharp
double entropy = heuristic == Heuristic.Entropy ? entropies[i] : remainingValues;
if (remainingValues > 1 && entropy <= min)
```

Two heuristics: **Shannon entropy**, or **raw candidate count**.

ArkPeak uses the second one:

```go
n := c.count()
if !found || n < bestCount {
```

The choice between them is subtle, so let me spend a moment on it.

## Shannon entropy, briefly

For a cell that could be one of several tiles with probabilities
*p₁, p₂, …, pₙ*, the Shannon entropy is:

```
H = -Σ pᵢ log₂ pᵢ
```

It measures *uncertainty*. A cell that could be `wall` or `window`, each
equally likely, has entropy 1. A cell that could be `wall` with
probability 0.99 and `window` with probability 0.01 has entropy about
0.08 — it is *nearly decided*, and the algorithm should treat it as
nearly decided.

That is the appeal of entropy over raw count. A cell with 3 candidates
that are all likely is *more* constrained than a cell with 2 candidates
that are all unlikely.

Gumin's implementation maintains entropy efficiently by keeping running
sums:

```csharp
weightLogWeights[i] += weightLogWeights[t];   // on ban
sumsOfWeights[i] -= weights[t];
sumsOfWeightLogWeights[i] -= weightLogWeights[t];
```

and computing entropy from the difference. It is not free.

## Why ArkPeak uses candidate count

ArkPeak's candidates are a bitmask, so the count is a single `popcount`:

```go
// popcount returns the number of set bits in v.
func popcount(v uint64) int {
```

That is one instruction on any modern CPU. Maintaining entropy the way
Gumin does means two running sums per cell and a `log` per observation.

For ArkPeak's weight range — `spire` at 2 up to `empty` at 30 — the two
heuristics agree closely. The difference shows up only when candidates
have wildly different weights, where entropy would prefer a cell whose
unlikely option is very unlikely.

**This is a deliberate simplification and worth knowing about.** If you
are porting ArkPeak and want it to look more like a hand-drawn town, this
is one of the two places to look.

## The real insight: why entropy at all

Here is the thing Gumin says that explains the entire feel of the
algorithm:

> Why the minimal entropy heuristic? I noticed that when humans draw
> something they often follow the minimal entropy heuristic themselves.
> That's why the algorithm is so enjoyable to watch.

Read that again. **Humans draw things in this order too.**

If you are drawing a city, do you start with an empty field and add
buildings one at a time in a grid order? No. You start with the thing
that is most obviously determined — the corner of the harbour, the main
square, the bridge — and work outward.

That is precisely the "most constrained first" heuristic. The human
artist and the algorithm have the same instinct, which is why the
algorithm's output feels like something drawn rather than something
generated.

### A demonstration you can run

Take a sheet of paper and draw a small town, but obey this rule: **always
draw the next most-obviously-determined piece.**

You will find that the rule is easy to follow, and that it changes the
drawing. Starting with a building in the middle of a field forces you to
think about all its neighbours at once. Starting with the building in the
corner where two roads meet forces almost nothing — the corner is
already determined by the roads and the sea.

The rule is not a constraint on the drawing. It is a *strategy* for it.

## Local minima: the failure mode

Now the hard part. Minimising entropy is a good heuristic for *finding a
solution*. It is not a heuristic for *finding a good solution*.

Gumin's "easy tilesets" observation is the key case:

> Note that the unrestrained knot tileset (with all 5 tiles being allowed)
> is not interesting for WFC, because you can't run into a situation where
> you can't place a tile. We call tilesets with this property "easy".
> Without special heuristics easy tilesets don't produce interesting global
> arrangements, because correlations of tiles in easy tilesets quickly fall
> off with a distance.

Read that carefully. If the rules are too *loose*, the algorithm
technically works perfectly — no contradictions, ever — and produces
**boring, uncorrelated output**. Tiles near each other are related, but
tiles far apart are unrelated, and you can see the seams.

This is the fundamental tension:

- **Tight rules** produce coherent, interesting towns — and sometimes
  contradiction.
- **Loose rules** never contradict — and produce visual mush.

And it is exactly the problem Oskar ran into. Recall the WFC repository's
note about his island generator:

> uses a custom observation heuristic that doesn't produce local minimums

Oskar needed a *custom observation heuristic* — a different rule for
choosing which cell to collapse — because the standard one produced
local minima on his island.

### What a local minimum actually is

In the optimisation sense, a local minimum is a configuration where no
single small change improves things, but a larger change would. In the
visual sense, it is a town where the generator has painted itself into a
corner aesthetically: the rules are satisfied, but the result looks
mechanical.

The standard heuristic optimises for **fewest choices**. It does not
optimise for **looks**. Those are different objectives, and the second
one is not expressible in the constraints.

### How you fight it

Options, roughly in order of how much they help:

1. **Tighten the rules.** More constraints means more correlation between
   distant cells. ArkPeak does this heavily: walls only touch walls,
   roofs only touch roofs or edges, and the vertical axis is constrained
   too.

2. **Change the observation heuristic.** Instead of "fewest candidates",
   use something that considers *spatial* properties — Gumin mentions
   "height, density, curvature" as heuristics used in the 3D voxel model.
   A cell in a dense area might be preferred over a cell in an empty one.

3. **Add constraints the solver knows about but the rules do not express.**
   This is ArkPeak's biggest lever, and it comes from the *ruleset* being
   sent to the frontend.

## ArkPeak's local-minimum countermeasures

ArkPeak fights local minima in three ways, all of them consequences of
decisions made earlier in the book.

### 1. The rules are tight

`town_rules.json` is not permissive. Compare ArkPeak's `empty` tile:

```json
{
  "id": "empty",
  "weight": 30,
  "layer": "0",
  "edges": { "n": "any", "ne": "any", "se": "any", "s": "any", "sw": "any", "nw": "any" },
  "up": "nothing",
  "down": "any"
}
```

The ground layer is deliberately permissive — `empty` and `floor` and
`garden` all have `any` on every side, so the ground always has a
solution. But the *building* tiles are tight: `wall` has `wall` on all six
sides, `window` has `window` on the north and `wall` elsewhere.

This is what creates correlation. Two `wall`s adjacent to each other form
a run; a run of walls is a building; a building is a *thing*. The
tightness of the building rules is what makes the ground-level output
look like a town rather than noise.

### 2. The layers are chained

Layer 1 is solved against layer 0. From `internal/town/town.go`:

```go
// SolveAll solves the whole town from its current constraints.
//
// The town is solved layer by layer, from the ground up. Each layer is
// solved against the layer below it: a cell on layer 1 can only hold a tile
// that its counterpart on layer 0 is able to support.
```

This is a *huge* correlation lever. The roofs are not independent of the
walls — they are derived from them. A wall gets a roof above it. A garden
gets nothing. That correlation is what makes the town read as buildings
rather than as two unrelated layers of noise.

### 3. The renderer draws structure, not just colour

This is the subtlest one, and it is a rendering decision rather than a
solving one.

A wall that only draws the faces it shares with another wall is *literally
a different shape* depending on its neighbours. ArkPeak's `buildWall`:

```go
for i := 0; i < 6; i++ {
    if !links.linked[i] {
        continue
    }
    body.push(faceBox(i, width, STOREY, WALL_THICKNESS));
}
```

So a wall in the middle of a run draws two faces (a straight segment), a
corner wall draws two faces at an angle, and an isolated wall draws
nothing but a solid block. The *silhouette* of the building therefore
encodes the constraint graph.

This is why ArkPeak can look good with flat coloured prisms on the ground
layer and still not look like a grid of hexagons.

## Ties, and why they matter

When two cells have the same candidate count, which do you pick? It
matters: different orders give different towns.

ArkPeak breaks ties with a **stable cell order**:

```go
// Keep a stable order so that equal-entropy ties always break the same
// way for a given seed.
sort.Slice(s.order, func(i, j int) bool {
    if s.order[i].Layer != s.order[j].Layer {
        return s.order[i].Layer < s.order[j].Layer
    }
    if s.order[i].Q != s.order[j].Q {
        return s.order[i].Q < s.order[j].Q
    }
    return s.order[i].R < s.order[j].R
})
```

Gumin's reference implementation adds a tiny random nudge to break ties:

```csharp
double noise = 1E-6 * random.NextDouble();
if (entropy + noise < min)
```

That is a nicer trick: it keeps the order *random* but *stable*, so ties
do not bias the result systematically toward one corner. ArkPeak's
version is deterministic, which is better for reproducibility but does
bias toward the top-left corner in ties.

**If you want ArkPeak's towns to look less regular, this is the second
knob to turn**, after the observation heuristic. A tiny random nudge,
exactly as Gumin does it, would spread the ties out.

## A thought experiment

You are painting a town. You place a wall tile. The solver has to decide:
does this wall become part of a building that extends left, right, or
both?

Under the minimal-entropy heuristic, it looks at all six neighbours of
your cell. If any of them is now heavily constrained (because your wall
ruled something out), it deals with that one first. Otherwise it deals
with whichever neighbour happens to be next in the stable order.

The result is that your wall propagates *outward in all directions
simultaneously*, and the town grows like a crystal rather than like a
wave. This is why Townscaper feels responsive: the change is immediate
and it is *local*, spreading only as far as the rules demand.

---

## Summary

| Idea | What it does |
|---|---|
| Minimal entropy | Pick the most constrained cell first — the human's own instinct |
| Candidate count | ArkPeak's cheap approximation of entropy |
| Local minimum | A valid but ugly result; the rules are satisfied but the town is mechanical |
| Tight rules | Correlation between distant cells; interesting output |
| Chained layers | Correlation between layers; buildings read as buildings |
| Structure in the geometry | The silhouette encodes the constraint graph |

---

*Previous: [Chapter 3 — WFC from the ground up](03-wfc.md) · Next: [Chapter 5 — Propagation, and the rules that make a town](05-propagation.md)*
