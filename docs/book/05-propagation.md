# Chapter 5 — Propagation, and the rules that make a town

---

## The heart of the thing

If the observation phase decides *what a town looks like*, the
propagation phase is what makes it *a town*.

Propagation is deterministic. Given a settled cell, the consequences are
completely forced. There is no randomness in it, no choices, no
strategy. It is the part you can reason about, and it is the part where
the rules you wrote actually do their work.

Let us look at how ArkPeak does it.

## The queue

```go
func (s *Solver) Propagate() error {
	queue := make([]CellKey, 0, len(s.cells))
	inQueue := make(map[CellKey]bool, len(s.cells))

	enqueue := func(k CellKey) {
		if !inQueue[k] {
			queue = append(queue, k)
			inQueue[k] = true
		}
	}

	// Seed the queue with every cell that is already settled, plus any cell
	// that has only one possible tile.
	for _, k := range s.order {
		c := s.cells[k]
		if c.settled {
			enqueue(k)
			continue
		}
		if c.count() == 1 {
			s.force(k)
			enqueue(k)
		}
	}
	...
```

This is a breadth-first sweep. When a cell's candidate set shrinks, that
cell is queued, and when it is processed, it constrains *its* neighbours,
which may shrink *their* candidate sets, queuing them in turn.

The `inQueue` map prevents the same cell being queued twice, which
matters because a cell can be constrained by several neighbours at once.

The doc comment on the function explains why the forced case matters:

> Cells that are left with a single candidate are *forced*: they are settled
> immediately and their own constraints propagated in turn. This is what
> lets a constraint at one end of a building ripple through the whole
> structure, and it is what stops the solver from committing to a cell whose
> only support has already been ruled out.

## Pruning: the actual rule enforcement

The rule enforcement itself is in `prune`:

```go
// prune removes from nc every tile that is incompatible with the settled tile
// of the neighbour at otherKey. It reports whether anything was removed.
func (s *Solver) prune(neighbourKey CellKey, nc *cell, otherKey CellKey, otherTile TileID) bool {
	removed := false

	// Hex adjacency: both cells must be on the same layer.
	if neighbourKey.Layer == otherKey.Layer {
		d, ok := directionBetween(otherKey, neighbourKey)
		if !ok {
			return false
		}
		// neighbourKey lies in direction d from otherKey, so otherKey's
		// edge facing the neighbour is its own d edge, and the
		// neighbour's edge facing otherKey is the opposite of d.
		otherConn := s.rules.EdgeConnector(otherTile, d)
		for t := 0; t < maxTiles; t++ {
			tile := TileID(t)
			if !nc.has(tile) {
				continue
			}
			if s.rules.HexConnectorsMatch(otherConn, s.rules.EdgeConnector(tile, d.Opposite())) {
				continue
			}
			...
			nc.candidates &^= tileMask(tile)
			removed = true
		}
		return removed
	}
```

Three things to notice.

**1. The direction is derived from coordinates.** `directionBetween` works
out which way round the hex the neighbour lies, purely from the two
cells' axial coordinates. The solver never has to be told "this is the
north-east neighbour" — it works it out.

**2. The connectors are looked up by name.** `s.rules.EdgeConnector(tile,
d)` returns the string on that tile's edge in that direction. The solver
compares *strings*. It has no idea what a "wall" is.

**3. `d.Opposite()` is the crux.** Two cells touch if the first's facing
connector matches the second's facing connector, and the second's facing
connector is in the *opposite* direction. Getting this backwards is the
classic WFC bug, and it produces a town where nothing lines up.

## ArkPeak's connector language

Here is the idea that makes ArkPeak's ruleset work, and it is simpler
than it first appears.

Every tile declares a connector on each of its six hex sides, plus one
`up` and one `down`. Two tiles may touch if their facing connectors form
an allowed pair.

```json
{
  "id": "wall",
  "weight": 14,
  "layer": "0",
  "edges": { "n": "wall", "ne": "wall", "se": "wall", "s": "wall", "sw": "wall", "nw": "wall" },
  "up": "wall",
  "down": "any"
}
```

So a wall is `wall` on all six sides. It only touches other walls and
windows. Two walls next to each other form a continuous surface.

And the allowed pairs:

