# Chapter 6 — ArkPeak's architecture

---

## The shape of the thing

ArkPeak has three layers, and the boundaries between them are strict:

```
┌─────────────────────────────────────────────────────────┐
│  frontend/src/            TypeScript, runs in a browser   │
│                                                         │
│    render/     Three.js scene, lights, water            │
│    game/       state mirror, camera, input              │
│    ui/         palette, HUD                             │
│    geometry/   grid maths, neighbour rules, buildings   │
└────────────────────────────┬────────────────────────────┘
                             │  Wails bindings (generated)
┌────────────────────────────┴────────────────────────────┐
│  internal/town/           Go, owns the truth            │
│                                                         │
│    town.go      authoritative state, re-solves, diffs   │
│    service.go   the API the frontend calls              │
│    types.go     the shapes that cross the boundary      │
│    rules/       town_rules.json — the tile rules        │
└────────────────────────────┬────────────────────────────┘
                             │
┌────────────────────────────┴────────────────────────────┐
│  internal/wfc/            Go, pure, no dependencies     │
│                                                         │
│    solver.go    observe / propagate / collapse           │
│    ruleset.go   compile the JSON into lookup tables      │
│    grid.go      hex coordinates and regions              │
└─────────────────────────────────────────────────────────┘
```

The rule that governs everything:

> **The solver never knows about rendering, and the renderer never
> decides what a cell contains.** The backend owns the truth; the frontend
> draws it and asks for changes.

## Why three layers and not two

The obvious question: why is `internal/wfc` separate from
`internal/town`?

Because they have completely different reasons to change.

`internal/wfc` is a **solver**. It knows about cells, tiles, connectors
and entropy. It knows nothing about towns, players, colours, layers or
graphics. It has no idea a "wall" is a thing you look at.

`internal/town` is a **domain**. It knows about layers, the player's
constraints, save files and diffs. It *uses* the solver.

If you put them together, then adding a save-file format would put you
in the same file as the entropy heuristic, and every change would risk
disturbing the algorithm. The separation means the solver's tests can be
written against a hand-made ruleset with five tiles, and they are fast:

```go
// testRuleset is a small, explicit ruleset used by the solver tests.
func testRuleset(t *testing.T) *Ruleset {
```

That test ruleset has five tiles and no layers and no colours. It is
readable in one screen. The real ruleset has eleven tiles, two layers,
six props and colours. The solver tests would be unreadable if they had
to use it.

## What the solver does not know

Let me make the boundary concrete by listing things the solver has no
concept of:

- **Layers.** Not really — the solver has a `layer` field on a cell, but
  it does not know that layer 1 is "above" layer 0 in any meaningful
  sense. It knows a cell has a layer index, and it knows the vertical
  connectors.

  Actually, it knows a bit more than that. `SetBelow` takes a map of
  what is underneath, and `applyBelowConstraints` uses it. But the
  *meaning* of "underneath" comes from the town, not the solver.

- **Colours.** The solver has `Constraint.Color`, but it only carries the
  colour through. It never makes a decision based on it.

- **The player.** There is no concept of a player. There are
  *constraints*, and the town layer decides where they come from.

- **Graphics.** Obviously.

- **Save files.** Obviously.

## What the town layer adds

`internal/town/town.go` is where the solver becomes a game.

### The town is not a grid, it is a region

The solver can solve any set of cells you give it. The town decides
*which* cells that is, and it is not a rectangle:

```go
// HexRegion returns every cell within radius steps of the origin, forming a
// hexagon centred on it.
//
// The town grid is hexagonal rather than rectangular: a rectangle in axial
// coordinates is a rhombus on screen, which reads as a skewed slab rather
// than an island. A hexagon has the shape players expect, and its edges
// give the solver a natural boundary.
func HexRegion(radius int) []CellKey {
```

A radius-24 island has `3·24² + 3·24 + 1 = 1801` cells per layer.

### The town has layers, and they are chained

```go
// SolveAll solves the whole town from its current constraints.
//
// The town is solved layer by layer, from the ground up. Each layer is
// solved against the layer below it: a cell on layer 1 can only hold a tile
// that its counterpart on layer 0 is able to support. Solving the layers
// separately would let a roof float above an empty plot, so the layers are
// chained.
```

