# Chapter 10 — The bugs worth learning from

---

## Why this chapter exists

Everything else in this book describes how ArkPeak works *now*. But the
most useful part of a working system is often the list of things that
were wrong before it worked.

These are five real bugs from this codebase. Each one is a different
*kind* of bug, and each one produced a symptom that pointed away from the
cause. That is what makes them worth studying.

---

## Bug 1 — The town that rewrote itself

### Symptom

The player places a tile. The neighbourhood looks fine. A minute later,
some of the buildings nearby have changed shape, and the player did not
touch them.

Nothing looked broken. There was no error, no crash, no visual glitch.
The town was just... drifting.

### Diagnosis

The regional re-solve was not stable. When the player placed a tile, the
solver re-solved a region of about 61 cells, and cells that had no reason
to change sometimes did.

The cause was in the *preference* mechanism. A cell's existing tile was
expressed as a **weight boost** — a large extra weight added to that
tile's normal weight:

```go
// preferredBoost is how much extra weight a cell's preferred tile receives.
// It is large enough to dominate the other weights, so a cell keeps its tile
// unless keeping it is impossible.
const preferredBoost = 1000
```

Which sounds like it would work. And it *mostly* worked. But "mostly" is
the problem: a boost makes a tile *more likely*, not *certain*. With
enough re-solves over a play session, the small probability of a change
composes into a certainty that something changes.

The failure is a classic one. A probabilistic guarantee was used to
implement a deterministic requirement.

### The fix

A preference is now a *hard* preference, checked before the dice are
rolled:

```go
// A cell that would like to keep its tile keeps it outright, as long as
// it is still a candidate. Rolling the dice here would leave a small
// chance of the cell changing on every re-solve, which would slowly
// reshape the town while the player did nothing.
if c.preferredSet && c.has(c.preferred) {
    c.candidates = tileMask(c.preferred)
    c.settled = true
    c.collapsed = c.preferred
    return nil
}
```

And the boost remains, but only as a *tie-breaker* for cells where the
preference is not available.

### The tests

Three tests, and the third is the important one:

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
```

```go
// TestPreferredTilesAreRespectedAcrossManySeeds checks the same guarantee
// over many different seeds.
//
// A single seed could pass by luck. The guarantee has to hold whatever
// the town looks like.
```

```go
// TestPreferredTileIsGivenUpWhenImpossible checks that the guarantee has a
// limit: a cell must be allowed to change when keeping its tile would make
// the solve impossible.
//
// If a cell could never change, the player could paint the town into a
// corner and no placement would ever work again.
```

### The lesson

**When you need "always", do not implement it with "probably".**

And note the third test. When you fix a bug by strengthening an
invariant, you must also check that you have not made the system
incapable of doing something it legitimately needs to do. A stronger
guarantee that creates a new deadlock is not a fix.

---

## Bug 2 — The props that all landed on the same tile

### Symptom

The town was decorated, and it looked *wrong* in a specific way. Trees
were growing out of the pavement. Flowers were growing on the rooftops.
The distribution was obviously skewed, but the counts looked plausible
at first glance.

### Diagnosis

The prop placement hashed only the cell's *coordinates*:

```ts
const hash = hash2(q, r);
```

and then asked "is this a tile that can hold a tree?".

The problem is the interaction. Consider a garden cell at (3, 7) and a
floor cell at (3, 7) — same coordinates, different tile. They got the
*same* hash, so the same random number, so the same decision. The tree
landed on the garden, and the same hash was then compared against the
floor's prop table, and by coincidence the floor also drew a prop.

Worse: because the hash depended only on position, the same positions
across the whole town kept drawing the same prop. Every `tree` in the
town was at the same offset in the noise.

### The fix

Include the tile in the hash. The hash function takes the tile as its
fourth argument and mixes it in last:

```ts
/**
 * ...
 *
 * The string is mixed in last, so a cell's hash changes when its tile
 * changes. Without that, every cell at a given position would make the
 * same decision whatever it held.
 */
