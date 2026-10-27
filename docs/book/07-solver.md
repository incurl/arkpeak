# Chapter 7 — Writing the solver in Go

---

## Why Go for this

The solver does a lot of bit twiddling and a lot of map lookups. Go is
good at both, has no garbage-collector pauses worth caring about in a
loop that runs for a few seconds, and compiles to a single binary — which
matters when the same binary has to be a Linux package, a Windows
installer and a macOS app bundle.

But the real reason is that Go's **explicit error handling** suits a solver.
A WFC solve can fail in two ways: it can find a contradiction (expected,
handled) or it can hit a bug (unexpected, should panic loudly). Go's
`error` return makes the first case explicit and the second case
impossible to ignore.

## The cell

Everything rests on this struct:

```go
type cell struct {
	// candidates is the set of tiles this cell may still hold, as a
	// bitmask. One bit per tile.
	candidates uint64

	// layer is the layer this cell is on.
	layer int

	// settled reports whether the cell has been collapsed to a single tile.
	settled bool

	// collapsed records the tile this cell was collapsed to, for use as a
	// fixed boundary during a later regional re-solve.
	collapsed TileID

	// color is the player-chosen colour carried through to the result.
	color string

	// fixed reports whether the cell was pinned by the player. Fixed cells
	// are never re-collapsed during a regional re-solve.
	fixed bool

	// preferred is a tile the cell would like to keep, used to keep a
	// regional re-solve stable. It is a preference, not a constraint: the
	// cell may still be collapsed to a different tile if the preference
	// turns out to be impossible.
	preferred TileID

	// preferredSet reports whether preferred is meaningful.
	preferredSet bool
}
```

Two of these fields (`preferred`, `preferredSet`) exist because of a bug
that Chapter 10 covers in full. For now, note that they exist at all.

### The bitmask

```go
// maxTiles is the maximum number of tile types a ruleset may define. The
// solver stores candidate sets as bitmasks, so this is bounded by the width
// of the mask type.
const maxTiles = 64
```

Sixty-four tiles, one bit each. `uint64` exactly.

```go
// tileMask returns the bitmask with only the given tile's bit set.
func tileMask(t TileID) uint64 {
	return 1 << uint64(t)
}

// maskTiles returns the tiles in the mask, in order.
func maskTiles(mask uint64, out []TileID) []TileID {
	...
}

// popcount returns the number of set bits in v.
func popcount(v uint64) int {
```

Every operation the solver needs on a candidate set is a handful of
instructions:

| Operation | Code |
|---|---|
| Is tile T allowed? | `c.has(t)` → `c.candidates&(1<<t) != 0` |
| How many options? | `c.count()` → `popcount(c.candidates)` |
| Forbid tile T | `c.candidates &^= tileMask(t)` |
| Force tile T | `c.candidates = tileMask(t)` |

The `&^=` operator is Go's *clear* operation: "set these bits to zero".
It is exactly right for pruning.

### Why 64 and not more

Because `uint64` fits in a register, and because `popcount` of a `uint64`
is a single instruction on the platforms people actually run. If ArkPeak
needed 200 tiles you would need a different representation — probably
`[]uint64` bitsets with a manual popcount — and you would lose most of
the simplicity.

Eleven tiles fits with room to spare. The constraint is a design
constraint: **if a ruleset needs more than 64 tile types, the data model
needs rethinking first.**

## Compiling the ruleset

The JSON ruleset is compiled once into lookup tables:

```go
type Ruleset struct {
	// Tiles is the ordered list of tile definitions. A TileID is an index
	// into this slice.
	Tiles []TileDef

	// byID maps a tile's string ID to its TileID.
	byID map[string]TileID

	// hexConnectors maps a connector pair to whether the two connectors may
	// meet across a hex edge. It is symmetric.
	hexConnectors map[connectorPair]bool

	// upConnectors maps a connector pair to whether the lower connector may
	// support the upper one. It is directional.
	upConnectors map[connectorPair]bool

	// layerTiles maps a layer key to the tiles valid on that layer. The
	// special key anyLayerKey holds tiles valid on every layer.
	layerTiles map[string][]TileID
}
```