### The town has a player, and the player's edits are constraints

```go
// PlaceConstraint places a player constraint and re-solves the affected
// region. It returns the cells that changed.
func (t *Town) PlaceConstraint(key wfc.CellKey, tile, color string) (GridDiff, error) {
```

And the crucial design decision, which Chapter 4 discussed: a placement
is a **constraint**, not a command. The solver decides what actually goes
there.

## The diff: how changes travel

When something changes, the backend does not send the whole town. It sends
a **diff**:

```go
// GridDiff describes exactly what changed in the town.
//
// The frontend applies a diff to rebuild only the affected render chunks
// rather than redrawing the whole town.
type GridDiff struct {
	// Settled lists cells that were added or changed.
	Settled []Cell `json:"settled"`
	// Removed lists cells that were cleared.
	Removed []Cell `json:"removed"`
	// Seed is the town's seed, included so the frontend can correlate
	// diffs with the town they belong to.
	Seed int64 `json:"seed"`
}
```

The diff is computed by comparing before and after:

```go
// diff compares the town's cells with a previous snapshot and reports the
// changes.
func (t *Town) diff(before map[wfc.CellKey]Cell) GridDiff {
```

Note the name: `GridDiff`. It is a *diff of a grid*, not an event, not a
message, not a stream. It is a value. That matters, and Chapter 8
explains why.

## The service: the actual API

`internal/town/service.go` exposes seven methods. This is the complete
surface between the two halves of the application:

```go
func (s *Service) NewTown(seed int64) (TownSnapshot, error)
func (s *Service) GetSnapshot() (TownSnapshot, error)
func (s *Service) PlaceConstraint(q, r, layer int, tileType string, color string) (GridDiff, error)
func (s *Service) CanPlace(q, r, layer int, tileType string) (bool, string)
func (s *Service) RemoveConstraint(q, r, layer int) (GridDiff, error)
func (s *Service) SolveRegion(q, r, radius int) (GridDiff, error)
func (s *Service) SaveTown(path string) error
func (s *Service) LoadTown(path string) error
```

Two of these are worth a note.

### `NewTown` returns a snapshot, and it takes a seed

```go
// NewTown replaces the current town with a fresh one for the given seed and
// solves it.
```

The frontend calls it on launch with a random seed:

```ts
const fresh = await TownService.NewTown(Date.now() % 2147483647);
```

Every launch gets a different town, and the seed is stored in the save
file so a loaded town is reproduced exactly.

### `CanPlace` is a *preview*

```go
// CanPlace reports whether a tile could be placed on a cell.
//
// The frontend calls this before sending a placement, so that the player is
// told what is wrong in their own terms instead of receiving a solver
// error. The two reasons a placement can fail are that the tile does not
// belong to the layer, and that the cell below cannot support it.
func (s *Service) CanPlace(q, r, layer int, tileType string) (bool, string) {
```

It returns a boolean *and a reason string*. The reason is written for a
player, not a programmer:

```go
return false, fmt.Sprintf("a %s needs something to stand on", tile)
```

This is a small design decision with a large effect on how the game feels.
The alternative — letting the placement fail and showing the solver's
error — produces messages like:

> `Error: RuntimeError: solve layer 1: wfc: constraints are contradictory:
> cells {Q:1 R:6 Layer:1} (tower) and {Q:2 R:6 Layer:1} (roof) cannot touch`

Which is *precise* and *useless*. The player cannot act on it.

## The snapshot carries the rules

This is the decision that keeps the frontend from restating the rules,
and it is easy to miss.

```go
type TownSnapshot struct {
	// Seed is the town's deterministic seed.
	Seed int64 `json:"seed"`
	// Radius is the radius of the town's hex grid, in cells.
	Radius int `json:"radius"`
	// Cells lists every settled cell in the town.
	Cells []Cell `json:"cells"`
	// Tiles lists the tile types available in the ruleset, so the frontend
	// can build its palette without hard-coding tile names.
	Tiles []string `json:"tiles"`
	// TileInfo lists the tile types together with the layer each belongs
	// to, so the frontend can offer only the tiles that are valid on the
	// layer the player is pointing at.
	TileInfo []TileInfo `json:"tileInfo"`
	// Connectors lists the connector pairs that may meet, so the renderer
	// can decide which cells connect without restating the rules.
	Connectors []ConnectorRule `json:"connectors"`
	// Props lists the decorative props, so the renderer can decorate the
	// town without restating the rules.
	Props []PropInfo `json:"props"`
}
```

