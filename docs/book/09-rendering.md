# Chapter 9 — Drawing a town in Three.js

---

## The rendering problem

The town is 3602 cells across two layers. Every frame, the camera moves,
the water ripples, the shadows shift.

Three things must be true at once:

1. **It must look like a town**, not a grid of hexagons.
2. **It must be fast enough** to hold 60 frames per second on a laptop.
3. **It must be cheap to update** when the player places a tile.

The third is the interesting one, and it drove a lot of the architecture.

## The hex grid

ArkPeak uses **flat-topped hexagons** in axial coordinates, matching the
Go solver's `wfc.CellKey`:

```ts
/** The six axial direction vectors, indexed by Direction. */
export const HEX_DIRECTIONS: readonly (readonly [number, number])[] = [
  [0, -1], // N
  [1, -1], // NE
  [1, 0], // SE
  [0, 1], // S
  [-1, 1], // SW
  [-1, 0], // NW
];
```

The order matters and must match the Go side's `Direction` constants
exactly, because the connector names are indexed by direction.

### Axial coordinates, in one paragraph

A cell is at `(q, r)`. The six neighbours are `(0,-1)`, `(1,-1)`,
`(1,0)`, `(0,1)`, `(-1,1)`, `(-1,0)`. That is all. The third coordinate
is `q + r`, and it is never stored.

To convert to a world position:

```ts
export function hexToWorld(q: number, r: number, layer: number): THREE.Vector3 {
  const x = HEX_SIZE * Math.sqrt(3) * (q + r / 2);
  const z = HEX_SIZE * 1.5 * r;
  const y = layer * LAYER_HEIGHT;
  return new THREE.Vector3(x, y, z);
}
```

The `q + r / 2` term is what makes axial coordinates work: it skews the
rows so the cells nest. `sqrt(3)` and `1.5` are the horizontal and
vertical spacings for a unit-size flat-topped hexagon.

### Going the other way

Picking a cell from a world position is the inverse:

```ts
export function worldToHex(point: THREE.Vector3): { q: number; r: number } {
  const qf = ((Math.sqrt(3) / 3) * point.x - (1 / 3) * point.z) / HEX_SIZE;
  const rf = ((2 / 3) * point.z) / HEX_SIZE;
  return axialRound(qf, rf);
}
```

`axialRound` is the fiddly part. Converting to fractional coordinates
gives you a point somewhere in the continuous hex plane, and you have to
round it to the nearest cell — but naive rounding is wrong at the edges,
because the hexes are not squares. `axialRound` uses the standard
cube-rounding trick: convert to cube coordinates, round all three, and
fix up the largest error.

```ts
export function axialRound(qf: number, rf: number): { q: number; r: number } {
  const sf = -qf - rf;

  let q = Math.round(qf);
  let r = Math.round(rf);
  const s = Math.round(sf);

  const dq = Math.abs(q - qf);
  const dr = Math.abs(r - rf);
  const ds = Math.abs(s - sf);

  if (dq > dr && dq > ds) {
    q = -r - s;
  } else if (dr > ds) {
    r = -q - s;
  }

  return { q, r };
}
```

## The island is a hexagon

The grid is not a rectangle. This was a deliberate change during
Milestone 3, and the reasoning is worth repeating because it is the kind
of thing that only becomes obvious when you see it.

A rectangle in axial coordinates is a **rhombus** on screen. The rows are
skewed by the `q + r / 2` term, so what should be a square is a
parallelogram. It reads as a skewed slab, not as an island.

So the grid is a hexagon:

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

The frontend mirrors the rule so that hover-picking agrees with the
backend about where the island ends:

```ts
/**
 * Reports whether a cell lies within a hexagon of the given radius centred
 * on the origin.
 *
 * This mirrors the Go solver's `wfc.InHexRegion`, so the frontend and the
 * backend agree on where the island ends.
 */
export function inHexRegion(radius: number, q: number, r: number): boolean {
```

## Jitter: breaking the lattice

A hex grid is very visible. The eye reads the rows and the columns, and
the town looks like a spreadsheet.