export function hash4(
  q: number,
  r: number,
  layer: number,
  text: string,
): number {
```

and the caller passes the tile:

```ts
const hash = hash4(cell.key.q, cell.key.r, cell.key.layer, cell.tile);
```

The consequence is recorded in the prop chooser's own comment:

```ts
/**
 * Chooses the prop for a cell, if any.
 *
 * The hash must include the tile. If it did not, every cell at a given
 * position would make the same decision whatever it held, and the
 * props would bunch up on whichever tile happened to share their
 * coordinates.
 *
 * @param tile the cell's tile.
 * @param hash a hash of the cell's coordinates and its tile.
 */
public choose(tile: string, hash: number): PropInfo | undefined {
```

### The lesson

**A hash is only as good as the set of things you hash.** If the thing
being hashed can influence the outcome, it belongs in the hash.

The general form: a deterministic hash used for *selection* must include
every input to the *selection*. This is the same principle as the
per-cell random stream in the solver (Chapter 7) — the random source must
be derived from everything that affects the result, or the result depends
on the order of operations.

The test for this class of bug is easy to state: **does the distribution
of outcomes match the intended distribution?** ArkPeak measures it, and
because the densities come from a hash rather than a random number
generator, the measurement is reproducible without running the game:

| Tile | Target density | Measured over a radius-24 island |
|---|---|---|
| garden | 85% | 86.4% |
| empty | 16% | 15.4% |
| floor | 12% | 11.4% |
| stairs | 20% | 20.0% |

If the measured densities are right, the distribution is right. That is a
much stronger check than "it looks OK".

---

## Bug 3 — The cells that vanished

### Symptom

Parts of the town were missing. Not obviously — you had to look. Some
buildings were simply absent from a region of the island, and there was a
visible straight-edged gap in the town.

### Diagnosis

The chunk system had two functions that had to agree: `chunkOf`, which
says which chunk a cell belongs to, and `chunkCells`, which lists the
cells in a chunk. They disagreed by a half-chunk offset:

```ts
// (the buggy versions)
export function chunkOf(q: number, r: number): ChunkCoord {
  return {
    cq: Math.floor(q / CHUNK_SIZE),
    cr: Math.floor(r / CHUNK_SIZE),
  };
}

export function chunkCells(cq: number, cr: number): CellKey[] {
  const startQ = cq * CHUNK_SIZE - CHUNK_SIZE / 2;
  ...
}
```

The consequence: a cell was assigned to chunk A, but appeared in chunk
B's cell list. Chunk A was rendered, and the cell was not in it. Chunk B
was rendered, and the cell was not in it either. The cell was in
*neither*, and vanished.

The two functions were written at different times, for different purposes,
and the offset was introduced in one of them.

### The fix

Two things. First, make them agree by construction — the current
`chunkOf` includes the same half-offset as `chunkCells`:

```ts
export function chunkOf(q: number, r: number): ChunkCoord {
  const half = CHUNK_SIZE / 2;
  return {
    cq: Math.floor((q + half) / CHUNK_SIZE),
    cr: Math.floor((r + half) / CHUNK_SIZE),
  };
}
```

Second — and this is the real fix — **check the agreement at startup**:

```ts
/**
 * Checks that the chunk partition is exact.
 *
 * chunkOf and chunkCells must agree: every cell returned by chunkCells
 * must belong to that chunk, and no cell may appear in two chunks. If they
 * disagree, a cell is drawn into a chunk that is never rendered, and it
 * silently disappears.
 *
 * This is checked at startup rather than in a unit test because the
 * frontend has no test runner; the check is cheap and runs once.
 */
export function verifyChunkPartition(): string | null {
```

The app now refuses to start with:

> Chunk partition is broken: cell (3, 7) is in chunk (0, 0) but chunkOf
> says (0, 1)

### The lesson

**When two functions must agree, assert their agreement.**

This is a general principle, and it is especially valuable when there is
no test runner. A startup check costs nothing (it runs once) and converts
a mysterious visual bug into a loud, immediate failure with a precise
message.

The same pattern is used for the ruleset:

```ts
const connectorProblem = verifyConnectorTables(
  snapshot.tileInfo ?? [],
  snapshot.connectors ?? [],
);
if (connectorProblem !== null) {
  hud.setStatus(`The ruleset is incomplete: ${connectorProblem}`);
  throw new Error(connectorProblem);
}
```

If you add a tile to `town_rules.json` and forget to add its connector
pairs, the app tells you immediately instead of rendering a broken town.

---

## Bug 4 — The failure that poisoned the town

### Symptom

The player tried something that did not fit. They got an error message.
Then they tried something else — and the town was broken. Large regions
had become unresolvable, and every subsequent attempt failed.

The error was *accurate* — it named the two tiles that conflicted — but
the town was permanently damaged by a single rejected action.

### Diagnosis

`PlaceConstraint` recorded the constraint *before* attempting the solve:

```go
t.constraints[key] = wfc.Constraint{Key: key, Tile: tile, Color: color}
diff, err := t.resolveAround(key, DefaultSolveRadius)
if err != nil {
    return GridDiff{}, err   // ← the constraint is still recorded!
}
```

When the solve failed, the constraint stayed in the town's state. Every
subsequent re-solve included it. Every subsequent re-solve failed the
same way. The town was poisoned.

This is a general hazard in any system where a *partial* mutation
precedes a *validation*: the validation fails, but the mutation is not
undone.

### The fix

Roll the constraint back on failure:

```go
diff, err := t.resolveAround(key, DefaultSolveRadius)
if err != nil {
    // The placement could not be satisfied. Roll the constraint
    // back, so a failed attempt does not poison the town: it must
    // not survive into a save file or into the next re-solve.
    if hadPrevious {
        t.constraints[key] = previous
    } else {
        delete(t.constraints, key)
        t.removeFromOrder(key)
    }
    return GridDiff{}, err
}
```

Note that the rollback restores the *previous* constraint if there was
one, and also removes the key from the ordering — because the ordering
is what lets the town be rebuilt exactly, and a stale key in it would
break that.

### The test

```go
// TestFailedPlacementDoesNotPoisonTheTown
```

### The lesson

**Undo partial mutations on failure, completely.**

The word "completely" is doing work. A rollback that restores the
constraint but leaves the ordering stale is a rollback that will produce
a *different* bug later, when the town is rebuilt or saved.

There is a general shape to this:

```go
// Save the old state.
old := state
// Mutate.
if err := mutate(); err != nil {
    // Put it back.
    state = old
    return err
}
```

It is not always possible to save-and-restore wholesale — sometimes the
mutation is large and the "old" state is expensive to keep. But where it
is possible, it is the safest thing you can do.

---

## Bug 5 — The error message nobody could act on

### Symptom

This one did not crash, and it did not corrupt anything. It was simply
bad.

The player tried to place a roof directly on open ground. The message
that appeared was:

```
Error: RuntimeError: solve layer 1: wfc: constraints are contradictory:
cells {Q:1 R:6 Layer:1} (tower) and {Q:2 R:6 Layer:1} (roof) cannot touch
```

Every part of that is true. It names the layer, the two cells, and the
two tiles. It is also completely useless, because:

1. The player did not place a *tower* or a *roof*. They placed a roof,
   and the tower appeared somewhere else as a consequence.
2. The cell coordinates are meaningless to a player who does not know
   the coordinate system.
3. There is no indication of what to do next.

### The fix, part one: a typed error

The solver now returns a structured error rather than a formatted string:

```go
// ConflictError reports two tiles that cannot sit next to each other.
//
// It carries the two tiles rather than only describing them, so that a
// caller can explain the problem in its own terms. The player does not need
// cell coordinates; they need to know which buildings clash.
type ConflictError struct {
	// First is the id of the first tile.
	First string
	// Second is the id of the second tile.
	Second string
	// FirstKey is the position of the first cell.
	FirstKey CellKey
	// SecondKey is the position of the second cell.
	SecondKey CellKey
}
```

The comment on that struct is the whole design in one sentence: *"The
player does not need cell coordinates; they need to know which buildings
clash."*

### The fix, part two: translate at the boundary

```go
// explain turns a solver error into something a player can act on.
//
// The solver's errors are precise about *why* a placement failed, but they
// read like a stack trace. A player needs to know what to do next.
//
// The solver's contradiction error carries the two tiles involved, so the
// message can be rebuilt from them rather than by scraping the error text.
func explain(err error) error {
	if err == nil {
		return nil
	}

	var conflict *wfc.ConflictError
	if errors.As(err, &conflict) {
		return fmt.Errorf(
			"that does not fit: a %s cannot sit next to a %s",
			conflict.First,
			conflict.Second,
		)
	}

	return err
}
```

Note `errors.As` rather than string matching. The message is *built from
the structured data*, not scraped out of a formatted string. This means
the error can be reworded without breaking the translation.

### The fix, part three: preview before committing

Better still, the frontend asks first:

```go
// CanPlace reports whether a tile could be placed on a cell.
//
// The frontend calls this before sending a placement, so that the player is
// told what is wrong in their own terms instead of receiving a solver
// error. The two reasons a placement can fail are that the tile does not
// belong to the layer, and that the cell below cannot support it.
func (s *Service) CanPlace(q, r, layer int, tileType string) (bool, string) {
```

which returns a message written for a player:

```go
return false, fmt.Sprintf("a %s needs something to stand on", tile)
```

### The lesson

**An error message is part of the user interface.**

There are three layers of defence here, and they are worth separating:

1. **Prevent** — check before acting (`CanPlace`).
2. **Translate** — make the failure legible (`explain`).
3. **Structure** — make the failure machine-readable (`ConflictError`).

And the general principle underneath all three:

> **Errors cross a boundary differently from data.** A `GridDiff` is a
> value the frontend consumes. An error is a message the *player* reads.
> They should not be the same thing.

---

## A pattern for finding these yourself

Every one of these five bugs had the same shape: **two things that must
agree, did not.**

| Bug | The two things |
|---|---|
| Town drift | The preference and the requirement |
| Props on the wrong tile | The hash and the decision |
| Missing cells | `chunkOf` and `chunkCells` |
| Poisoned town | The mutation and the validation |
| Unusable error | The solver's truth and the player's need |

Here is the checklist that came out of writing them:

1. **Find the places where two representations of the same thing exist.**
   In ArkPeak: the solver's grid and the frontend's mirror; the ruleset's
   connectors and the renderer's rules; `chunkOf` and `chunkCells`; the
   cell coordinates and the world positions.

2. **For each pair, ask: what happens if they disagree?** If the answer
   is "something is silently wrong", add a check.

3. **For each pair, ask: what happens if the *order* of updates changes?**
   That is Bug 2 and Bug 4.

4. **Write the test that would have caught it.** Each of the five bugs
   has a test named after the guarantee, not the implementation.

## One more thing about the tests

The Go test suite is the backbone of this. It has 55 tests across the two
packages, and it runs in about 47 seconds.

Two of its conventions are worth adopting:

**Test the guarantee, not the implementation.**

```go
// TestPreferredTileIsGivenUpWhenImpossible checks that the guarantee has a
// limit: a cell must be allowed to change when keeping its tile would make
// the solve impossible.
```

A test that asserts "the solver returns tile 3 at cell (2, 2)" breaks when
you improve the solver. A test that asserts "a cell keeps its tile unless
keeping it is impossible" survives.

**Make the test's comment explain the bug it came from.**

Every test in the solver package has a comment that says what went wrong
before. This is not documentation for its own sake: six months later, when
someone wants to "simplify" the preference mechanism, the comment is the
only thing that will stop them reintroducing Bug 1.

---

*Previous: [Chapter 9 — Rendering](09-rendering.md) · Next: [Chapter 11 — Where to go next](11-next.md)*