`TileInfo` carries each tile's six edge connectors, its `up` and `down`,
and its `solid` flag. `Connectors` carries the allowed pairs.

**The renderer therefore does not know a single rule.** It knows how to
draw a wall, and it knows that the wall's north edge is `wall` and that
`wall` may touch `window`. It does not know that walls touch walls.

This is why adding a tile to the ruleset requires no frontend change at
all. The palette picks it up from `Tiles`, the renderer picks it up from
`TileInfo`, and the rules come along in `Connectors`.

## The frontend mirrors, it does not own

`frontend/src/game/state.ts` holds a copy of the grid:

```ts
export class TownState {
  /** The settled cells, keyed by "q,r,layer". */
  private readonly cells = new Map<string, string>();

  /** The colour of each settled cell, keyed the same way. */
  private readonly colors = new Map<string, string>();

  /**
   * The cells of each chunk, keyed by chunk.
   *
   * This index lets the renderer ask for a single chunk's cells without
   * walking the whole town, which is what makes a partial rebuild cheap.
   */
  private readonly cellsByChunk = new Map<...>();

  /** The chunks whose contents have changed since the last drain. */
  private readonly dirtyChunks = new Set<string>();
```

Note what is *not* here: no logic. The state does not know what a wall
is, does not know the rules, and does not decide anything. It is a
mirror, and a dirty-flag tracker.

The frontend can be *ahead* of the backend briefly, and it can be
*behind* — but it can never be *wrong*, because the backend never asks it
for anything. The frontend only ever asks:

```ts
const diff = await TownService.PlaceConstraint(q, r, layer, tile, color);
this.applyDiff(this.state, diff);
```

## Where the rules *do* appear in the frontend

Honesty requires naming the exceptions. There are exactly two places
where a tile name appears in the frontend, and both are about *drawing*,
not about *rules*:

**1. The geometry builder.** `buildCell` switches on the tile name to
decide what shape to draw. A wall is drawn as faces; a tower is drawn as
a block; a spire as a cone.

**2. The colour function.** `colorFor` gives roofs a terracotta tint and
gardens a green one.

Neither of these knows any rule. A wall's *shape* does not depend on
what may touch it — that is passed in separately, as the `links`
parameter.

And in the palette, tile names are used to build the layer groups, again
purely from the `TileInfo` data.

## A dependency you might not expect

The frontend has a dependency the Go side does not:

```json
"dependencies": {
    "three": "^0.180.0"
}
```

And a subtle one, from `frontend/src/game/geometry/buildings.ts`:

```ts
import { mergeGeometries } from "three/examples/jsm/utils/BufferGeometryUtils.js";
```

Three.js ships its addons as `examples/jsm`. This one merges a cell's
parts into a single geometry, so that a cell is one draw call. With
3602 cells and up to six parts each, that is the difference between
3602 and 20000 draw calls.

## The dependency graph, honestly

```
internal/wfc          → nothing (standard library only)
internal/town         → internal/wfc
internal/town/rules   → a JSON file, embedded
main.go               → internal/town, wails/v3
frontend/src          → three, @wailsio/runtime, the generated bindings
```

The solver has **no dependencies at all**. Not on the renderer, not on
Wails, not on the rules file format beyond what `ruleset.go` defines. It
is a self-contained algorithm that happens to live in the same repository
as a game.

That is the property that makes it worth keeping separate, and worth
testing on its own.

---

## Summary

| Layer | Owns | Knows nothing about |
|---|---|---|
| `internal/wfc` | The algorithm | Towns, players, layers, graphics |
| `internal/town` | The game state | Graphics, the DOM, the camera |
| `frontend/src` | The picture | The rules, the solver, the truth |

---

*Previous: [Chapter 5 — Propagation](05-propagation.md) · Next: [Chapter 7 — Writing the solver in Go](07-solver.md)*