Two hash maps for the connectors, keyed by a pair:

```go
// connectorPair is an ordered pair of connector values.
type connectorPair struct {
	lower string
	upper string
}
```

Because the pair is *ordered*, the hex and up tables can be different
tables with different semantics:

```go
if rule.Axis == AxisHex {
    // Hex adjacency is symmetric.
    r.hexConnectors[connectorPair{rule.A, rule.B}] = true
    r.hexConnectors[connectorPair{rule.B, rule.A}] = true
} else {
    // Vertical adjacency is directional.
    r.upConnectors[connectorPair{rule.A, rule.B}] = true
}
```

That asymmetry is the whole point. `wall` supports `tower`, but `tower`
does not support `wall`. You cannot express that with one symmetric
table.

### The lookup

```go
// HexConnectorsMatch reports whether two connector values may meet across a
// hex edge.
//
// The wildcard connector matches anything, so a tile that is indifferent to
// its neighbours is compatible with all of them.
func (r *Ruleset) HexConnectorsMatch(a, b string) bool {
	if a == ConnectorAny || b == ConnectorAny {
		return true
	}
	if a == ConnectorNothing || b == ConnectorNothing {
		return false
	}
	return r.hexConnectors[connectorPair{a, b}]
}
```

Two special cases before the table lookup, then a map read. That is the
hot path of the entire solver, and it is three branches and a hash.

## Determinism: the design decision that mattered most

This is the part of ArkPeak's solver that differs most from Gumin's
reference implementation, and it is worth understanding properly.

Gumin's implementation uses one random stream:

```csharp
int r = distribution.Random(random.NextDouble());
```

ArkPeak uses one random stream **per cell**:

```go
// cellRNG returns the random source dedicated to a cell. It is seeded from
// the town seed and the cell's coordinates, making each cell's random
// decisions independent of solve order.
func (s *Solver) cellRNG(key CellKey) *rand.Rand {
	h := uint64(s.seed)
	h = h*6364136223846793005 + uint64(key.Q+0x10000)*1442695040888963407
	h = h*6364136223846793005 + uint64(key.R+0x10000)*1442695040888963407
	h = h*6364136223846793005 + uint64(key.Layer+0x10000)*1442695040888963407
	h ^= h >> 33
	return rand.New(rand.NewSource(int64(h)))
}
```

That is a hash of `(seed, q, r, layer)` used as a seed for a fresh
`rand.Rand`.

### Why this matters

Consider the regional re-solve. When the player places a wall, the solver
re-solves a region of about 61 cells. Those cells collapse in an order
determined by which is most constrained, and that order *changes* depending
on the wall you placed.

With a single global random stream, the random numbers each cell receives
depend on how many cells were collapsed before it. So the same cell, in the
same town, could get a different tile depending on the path taken to reach
it.

With a per-cell stream, the cell's random numbers depend only on its own
coordinates and the seed. **The order of collapses cannot affect the
outcome.**

This is not a small thing. It is what makes the following properties
true:

- A regional re-solve is **reproducible**. Solving the same region with
  the same seed gives the same result, always.
- A town rebuilt from its constraints is **identical** to the town that
  was saved.
- The frontend's mirror never disagrees with the backend, because the
  backend can recompute any region at any time and get the same answer.

### The hash function

```go
h := uint64(s.seed)
h = h*6364136223846793005 + uint64(key.Q+0x10000)*1442695040888963407
```

That is the constants from SplitMix64 / PCG-style hashing: a large odd
multiplier (the "golden ratio" constant) and an increment. The `+0x10000`
offsets keep the coordinates from being negative, because Go's conversion
of a negative `int` to `uint64` would produce a huge number.

Then a final xor-shift to mix the high bits down:

```go
h ^= h >> 33
```

This is a well-known pattern and it does not need to be perfect. It needs
to be *deterministic* and *well distributed*. Determinism it certainly
is.

### The cost

One `rand.Rand` allocation per collapse. For a 1801-cell town, that is
1801 small allocations — negligible compared to the solving itself. The
reference implementation's single stream is marginally cheaper, but it
buys the guarantee ArkPeak needs more.