The fix is a small deterministic offset per cell:

```ts
export const JITTER_AMOUNT = 0.1;

/**
 * Returns the deterministic jitter offset for a cell.
 *
 * The offset is derived from a hash of the cell's coordinates, so it is
 * stable across runs and across re-solves.
 */
export function hexJitter(q: number, r: number, layer: number): THREE.Vector3 {
  const h = hash3(q, r, layer);
  const angle = (h & 0xffff) * (Math.PI * 2) / 0x10000;
  const magnitude = ((h >>> 16) & 0xff) / 0xff;

  const spread = magnitude * JITTER_AMOUNT * HEX_SIZE;
  return new THREE.Vector3(
    Math.cos(angle) * spread,
    // The vertical jitter is smaller: a cell that bobs up and down reads as
    // unstable rather than hand-placed.
    Math.sin(angle) * spread * 0.35,
    Math.sin(angle) * spread,
  );
}
```

Ten percent of a cell's size. Enough to break the rows; not enough to
look like a mistake.

The offset is **deterministic** — derived from the coordinates, not
random. A tree that was at (3, 7) is still at (3, 7) after a redraw, and
after a restart.

## The neighbour model

This is the core of Milestone 5, and Chapter 5 covered the reasoning. Here
is the code.

```ts
export class TileRules {
  /** The edge connectors of each tile, keyed by tile id. */
  private readonly edges = new Map<string, EdgeConnectors>();

  /** The hex connector pairs that may meet. */
  private readonly hexPairs = new Set<string>();

  /**
   * Reports whether two connectors may meet across a hex edge.
   *
   * The wildcard connector matches anything, so a cell that is indifferent
   * to its neighbours connects to all of them.
   */
  public hexConnect(a: string, b: string): boolean {
    if (a === CONNECTOR_ANY || b === CONNECTOR_ANY) {
      return true;
    }
    if (a === CONNECTOR_NOTHING || b === CONNECTOR_NOTHING) {
      return false;
    }
    return this.hexPairs.has(pairKey(a, b));
  }
}
```

The rules arrive from the backend in the snapshot, so this class never
restates them.

### Two questions, two answers

```ts
export interface NeighbourLinks {
  /** Whether the cell may sit next to the neighbour, on each side. */
  readonly compatible: readonly boolean[];
  /** Whether a shared surface is drawn, on each side. */
  readonly linked: readonly boolean[];
  /** Whether each side has a neighbour cell at all. */
  readonly present: readonly boolean[];
}
```

`compatible` is the solver's question. `linked` is the renderer's
question. Chapter 5 explained why they differ; the short version is that
`empty` has `any` on every side, so it is compatible with everything, but
a wall should not draw a face towards it.

## Building the geometry

Each cell's geometry is built from parts and merged into one mesh:

```ts
export function buildCell(
  tile: string,
  links: NeighbourLinks,
  supported: boolean,
): CellParts {
```

The parts are split by material:

```ts
export interface CellParts {
  /** The main body: walls, floors, roofs, towers. */
  readonly body: THREE.BufferGeometry[];
  /** The details: window panes, doors, trim. */
  readonly details: THREE.BufferGeometry[];
}
```

### A wall, drawn from its links

```ts
function buildWall(links: NeighbourLinks, glazed: boolean): CellParts {
  const body: THREE.BufferGeometry[] = [];
  const details: THREE.BufferGeometry[] = [];

  const width = CELL_RADIUS * Math.sqrt(3);
  const anyLinked = links.linked.some((c) => c);

  if (!anyLinked) {
    // An isolated wall: a solid block, so it does not look like a
    // missing piece of a building.
    body.push(hexPrism(CELL_RADIUS * 0.9, STOREY));
    return { body, details };
  }

  for (let i = 0; i < 6; i++) {
    if (!links.linked[i]) {
      continue;
    }
    body.push(faceBox(i, width, STOREY, WALL_THICKNESS));
  }
```

An isolated wall is a solid block. A wall in the middle of a run draws
one face. A corner wall draws two. A wall in a cluster draws three, four,
five or six.

