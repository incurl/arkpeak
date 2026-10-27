import type { PropInfo } from "../../types";

/**
 * Where the props go.
 *
 * A prop is decoration that sits on a cell: a tree on a garden, a lamp on
 * a floor. The props are declared in the ruleset, and this module decides
 * which of them appears on which cell.
 *
 * The placement is deterministic: it is derived from the cell's
 * coordinates, so a tree stays a tree when the town is redrawn, and the
 * same town always looks the same. Nothing here is random at run time.
 */

/**
 * How a prop is placed on a cell.
 */
export interface PlacedProp {
  /** The prop's id. */
  readonly id: string;
  /** Where the prop sits, in the cell's local space. */
  readonly x: number;
  readonly z: number;
  /** How large the prop is, relative to its declared height. */
  readonly scale: number;
  /** Which way the prop faces, in radians. */
  readonly rotation: number;
}

/**
 * The props that fit on a tile, with their weights.
 */
export class PropRules {
  /** The props, grouped by the tile they fit on. */
  private readonly byTile = new Map<string, PropInfo[]>();

  constructor(props: readonly PropInfo[]) {
    for (const prop of props) {
      for (const tile of prop.tiles ?? []) {
        const list = this.byTile.get(tile);
        if (list === undefined) {
          this.byTile.set(tile, [prop]);
        } else {
          list.push(prop);
        }
      }
    }
  }

  /** The props that may appear on the given tile. */
  public forTile(tile: string): readonly PropInfo[] {
    return this.byTile.get(tile) ?? [];
  }

  /**
   * Chooses the prop for a cell, if any.
   *
   * A cell does not always get a prop: some cells are left empty so the
   * town does not look over-decorated. The chance of a prop depends on
   * the tile, so a garden is fuller than a corridor.
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
    const candidates = this.forTile(tile);
    if (candidates.length === 0) {
      return undefined;
    }

    // The chance of a prop at all, taken from the first byte of the hash.
    const chance = (hash & 0xff) / 0xff;
    if (chance > this.density(tile)) {
      return undefined;
    }

    // Pick a prop by weight, using the next bits of the hash.
    let total = 0;
    for (const prop of candidates) {
      total += prop.weight;
    }

    let pick = ((hash >>> 8) & 0xffff) % total;
    for (const prop of candidates) {
      pick -= prop.weight;
      if (pick < 0) {
        return prop;
      }
    }

    return candidates[candidates.length - 1];
  }

  /**
   * How likely a cell of the given tile is to get a prop, from 0 to 1.
   */
  private density(tile: string): number {
    switch (tile) {
      case "garden":
        return 0.85;
      case "empty":
        return 0.16;
      case "floor":
        return 0.12;
      case "stairs":
        return 0.2;
      default:
        return 0;
    }
  }
}

/**
 * Works out where a prop sits on its cell.
 *
 * The prop is offset from the centre and turned to face outwards, so a
 * row of trees does not look like a row of copies.
 */
export function placeProp(
  prop: PropInfo,
  hash: number,
): PlacedProp {
  const angle = ((hash >>> 16) & 0xffff) * (Math.PI * 2) / 0x10000;
  const spread = (((hash >>> 24) & 0xff) / 0xff) * 0.34;

  // The size varies a little from cell to cell, so a row of trees is not
  // a row of identical trees.
  const size = ((hash >>> 8) & 0xff) / 0xff;
  const scale = 1 + (size - 0.5) * 2 * (prop.scale ?? 0);

  return {
    id: prop.id,
    x: Math.cos(angle) * spread,
    z: Math.sin(angle) * spread,
    scale,
    rotation: angle,
  };
}