```json
{ "a": "wall", "b": "wall", "axis": "hex" },
{ "a": "wall", "b": "window", "axis": "hex" },
{ "a": "wall", "b": "arch", "axis": "hex" },
{ "a": "window", "b": "arch", "axis": "hex" },
{ "a": "roof", "b": "roof", "axis": "hex" },
{ "a": "roof", "b": "edge", "axis": "hex" },
{ "a": "edge", "b": "edge", "axis": "hex" },

{ "a": "wall", "b": "wall", "axis": "up" },
{ "a": "wall", "b": "tower", "axis": "up" },
{ "a": "wall", "b": "spire", "axis": "up" },
{ "a": "window", "b": "wall", "axis": "up" },
{ "a": "window", "b": "tower", "axis": "up" },
{ "a": "window", "b": "spire", "axis": "up" }
```

### The two special values

```
  any     - matches any other connector (empty plots, floors, gardens)
  nothing - matches nothing at all (a roof cannot support anything)
```

`any` is the wildcard. `empty` has `any` on every side, so it is
compatible with everything — which is exactly right: an empty plot can be
next to anything.

`nothing` is the opposite. `roof` has `up: "nothing"`, meaning **nothing
may rest on a roof**. A roof is the top of a building.

### Two axes, and why that matters

- **`hex`** — sideways adjacency, on the same layer. Symmetric: if `wall`
  may touch `window`, then `window` may touch `wall`.
- **`up`** — vertical adjacency, between a cell and the one above it.
  **Directed**: `wall` supports `tower`, but `tower` does not support
  `wall`.

The vertical axis is what makes a town a town rather than a flat
mosaic. `wall` has `up: "wall"` — a wall supports another wall, so you
can build a tower of walls. And `roof` has `down: "wall"`, so a roof
needs a wall underneath.

### A worked reading of the rules

Take the pair `{ "a": "roof", "b": "edge", "axis": "hex" }`.

`roof` and `edge` are both layer-1 tiles. `roof_edge` declares:

```json
{
  "id": "roof_edge",
  "edges": { "n": "edge", "ne": "roof", "se": "roof", "s": "edge", "sw": "roof", "nw": "roof" }
}
```

So `roof_edge` is `edge` on its north and south sides, and `roof` on the
other four. It is the **border** of a roof: the strip of roof that
overhangs the wall below, with `roof` continuing across the top.

That single distinction — `edge` on two sides, `roof` on four — is enough
for the solver to work out the shape of every roof in the town, with no
rule anywhere saying "roofs have an edge". The shape *emerges* from the
connectors.

## The unified mechanism

Notice what is **not** in the ruleset: no list of "wall can touch wall,
window, arch". No per-tile special cases. No switch statement anywhere in
the solver mentioning a tile by name.

This is the design goal stated in `internal/wfc/ruleset.go`:

> This is a single unified mechanism. There is no separate list of
> "allowed tile pairs" — the connector values carry that meaning, so a
> ruleset author controls adjacency purely by choosing connector names.

Adding a new tile to ArkPeak means:

1. Add a tile definition to `town_rules.json` with its six edges, its up
   and down, and its weight.
2. Add whatever connector pairs it should allow.
3. Nothing else. The solver, the renderer, and the palette all pick it up
   automatically.

You can verify this. The frontend palette is built from
`TownSnapshot.tiles`, which comes from the ruleset. The renderer's
`buildCell` has a `switch` on tile name — but that is a *drawing*
concern, not a *rule* concern, and adding a tile that the switch does not
know simply draws nothing.

## Two questions the rules answer that you might not expect

### Which side is a window on?

```json
{ "id": "window", "edges": { "n": "window", "ne": "wall", ... } }
```

A window's *north* edge is `window`, its other five are `wall`. This is
meaningful: a window only pairs with a window on the north-south axis, so
you get a line of windows running north-south, and walls elsewhere. It is
a small rule that produces a very specific architectural look.

### Which cells can hold a roof?

A roof needs `down: "wall"`. Only `wall` and `window` have `up: "wall"`.
So a roof can only sit on a wall or a window.

Look at what that means for the shape of the town: roofs appear over
buildings and nowhere else. The solver discovers "roofs only go on
buildings" from a single string comparison.

## Why the wildcard is dangerous for drawing

Here is a subtlety that cost ArkPeak a debugging session, and it is worth
understanding because it will bite anyone who builds a similar system.

`empty` has `any` on every side. So `empty` is **compatible** with
everything — the solver will happily put an empty plot next to a wall.

But should the *renderer* draw a wall face towards an empty plot?

No! A wall face next to an empty plot would be a wall with a face
pointing at nothing. It should look like the end of a building, not like a
wall in the middle of a field.

ArkPeak separates the two questions:

```go
// A shared surface needs both sides to declare a real connector. A
// wildcard side is indifferent to its neighbour, so nothing is
// drawn there.
const bothDeclared =
    mine !== CONNECTOR_ANY &&
    theirs !== CONNECTOR_ANY &&
    mine !== CONNECTOR_NOTHING &&
    theirs !== CONNECTOR_NOTHING;

linked.push(bothDeclared && rules.hexConnect(mine, theirs));
```

