# Chapter 3 — Wave Function Collapse from the ground up

---

## The name, and why it is a lie

"Wave Function Collapse" comes from quantum mechanics, where a particle
can be in a superposition of states until it is *observed*, at which point
it becomes one definite thing.

The algorithm borrows the vocabulary but none of the physics. Maxim Gumin,
who wrote the reference implementation, is upfront about it:

> The coefficients in these superpositions are real numbers, not complex
> numbers, so it doesn't do the actual quantum mechanics, but it was
> inspired by QM.

So: "wave" means *a set of possibilities for one cell*. "Collapse" means
*narrowing that set down to one*. "Observation" means *choosing which cell
to collapse next*.

If you know one idea about quantum mechanics — that observation seems
random — you already know the algorithm's one surprising property: **the
collapse is random**, and the randomness is the point.

## The problem, stated plainly

You have a grid. Each cell must end up holding exactly one tile. Tiles
have rules about what may touch what. You want a grid that satisfies every
rule.

That is it. Everything else is detail.

In ArkPeak's ruleset, the rules are:

```json
{ "a": "wall", "b": "wall", "axis": "hex" },
{ "a": "wall", "b": "window", "axis": "hex" },
{ "a": "roof", "b": "roof", "axis": "hex" },
{ "a": "roof", "b": "edge", "axis": "hex" }
```

Read that as: *a wall may touch a wall. A wall may touch a window. A roof
may touch a roof. A roof may touch an edge.* Anything not listed may not
touch.

Your job: fill a grid so that every touching pair of tiles is allowed.

## Why the naive approach fails

You could try: pick a cell, pick a random tile that is compatible with its
already-placed neighbours, place it, repeat. If you get stuck — no tile
fits — back up and try a different choice.

That works, and it is what most people write first. It is also terrible,
for two reasons:

1. **It is exponentially slow.** Getting stuck near the end, after placing
   most of the grid, means undoing a lot of work. And "getting stuck" is
   common.

2. **It produces ugly results.** Because you place cells in a fixed order,
   you are placing along rows, and the result looks like rows.

WFC fixes both with one idea: **collapse the most constrained cell first.**

## The algorithm

From Gumin's `README.md`, the canonical description:

> WFC initializes output bitmap in a completely unobserved state, where
> each pixel value is in superposition of colors of the input bitmap. Then
> the program goes into the observation-propagation cycle:
>
> * On each observation step an NxN region is chosen among the unobserved
>   which has the lowest Shannon entropy. This region's state then collapses
>   into a definite state according to its coefficients.
> * On each propagation step new information gained from the collapse on the
>   previous step propagates through the output.

Translated into ArkPeak's terms — one cell, not an NxN region, and *tiles*
not colours — the loop is:

```
while there are unsettled cells:
    1. OBSERVE
       Pick the unsettled cell with the fewest remaining candidates.
       Choose one of those candidates at random, weighted by the tile's
       weight. That cell is now settled.

    2. PROPAGATE
       That choice forbids some tiles in neighbouring cells.
       Those cells lose candidates, which may forbid more tiles further
       away. Repeat until nothing changes.

    If any cell ends up with zero candidates, we have a contradiction.
```

Two phases, over and over, until the grid is full.

### The state of a cell

Every cell holds a **set** of candidate tiles — the tiles it might still
be. ArkPeak stores this as a **bitmask** in a `uint64`:

```go
type cell struct {
	// candidates is the set of tiles this cell may still hold, as a
	// bitmask. One bit per tile.
	candidates uint64
	...
}

// popcount returns the number of set bits in v.
func popcount(v uint64) int {
```

With at most 64 tiles, the whole set fits in one machine word. This is not
a micro-optimisation; it makes the algorithm *readable*. "How many options
does this cell have?" is `popcount(c.candidates)`. "Is `wall` still
allowed?" is `c.has(wall)`.

### The two phases in code

**Observation** — pick the most constrained cell, then collapse it:

```go
// SelectCell returns the unsettled cell with the fewest remaining
// candidates.
func (s *Solver) SelectCell() (CellKey, bool) {
	best := CellKey{}
	bestCount := 0
	found := false

	for _, k := range s.order {
		c := s.cells[k]
		if c.settled {
			continue
		}
		n := c.count()
		if n == 0 {
			// A cell with no candidates is a contradiction; report it
			// as the most constrained cell so the caller reports it.
			return k, true
		}
		if !found || n < bestCount {
			best = k
			bestCount = n
			found = true
		}
	}

	return best, found
}
```

**Propagation** — the other half:

```go
// Propagate removes tiles that are incompatible with the settled neighbours of
// every cell in the region.
func (s *Solver) Propagate() error {
	queue := make([]CellKey, 0, len(s.cells))
	...
	// The queue is processed breadth-first. Each settled cell constrains its
	// neighbours; each removal in turn re-constrains that cell's neighbours.
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		...
		for _, n := range s.neighborsOf(key) {
			...
			if s.prune(n, nc, key, c.collapsed) {
				switch nc.count() {
				case 0:
					return fmt.Errorf("%w: cell %+v has no valid tile (forced by neighbour %+v)",
						ErrContradiction, n, key)
				case 1:
					// The cell is forced: settle it now so its own
					// constraints propagate further.
					s.force(n)
					enqueue(n)
				default:
					enqueue(n)
				}
			}
		}
	}
	return nil
}
```

Note the `case 1:` branch. When propagation leaves a cell with exactly one
option, it is **forced** — settled immediately, and queued so its own
constraints propagate onward. This is why a single wall can ripple through
an entire building: the wall settles, its neighbours drop to one option,
those settle, and so on.

## A complete worked example

