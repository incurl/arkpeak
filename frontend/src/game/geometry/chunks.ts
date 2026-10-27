import * as THREE from "three";

/**
 * Chunking of the hex grid.
 *
 * The town is split into chunks so that a change to a few cells only
 * rebuilds the chunks that contain them, rather than the whole town. A
 * chunk is a square of axial coordinates, which is simple to compute and
 * easy to reason about: two cells are in the same chunk when their chunk
 * coordinates match.
 */

/** The number of hex cells along one side of a chunk. */
export const CHUNK_SIZE = 8;

/**
 * The chunk a cell belongs to.
 */
export interface ChunkCoord {
  readonly cq: number;
  readonly cr: number;
}

/** The key used to look a chunk up in a map. */
export function chunkKey(cq: number, cr: number): string {
  return `${cq},${cr}`;
}

/**
 * The chunk a cell belongs to.
 *
 * A chunk covers the cells from cq*CHUNK_SIZE - CHUNK_SIZE/2 up to (but not
 * including) cq*CHUNK_SIZE + CHUNK_SIZE/2, which is what chunkCells
 * returns. The two must agree exactly, or a cell would be rebuilt into a
 * chunk that never gets drawn.
 */
export function chunkOf(q: number, r: number): ChunkCoord {
  const half = CHUNK_SIZE / 2;
  return {
    cq: Math.floor((q + half) / CHUNK_SIZE),
    cr: Math.floor((r + half) / CHUNK_SIZE),
  };
}

/**
 * Returns every cell in a chunk.
 *
 * The chunk is the square of axial coordinates centred on the chunk's
 * middle, which lines the chunk boundaries up with the hex grid's own
 * symmetry.
 */
export function chunkCells(cq: number, cr: number): { q: number; r: number }[] {
  const half = CHUNK_SIZE / 2;
  const centreQ = cq * CHUNK_SIZE;
  const centreR = cr * CHUNK_SIZE;

  const cells: { q: number; r: number }[] = [];
  for (let q = centreQ - half; q < centreQ + half; q++) {
    for (let r = centreR - half; r < centreR + half; r++) {
      cells.push({ q, r });
    }
  }
  return cells;
}

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
  for (let cq = -3; cq <= 3; cq++) {
    for (let cr = -3; cr <= 3; cr++) {
      for (const { q, r } of chunkCells(cq, cr)) {
        const owner = chunkOf(q, r);
        if (owner.cq !== cq || owner.cr !== cr) {
          return (
            `chunk ${cq},${cr} contains cell ${q},${r}, ` +
            `which belongs to chunk ${owner.cq},${owner.cr}`
          );
        }
      }
    }
  }
  return null;
}

/**
 * A chunk of the grid, holding the meshes for the cells it contains.
 */
export class Chunk {
  /** The chunk's coordinate. */
  public readonly coord: ChunkCoord;

  /** The group holding the chunk's meshes. */
  public readonly group = new THREE.Group();

  /**
   * The cells this chunk covers, keyed by "q,r,layer".
   *
   * The chunk holds the cells' data, not their meshes: a cell's geometry
   * depends on its neighbours, and a neighbour may live in another chunk.
   */
  private readonly cells = new Map<
    string,
    { key: { q: number; r: number; layer: number }; tile: string; color: string }
  >();

  /** The signature of each cell, so unchanged cells are left alone. */
  private readonly signatures = new Map<string, string>();

  constructor(coord: ChunkCoord) {
    this.coord = coord;
  }

  /**
   * Rebuilds the chunk from the given cells.
   *
   * The chunk holds the cells' data, not their meshes. The renderer turns
   * the data into geometry, because a cell's geometry depends on its
   * neighbours, which may live in another chunk.
   *
   * @param cells the cells this chunk covers, with their tile and colour.
   */
  public rebuild(
    cells: Iterable<{
      key: { q: number; r: number; layer: number };
      tile: string;
      color: string;
    }>,
  ): void {
    const seen = new Set<string>();

    for (const cell of cells) {
      const { q, r, layer } = cell.key;
      const key = `${q},${r},${layer}`;
      seen.add(key);

      const signature = `${cell.tile}:${cell.color}`;
      if (this.signatures.get(key) === signature) {
        continue;
      }

      this.cells.set(key, cell);
      this.signatures.set(key, signature);
    }

    // Drop the cells that are no longer settled.
    for (const key of [...this.cells.keys()]) {
      if (!seen.has(key)) {
        this.cells.delete(key);
        this.signatures.delete(key);
      }
    }
  }

  /** The number of cells in the chunk. */
  public get size(): number {
    return this.cells.size;
  }

  /**
   * The cells this chunk covers.
   */
  public get cellList(): readonly {
    key: { q: number; r: number; layer: number };
    tile: string;
    color: string;
  }[] {
    return [...this.cells.values()];
  }
}
