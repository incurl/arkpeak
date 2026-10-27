import type { CellKey, TownSnapshot } from "../types";

/**
 * A local mirror of the backend's settled grid.
 *
 * The backend is authoritative: the frontend never decides what a cell
 * contains, it only records what the backend told it. That keeps the render
 * layer a pure function of the mirrored state, and makes a diff trivial to
 * apply.
 */
export class TownState {
  /** The settled cells, keyed by "q,r,layer". */
  private readonly cells = new Map<string, string>();

  /** The colour of each settled cell, keyed the same way. */
  private readonly colors = new Map<string, string>();

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

    this.seed = snapshot.seed;
    this.radius = snapshot.radius;
    this.tiles = snapshot.tiles ?? [];

    for (const cell of snapshot.cells ?? []) {
      this.set(cell.q, cell.r, cell.layer, cell.tile, cell.color);
    }
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
  }

  /** Removes a settled cell. */
  public remove(q: number, r: number, layer: number): void {
    const key = TownState.key(q, r, layer);
    this.cells.delete(key);
    this.colors.delete(key);
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
}