Three cells in a row. Two tiles: `W` (wall) and `E` (empty). `W` may only
touch `W`. `E` may touch anything.

```
   [ 1 ]  [ 2 ]  [ 3 ]
```

**Start.** Every cell can be `W` or `E`. All three have 2 candidates.

**Observe.** All are equally constrained, so pick the first: cell 1.
Choose at random — say `W`. Cell 1 is settled on `W`.

**Propagate.** Cell 1 is `W`, so cell 2 cannot be `E`. Cell 2 is now
forced to `W`, settles, and propagates to cell 3, which is also forced to
`W`.

```
   [ W ]  [ W ]  [ W ]
```

Done. Three `W`s.

**Now the interesting part.** Suppose the rules also allowed `E` next to
`E`, and the first collapse had picked `E` instead:

```
   [ E ]  [ E ]  [ E ]
```

The randomness of the *first* choice, combined with propagation, produced
a completely different — but equally valid — town.

That is the whole trick. **One random decision, propagated, becomes a
structured result.** And because the *first* decision is the most
constrained one, that random decision is usually made where it has the
most influence — and the influence spreads outward in a shape determined
by the rules.

## Where the randomness comes from

Gumin's reference implementation picks a cell and then:

```csharp
void Observe(int node, Random random)
{
    bool[] w = wave[node];
    for (int t = 0; t < T; t++) distribution[t] = w[t] ? weights[t] : 0.0;
    int r = distribution.Random(random.NextDouble());
    for (int t = 0; t < T; t++) if (w[t] != (t == r)) Ban(node, t);
}
```

Two things to notice:

1. **It is weighted.** The probability of picking a tile is proportional to
   its weight. This is how "more common" tiles become more common.

2. **It is not seeded from a single global stream.** This is ArkPeak's
   most important design decision, and it is explained in Chapter 7.

## The contradiction problem

Sometimes propagation leaves a cell with **zero** candidates. That cell
cannot be anything. The configuration is impossible.

Gumin notes:

> The problem of determining whether a certain bitmap allows other
> nontrivial bitmaps satisfying condition (C1) is NP-hard, so it's
> impossible to create a fast solution that always finishes. In practice,
> however, the algorithm runs into contradictions surprisingly rarely.

There are three ways to handle a contradiction, and ArkPeak uses the third:

1. **Restart.** Throw the whole thing away and try again with a different
   seed. Works, but wasteful and can livelock.

2. **Backtrack.** Undo the last collapse and try a different choice. The
   classic approach, and it is what makes naive implementations slow.

3. **Never get into one.** Design the rules so contradictions are
   impossible, and make the solver's choices such that they cannot arise.

ArkPeak does (3), by a combination of means:

- The ruleset is designed so the ground layer always has a solution
  (every ground tile has `any` connectors, so the ground is always
  satisfiable).
- Layer 1 is only drawn where the ground below it is solid — see Chapter
  9.
- When a player *does* create a contradiction, the constraint is rolled
  back so the town is not poisoned. See Chapter 10.

## The tiled model versus the overlapping model

Gumin describes two variants. ArkPeak uses the first.

**Overlapping model.** You give the algorithm a sample image. It counts
NxN pixel patterns in it and generates new images made of the same
patterns. The "constraints" are implicit in the sample.

**Tiled model (the "simple" one).** You give it a list of tiles and their
adjacency data. The constraints are explicit.

> The simplest nontrivial case of the algorithm is when NxN=1x2 (well,
> NxM). If we simplify it even further by storing not the probabilities of
> pairs of colors but the probabilities of colors themselves, we get what we
> call a "simple tiled model". The propagation phase in this model is just
> adjacency constraint propagation.

ArkPeak is a tiled model with **hex** cells instead of square ones, and
with three axes of adjacency (six hex sides, plus up and down) instead of
two.

The tiled model has a property that matters enormously for a game: **you
can enumerate the rules by hand.** A ruleset is a short list of allowed
pairs. A designer can read it, change it, and predict the effect.

That is exactly what makes Townscaper-like games possible. If the
generator's behaviour were an emergent property of a sample bitmap, the
player could not reason about it at all.

## Constraint propagation is older than WFC

One more piece of context, because it explains why the propagation phase
looks the way it does.

Gumin:

> WFC's propagation phase is very similar to the loopy belief propagation
> algorithm. In fact, I first programmed belief propagation, but then
> switched to constraint propagation with a saved stationary distribution,
> because BP is significantly slower without a massive parallelization (on
> a CPU) and didn't produce significantly better results in my problems.

The propagation phase is essentially the **arc consistency** algorithm
from constraint satisfaction: if tile A at cell 1 rules out tile B at cell
2, and ruling out B at cell 2 rules out tile C at cell 3, then C is
forbidden at cell 3.

This matters because it means the propagation phase is *forced*. There is
no randomness in it at all. Given a settled cell, the consequences are
completely determined. All the randomness lives in the observation phase.

That is a useful mental model:

> **The randomness is a single seed. Everything else is consequence.**

---

## Summary

| Concept | Name | In ArkPeak |
|---|---|---|
| A cell's set of possibilities | the wave | `cell.candidates`, a `uint64` bitmask |
| Narrowing to one | collapse | `Collapse(key)` |
| Choosing which cell | observation | `SelectCell()` |
| Spreading consequences | propagation | `Propagate()` |
| The rules | adjacency | `town_rules.json` connectors |
| A tile that cannot go anywhere | contradiction | `ErrContradiction` |
| Weights | likelihood | `TileDef.Weight` |

---

*Previous: [Chapter 2 — Before WFC](02-before-wfc.md) · Next: [Chapter 4 — Entropy, and why the algorithm feels like drawing](04-entropy.md)*