**The silhouette of the building encodes the constraint graph.** This is
the single most important idea in the renderer, and it is why ArkPeak can
look good without any textures or carefully modelled assets.

### The geometry cache

The geometry depends only on the tile and the *pattern* of linked sides.
There are 64 possible patterns (six sides, each on or off) and eleven
tiles — so at most 704 distinct geometries, and in practice about a
hundred.

```ts
private geometryFor(
  tile: string,
  links: NeighbourLinks,
  supported: boolean,
): THREE.BufferGeometry | undefined {
  const pattern = links.linked.map((c) => (c ? "1" : "0")).join("");
  const key = `${tile}:${pattern}:${supported ? "s" : "-"}`;

  let geometry = this.geometries.get(key);
  if (geometry === undefined) {
    const parts = buildCell(tile, links, supported);
    const merged = mergeParts(parts.body);
    if (merged === undefined) {
      return undefined;
    }
    geometry = merged;
    this.geometries.set(key, geometry);
  }
  return geometry;
}
```

Measured on a real town: **2327 meshes sharing 121 distinct
geometries**, rather than 2327 unique geometries.

### Merging, and why it matters

```ts
import { mergeGeometries } from "three/examples/jsm/utils/BufferGeometryUtils.js";
```

A wall can be up to six boxes. Without merging, that is six draw calls
per cell, and 3602 cells is over twenty thousand draw calls. Merged, it
is one per cell.

## Chunks: rebuilding only what changed

When the player places a tile, the town changes in a few cells. But the
*geometry* of neighbouring cells changes too — a wall that gains a
neighbour gains a face.

So the unit of rebuilding cannot be the cell. It is the **chunk**.

```ts
/** The number of hex cells along one side of a chunk. */
export const CHUNK_SIZE = 8;
```

A chunk is a square of axial coordinates:

```ts
export function chunkOf(q: number, r: number): ChunkCoord {
  const half = CHUNK_SIZE / 2;
  return {
    cq: Math.floor((q + half) / CHUNK_SIZE),
    cr: Math.floor((r + half) / CHUNK_SIZE),
  };
}
```

A radius-24 island spans 7 chunks along each axis, so 49 chunks in all.

### The dirty set

The state tracks which chunks have changed:

```ts
/** The chunks whose contents have changed since the last drain. */
private readonly dirtyChunks = new Set<string>();
```

The renderer drains it each frame:

```ts
public rebuildDirty(state: TownState): number {
  if (state.radius !== this.radius) {
    this.radius = state.radius;
    this.rebuildGround();
  }

  const dirty = state.drainDirtyChunks();
  for (const key of dirty) {
    this.rebuildChunk(key, state);
  }
  this.dropEmptyChunks(state);
  return dirty.length;
}
```

And the frame loop is:

```ts
const frame = (now: number): void => {
  const dt = Math.min((now - last) / 1000, 0.1);
  last = now;

  camera.update(dt);
  scene.update(dt, state);
  scene.render();

  requestAnimationFrame(frame);
};
```

`scene.update` does **nothing** on a frame where nothing changed. The
dirty set is empty, the loop is empty, and the cost is one `Set` lookup.

### The partition must be exact

Here is a bug that is easy to write and hard to see. The chunk
coordinates and the chunk's cells must agree exactly:

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

The original version of this code had `chunkOf` and `chunkCells`
disagreeing by a half-chunk offset, and the symptom was that some cells
were simply not drawn. The check now runs at startup and refuses to boot
if the partition is broken.

**This is the pattern worth copying.** When two pieces of code must agree
and there is no test runner, check their agreement at startup.

### What the measurement showed

Running the real geometry code against a real town snapshot gives:

| Measure | Result |
|---|---|
| Cells in the town | 3602 (1801 on the ground, 1801 on the upper storey) |
| Cells actually drawn | 2327 — all 1801 ground cells, plus the 526 upper ones that sit over solid ground |
| Distinct geometries in the cache | **121** |
| Distinct tile-and-neighbour patterns | 121 |
| Wall vertex count by linked sides | exactly `24 × links` |