## The observation phase

```go
// SelectCell returns the unsettled cell with the fewest remaining
// candidates.
//
// Ties are broken by the solver's stable cell order, so the choice is
// deterministic for a given seed and region. It returns false when every cell
// is settled.
func (s *Solver) SelectCell() (CellKey, bool) {
```

Chapter 4 discussed the entropy heuristic and why ArkPeak uses candidate
count instead.

One detail worth noticing: the zero-candidate case.

```go
n := c.count()
if n == 0 {
    // A cell with no candidates is a contradiction; report it
    // as the most constrained cell so the caller reports it.
    return k, true
}
```

A cell with no options is the *most* constrained cell possible, so it is
returned immediately and the caller discovers the contradiction when it
tries to collapse it. This means a contradiction is reported in terms of
the cell that actually has the problem.

## The collapse phase

```go
// Collapse picks a tile for the given cell using weighted random selection
// and settles it.
//
// The random source is derived per cell from the town's seed, so the choice
// for a cell does not depend on how many cells were collapsed before it.
// That keeps regional re-solves reproducible.
func (s *Solver) Collapse(key CellKey) error {
```

The weighted choice:

```go
total := 0
for _, t := range tiles {
    total += s.weightFor(c, t)
}
if total <= 0 {
    return fmt.Errorf("%w: cell %+v has no weighted candidates", ErrContradiction, key)
}

pick := s.cellRNG(key).Intn(total)
chosen := tiles[0]
for _, t := range tiles {
    w := s.weightFor(c, t)
    if pick < w {
        chosen = t
        break
    }
    pick -= w
}
```

This is a standard weighted pick: draw a number in `[0, total)` and walk
the tiles subtracting their weights until the number goes negative.

`weightFor` adds the preference boost:

```go
// preferredBoost is how much extra weight a cell's preferred tile receives.
// It is large enough to dominate the other weights, so a cell keeps its tile
// unless keeping it is impossible.
const preferredBoost = 1000
```

Chapter 10 explains why this boost was not enough, and what replaced it.

## The solve loop

```go
// Solve collapses every cell in the region.
//
// Cells are collapsed in ascending order of remaining candidate count, so the
// most constrained cell is decided first and contradictions surface early.
func (s *Solver) Solve() error {
	if err := s.Propagate(); err != nil {
		return err
	}

	for {
		key, ok := s.SelectCell()
		if !ok {
			return nil
		}
		if err := s.Collapse(key); err != nil {
			return err
		}
		if err := s.Propagate(); err != nil {
			return err
		}
	}
}
```

That is the entire algorithm. Observation, propagation, observation,
propagation, until every cell is settled.

Notice that `Propagate` is called *before* the first observation. That
handles the case where the region starts with forced cells (a constraint
pins one, and propagation cascades from it).

## The regional re-solve

This is where the game lives, and it is the part with no equivalent in
Gumin's work.

When the player places a tile, the town does not re-solve everything. It
re-solves a region:

```go
// resolveAround re-solves the cells within radius of center and reports what
// changed.
//
// Cells outside the re-solved region are treated as fixed boundaries, so a
// local change cannot disturb the rest of the town. The layers are solved
// from the ground up, so a change on layer 0 correctly affects layer 1.
//
// The cells already in the region keep their current tile unless the change
// makes it impossible. This keeps a re-solve stable: placing a block in one
// spot does not reshuffle the whole neighbourhood.
func (t *Town) resolveAround(center wfc.CellKey, radius int) (GridDiff, error) {
```

Three mechanisms make it stable:

**1. Outside cells are pinned.**

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

`FixCell` sets the cell's candidates to exactly one tile and marks it
settled:

```go
// FixCell pins a cell to a tile without marking it as a player constraint.
// It is used to seed a regional re-solve from the town's existing state.
func (s *Solver) FixCell(key CellKey, tile TileID) {
	c, ok := s.cells[key]
	if !ok {
		return
	}
	c.candidates = tileMask(tile)
	c.settled = true
	c.collapsed = tile
}
```

**2. Inside cells are preferred, not pinned.**