**`compatible`** is the solver's question: *may these two cells sit next
to each other at all?*

**`linked`** is the renderer's question: *do they form a continuous
surface?*

A wildcard side is compatible with everything but forms a surface with
nothing. This is why `arch` — whose other four sides are `any` — does not
grow a wall towards an empty plot, but still has piers and a lintel.

You can see the difference in the geometry:

```go
// A wall draws a face on each side that meets another wall.
for i := 0; i < 6; i++ {
    if !links.linked[i] {
        continue
    }
    body.push(faceBox(i, width, STOREY, WALL_THICKNESS));
}
```

The word "linked" is doing real work in this codebase. It is the
difference between "the solver permits this" and "the player sees this".

## The vertical axis, and the layer chain

The most important structural decision in ArkPeak is that the town is
solved **from the ground up**.

```go
func (t *Town) SolveAll() error {
	previous := map[wfc.CellKey]wfc.ResultCell{}

	for layer := 0; layer < t.layerCount(); layer++ {
		keys := t.gridKeys(layer)

		solver := wfc.NewSolver(t.rules, t.seed)
		solver.SetRegion(keys)

		// Pin the layer below to what it settled on, so this layer is
		// solved in the context of what supports it.
		solver.SetBelow(tileMap(previous, t.rules))
		...
```

Each layer is solved with the layer below **pinned**. Layer 1 knows what
layer 0 decided, and the vertical connectors constrain what layer 1 may
be.

Without this, a roof could float above an empty plot. With it, the town
is a stack of things that support each other.

### Regional re-solves preserve the chain

When the player places a tile, only a region is re-solved — but the
region is re-solved *on every layer*, ground up, so a change on the ground
correctly affects the roof above.

```go
for layer := 0; layer < t.layerCount(); layer++ {
    keys := t.regionKeysOnLayer(center, radius, layer)
    ...
    // The layer below constrains this layer vertically.
    solver.SetBelow(t.tilesBelowLayer(layer))
```

And the cells *outside* the region are pinned to their current tile, so
they act as fixed boundaries:

```go
// Pin the cells outside the region to their current tile, so the
// region solves consistently with the rest of the town.
for key, cell := range t.cells {
    if key.Layer != layer || solver.InRegion(key) {
        continue
    }
    if tile, ok := t.rules.Lookup(cell.Tile); ok {
        solver.FixCell(key, tile)
    }
}
```

Without that pinning, a local change could ripple across the whole
town. With it, the change is local, and the rest of the town is a fixed
boundary condition.

## The layer that is not drawn

One last piece of the puzzle, which is a *rendering* decision but depends
entirely on the rules.

The solver settles layer 1 for **every** cell in the island — 1801 of
them. But most sit over open ground. Drawing them all would cover the
island in a sheet of roof.

The ruleset already says what should happen, via the `solid` flag:

```json
{ "id": "wall", "solid": true },
{ "id": "roof", "solid": true },
{ "id": "empty", "solid": false },
{ "id": "garden", "solid": false }
```

And the renderer uses it:

```go
// The cell below decides whether this cell is drawn at all.
//
// A storey is only built on top of something solid, so the upper
// storey of a plot is not drawn unless the ground beneath it is a
// building. Without this the whole island would be covered by a
// sheet of roof.
const belowIsSolid = below !== undefined && rules.isSolid(below);

if (key.layer > 0 && !belowIsSolid) {
    return undefined;
}
```

Measured on a real town, that turns 1801 settled upper-storey cells into
**526 drawn ones** — exactly those over a wall or a window. The rest are
skipped, and the island keeps its bare ground and gardens visible.

The rules told the renderer what to draw. The renderer did not have to
know anything about walls or roofs.

---

## Summary

| Concept | Where it lives |
|---|---|
| Connector values | `town_rules.json`, the `edges` map of each tile |
| Allowed pairs | `town_rules.json`, the `connectors` list |
| Direction | Derived from coordinates by `directionBetween` |
| Symmetry | Hex rules are symmetric, up rules are directed |
| Propagation | `Propagate()`, the queue loop |
| Pruning | `prune()`, the rule enforcement |
| Wildcard | `any` — compatible with everything, surface with nothing |
| Layer chain | `SolveAll`, `SetBelow` |

---

*Previous: [Chapter 4 — Entropy](04-entropy.md) · Next: [Chapter 6 — ArkPeak's architecture](06-architecture.md)*