That last row is the neighbour-awareness proof. A wall with one linked
side has 24 vertices; with five, it has 120. A wall with no linked sides
is a solid hexagonal block instead, at 40 vertices. The geometry is a
strict function of the neighbour pattern.

And the 121 is the reason the cache is worth having: 2327 meshes, 121
geometries, one draw call each.

## Props

The town is decorated with trees, shrubs, flowers, lamps, barrels and
rocks. They are declared in the ruleset:

```json
{ "id": "tree", "weight": 6, "tiles": ["garden", "empty"], "height": 1.1, "scale": 0.35 }
```

And they are placed by the frontend, deterministically:

```ts
/**
 * Chooses the prop for a cell, if any.
 *
 * The hash must include the tile. If it did not, every cell at a given
 * position would make the same decision whatever it held, and the
 * props would bunch up on whichever tile happened to share their
 * coordinates.
 */
public choose(tile: string, hash: number): PropInfo | undefined {
```

That comment records a bug: the first implementation hashed only the
coordinates, and every prop in the town landed on `floor`. Including the
tile fixed it.

The densities come from the same hash, so they are reproducible without
running the game. The target densities are 85% for a garden, 16% for an
empty plot, 12% for a floor and 20% for stairs; measured over a radius-24
island they land within a couple of points of those targets, because the
hash is close to uniform.

## The scene

`frontend/src/render/scene.ts` is deliberately short:

```ts
const sun = new THREE.DirectionalLight(SUN_COLOR, 2.1);
sun.position.set(18, 30, 12);
sun.castShadow = true;
sun.shadow.mapSize.set(2048, 2048);
```

A sun with shadows, an ambient light, a hemisphere light, and a fog that
matches the sky. The fog is doing real work: it hides the edge of the
island and makes the town feel like it is in a soft, hazy world.

The water is a single plane with a vertex shader that wobbles:

```glsl
// Two crossing wave trains give a gentle, irregular swell.
float w1 = sin(pos.x * 0.18 + uTime * 0.7) * 0.5;
float w2 = sin(pos.z * 0.23 - uTime * 0.5) * 0.35;
float w3 = sin((pos.x + pos.z) * 0.11 + uTime * 0.9) * 0.25;
```

Nothing simulated. Just a moving surface that catches the light.

## The camera

Orbit, pan, zoom:

```ts
this.targetAzimuth -= dx * 0.005;
this.targetPolar = clamp(
  this.targetPolar - dy * 0.005,
  this.minPolar,
  this.maxPolar,
);
```

The polar angle is clamped so you cannot look at the town from directly
above (which would show the grid) or from below (which would show
nothing).

And the camera frames the island on load:

```ts
/**
 * Frames the camera on a hex island of the given radius.
 *
 * The camera starts high enough to see the whole island and close enough
 * that the cells are readable.
 */
public frameIsland(radius: number): void {
  const span = radius * 2 + 4;
  this.targetDistance = Math.min(Math.max(span * 1.6, 12), 70);
```

## The complete render pipeline

```
pointer move
    ↓
raycast onto the active layer's plane
    ↓
worldToHex → cell (q, r, layer)
    ↓
hover marker moves; palette shows that layer's tiles
    ↓
click
    ↓
CanPlace? → if not, show the reason and stop
    ↓
PlaceConstraint → Go → solve the region → GridDiff
    ↓
state.applyDiff(diff) → mark the affected chunks dirty
    ↓
next frame: drain the dirty set, rebuild those chunks
```

Every arrow is either a value crossing the Wails boundary or a local
function call. There is no hidden state and no polling.

---

## Summary

| Technique | Why |
|---|---|
| Flat-topped hexagons | Matches the solver's coordinates exactly |
| Hexagonal island | A rectangle in axial coordinates is a skewed rhombus |
| Deterministic jitter | Breaks the grid without being random |
| Draw only linked faces | The silhouette encodes the constraint graph |
| Geometry cache | 2327 cells share 121 geometries |
| Chunked rebuilds | One click rebuilds one chunk of forty-nine |
| Startup assertions | Catch agreement bugs with no test runner |
