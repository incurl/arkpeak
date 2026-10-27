import * as THREE from "three";

/**
 * Hex grid maths.
 *
 * The grid uses "pointy-top" axial coordinates, matching the Go solver's
 * `wfc.CellKey`. A cell's world position is derived from its axial
 * coordinates, and the renderer adds a per-cell jitter on top so the town
 * looks hand-placed rather than machine-latticed.
 *
 * The jitter is deterministic: it is derived from the cell's coordinates, so
 * a cell never moves between frames or between runs.
 */

/** The width of a hex cell, edge to edge. */
export const HEX_SIZE = 1.0;

/** The vertical distance between layers. */
export const LAYER_HEIGHT = 1.6;

/** The maximum jitter applied to a cell, as a fraction of the hex size. */
export const JITTER_AMOUNT = 0.1;

/** The radius of the hex outline used for the ground and the hover marker. */
export const HEX_RADIUS = HEX_SIZE * 0.94;

/** The six axial direction vectors, matching the Go solver's order. */
export const HEX_DIRECTIONS: readonly (readonly [number, number])[] = [
  [0, -1], // N
  [1, -1], // NE
  [1, 0], // SE
  [0, 1], // S
  [-1, 1], // SW
  [-1, 0], // NW
];

/**
 * Converts an axial hex coordinate to a world position.
 *
 * Pointy-top hexes: the horizontal spacing is sqrt(3) times the size, and
 * the vertical spacing is 3/2 times the size.
 */
export function hexToWorld(q: number, r: number, layer: number): THREE.Vector3 {
  const x = HEX_SIZE * Math.sqrt(3) * (q + r / 2);
  const z = HEX_SIZE * 1.5 * r;
  const y = layer * LAYER_HEIGHT;
  return new THREE.Vector3(x, y, z);
}

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

/**
 * Hashes three integers into a 32-bit value.
 *
 * This is a simple integer hash; it does not need to be cryptographic, only
 * well distributed and deterministic.
 */
export function hash3(q: number, r: number, layer: number): number {
  let h = 2166136261;
  h = Math.imul(h ^ (q | 0), 16777619);
  h = Math.imul(h ^ (r | 0), 16777619);
  h = Math.imul(h ^ (layer | 0), 16777619);
  h ^= h >>> 15;
  h = Math.imul(h, 2246822507);
  h ^= h >>> 13;
  return h >>> 0;
}

/**
 * Hashes three integers and a string into a 32-bit value.
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
  let h = hash3(q, r, layer);
  for (let i = 0; i < text.length; i++) {
    h = Math.imul(h ^ text.charCodeAt(i), 16777619);
  }
  h ^= h >>> 15;
  h = Math.imul(h, 2246822507);
  h ^= h >>> 13;
  return h >>> 0;
}

/**
 * Finds the cell nearest to a world position.
 *
 * This is the inverse of hexToWorld: it converts the world position to
 * fractional axial coordinates and rounds them to the nearest cell. The
 * layer is not needed to find the cell, because the caller already knows
 * which layer's plane it intersected.
 */
export function worldToHex(point: THREE.Vector3): { q: number; r: number } {
  const qf =
    ((Math.sqrt(3) / 3) * point.x - (1 / 3) * point.z) / HEX_SIZE;
  const rf = ((2 / 3) * point.z) / HEX_SIZE;

  return axialRound(qf, rf);
}

/** Rounds fractional axial coordinates to the nearest cell. */
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

/**
 * Reports whether a cell lies within a hexagon of the given radius centred
 * on the origin.
 *
 * This mirrors the Go solver's `wfc.InHexRegion`, so the frontend and the
 * backend agree on where the island ends.
 */
export function inHexRegion(radius: number, q: number, r: number): boolean {
  const dq = q;
  const dr = r;
  const ds = dq + dr;

  const abs = (v: number): number => (v < 0 ? -v : v);
  return (abs(dq) + abs(dr) + abs(ds)) / 2 <= radius;
}
