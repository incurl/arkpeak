import type { CellKey, GridDiff, TownSnapshot } from "../types";
import { chunkKey, chunkOf } from "./geometry/chunks";

/**
 * A local mirror of the backend's settled grid.
 *
 * The backend is authoritative: the frontend never decides what a cell
 * contains, it only records what the backend told it. That keeps the render
 * layer a pure function of the mirrored state, and makes a diff trivial to
 * apply.
 *
 * The state also tracks which chunks have changed since the renderer last
 * looked. The renderer rebuilds only those, so a click that changes three
 * cells does not redraw the whole town.
 */
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
  private readonly cellsByChunk = new Map<
    string,
    Map<string, { q: number; r: number; layer: number; tile: string; color: string }>
  >();

  /** The chunks whose contents have changed since the last drain. */
  private readonly dirtyChunks = new Set<string>();

  /** The seed of the town this state mirrors. */
  public seed = 0;

  /** The radius of the town's hex grid, in cells. */
  public radius = 0;

  /** The tile types available in the ruleset. */
  public tiles: string[] = [];

  /** Replaces the whole state with a fresh snapshot. */
  public load(snapshot: TownSnapshot): void {
    this.cells.clear();
    this.colors.clear();
    this.cellsByChunk.clear();
    this.dirtyChunks.clear();

    this.seed = snapshot.seed;
    this.radius = snapshot.radius;
    this.tiles = snapshot.tiles ?? [];

    for (const cell of snapshot.cells ?? []) {
      this.set(cell.q, cell.r, cell.layer, cell.tile, cell.color);
    }

    // A fresh snapshot invalidates every chunk.
    this.invalidateAll();
  }

  /**
   * Applies a diff from the backend.
   *
   * Only the chunks containing a changed cell are marked dirty, so the
   * renderer rebuilds those and leaves the rest of the town alone.
   */
  public applyDiff(diff: GridDiff): void {
    for (const cell of diff.removed ?? []) {
      this.remove(cell.q, cell.r, cell.layer);
    }
    for (const cell of diff.settled ?? []) {
      this.set(cell.q, cell.r, cell.layer, cell.tile, cell.color);
    }
  }

  /** Marks every chunk as needing a rebuild. */
  public invalidateAll(): void {
    this.dirtyChunks.clear();
    for (const key of this.cellsByChunk.keys()) {
      this.dirtyChunks.add(key);
    }
  }

  /**
   * Returns the chunks that have changed since the last call, and clears
   * the set.
   *
   * The renderer calls this to find out what to rebuild.
   */
  public drainDirtyChunks(): string[] {
    const dirty = [...this.dirtyChunks];
    this.dirtyChunks.clear();
    return dirty;
  }

  /** Reports whether any chunk is waiting to be rebuilt. */
  public get hasDirtyChunks(): boolean {
    return this.dirtyChunks.size > 0;
  }

  /** Records a settled cell. */
  public set(
    q: number,
    r: number,
    layer: number,
    tile: string,
    color: string,
  ): void {
    const key = TownState.key(q, r, layer);
    this.cells.set(key, tile);
    this.colors.set(key, color ?? "");

    const coord = chunkOf(q, r);
    const chunkKey = TownState.chunkKey(coord.cq, coord.cr);
    let bucket = this.cellsByChunk.get(chunkKey);
    if (bucket === undefined) {
      bucket = new Map();
      this.cellsByChunk.set(chunkKey, bucket);
    }
    bucket.set(key, { q, r, layer, tile, color: color ?? "" });

    this.dirtyChunks.add(chunkKey);
  }

  /** Removes a settled cell. */
  public remove(q: number, r: number, layer: number): void {
    const key = TownState.key(q, r, layer);
    this.cells.delete(key);
    this.colors.delete(key);

    const coord = chunkOf(q, r);
    const chunkKey = TownState.chunkKey(coord.cq, coord.cr);
    this.cellsByChunk.get(chunkKey)?.delete(key);

    this.dirtyChunks.add(chunkKey);
  }

  /**
   * Returns the cells of a chunk.
   *
   * The renderer asks for one chunk at a time when it rebuilds, so this
   * must not walk the whole town.
   */
  public cellsInChunk(
    cq: number,
    cr: number,
  ): { key: { q: number; r: number; layer: number }; tile: string; color: string }[] {
    const bucket = this.cellsByChunk.get(TownState.chunkKey(cq, cr));
    if (bucket === undefined) {
      return [];
    }

    const out: {
      key: { q: number; r: number; layer: number };
      tile: string;
      color: string;
    }[] = [];
    for (const cell of bucket.values()) {
      out.push({
        key: { q: cell.q, r: cell.r, layer: cell.layer },
        tile: cell.tile,
        color: cell.color,
      });
    }
    return out;
  }

  /** Reports whether a chunk holds any cells. */
  public hasCellsInChunk(key: string): boolean {
    const bucket = this.cellsByChunk.get(key);
    return bucket !== undefined && bucket.size > 0;
  }

  /** Returns the tile of a cell, or undefined if it is not settled. */
  public tile(q: number, r: number, layer: number): string | undefined {
    return this.cells.get(TownState.key(q, r, layer));
  }

  /** Returns the colour of a cell, or an empty string. */
  public color(q: number, r: number, layer: number): string {
    return this.colors.get(TownState.key(q, r, layer)) ?? "";
  }

  /** Reports whether a cell is settled. */
  public has(q: number, r: number, layer: number): boolean {
    return this.cells.has(TownState.key(q, r, layer));
  }

  /** Iterates over every settled cell. */
  public *entries(): IterableIterator<{
    key: CellKey;
    tile: string;
    color: string;
  }> {
    for (const [key, tile] of this.cells) {
      const [q, r, layer] = key.split(",").map(Number);
      yield {
        key: { q, r, layer },
        tile,
        color: this.colors.get(key) ?? "",
      };
    }
  }

  /** The number of settled cells. */
  public get size(): number {
    return this.cells.size;
  }

  /** Builds the map key for a cell position. */
  private static key(q: number, r: number, layer: number): string {
    return `${q},${r},${layer}`;
  }

  /** Builds the key for a chunk. */
  private static chunkKey(cq: number, cr: number): string {
    return chunkKey(cq, cr);
  }
}