```go
// Keep the cells that already have a tile, unless the player has
// placed a constraint on them. The constraint is applied after,
// so it wins.
for key, cell := range t.cells {
    if key.Layer != layer || !solver.InRegion(key) {
        continue
    }
    if _, pinned := t.constraints[key]; pinned {
        continue
    }
    if tile, ok := t.rules.Lookup(cell.Tile); ok {
        solver.KeepCell(key, tile)
    }
}
```

`KeepCell` sets a *preference*, not a pin:

```go
// KeepCell records a cell's current tile as a strong preference.
//
// Unlike FixCell, the cell is not pinned: the solver may still choose a
// different tile if the preference turns out to be impossible. This is what
// keeps a regional re-solve stable — a cell keeps its tile unless the
// player's change genuinely rules it out.
func (s *Solver) KeepCell(key CellKey, tile TileID) {
```

**3. Player constraints are applied last, and win.**

```go
if err := solver.ConstraintsWithin(t.constraintList()); err != nil {
    return err
}
```

`applyConstraint` *pins*:

```go
func (s *Solver) applyConstraint(c *cell, con Constraint) error {
	...
	c.candidates = tileMask(tile)
	c.settled = true
	c.collapsed = tile
	c.color = con.Color
	c.fixed = true
	return nil
}
```

So the precedence is: **player constraint > preference > free choice.**

## Testing the solver

The solver is tested against a hand-made five-tile ruleset. Here is one
of the tests, and it is a good example of the style used throughout:

```go
// TestPreferredTilesSurviveReSolve checks that a cell which would like to
// keep its tile actually keeps it.
//
// This is the guarantee a regional re-solve rests on: placing a block
// somewhere must not quietly reshape the neighbourhood around it. The
// preference used to be applied as a weight boost, which left a small
// chance of the cell changing on every re-solve. Over the many re-solves
// of a play session, that was enough to slowly rewrite the town.
//
// The tiles in the test ruleset all have the same weight, so a change
// here cannot be explained by the weights.
func TestPreferredTilesSurviveReSolve(t *testing.T) {
```

Note the comment. The test exists because of a bug, and the comment
records *why* the test is shaped the way it is. The "all the same weight"
detail is not decoration: it rules out the alternative explanation, so
that a failure means what it says.

There is a companion test across many seeds, because a single seed could
pass by luck:

```go
// TestPreferredTilesAreRespectedAcrossManySeeds checks the same guarantee
// over many different seeds.
//
// A single seed could pass by luck. The guarantee has to hold whatever
// the town looks like.
```

And a test for the limit of the guarantee:

```go
// TestPreferredTileIsGivenUpWhenImpossible checks that the guarantee has a
// limit: a cell must be allowed to change when keeping its tile would make
// the solve impossible.
//
// If a cell could never change, the player could paint the town into a
// corner and no placement would ever work again.
```

That third test is the one people forget to write, and it is the one that
stops a "fix" from being a new bug.

## Performance

A radius-24 island is 1801 cells per layer, two layers. A full solve
takes about 1.8 seconds in Go on a typical laptop. A regional re-solve of
61 cells takes milliseconds.

The Go test suite takes about 47 seconds, most of which is the town
package's tests, each of which solves a full town.

The bitmask representation is the reason this is fast enough to be
interactive. A solver using `map[TileID]bool` per cell would be perhaps
five to ten times slower, which would be the difference between "the town
appears instantly" and "there is a spinner".

---

## Summary

| Decision | Why |
|---|---|
| `uint64` bitmask | Candidate sets fit in a register; `popcount` is one instruction |
| Per-cell random stream | Re-solves are reproducible; order cannot affect outcome |
| Compiled connector tables | The hot path is a map read, not a scan |
| `FixCell` vs `KeepCell` | Pinning outside the region, preference inside it |
| Constraints applied last | The player's choice always wins |
| Small, hand-made test ruleset | Solver tests stay readable and fast |

---

*Previous: [Chapter 6 — Architecture](06-architecture.md) · Next: [Chapter 8 — Wails: a Go program with a web page inside](08-wails.md)*
