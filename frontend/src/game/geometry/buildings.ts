import * as THREE from "three";
import { mergeGeometries } from "three/examples/jsm/utils/BufferGeometryUtils.js";

import type { NeighbourLinks } from "./neighbours";
import {
  CELL_RADIUS,
  STOREY_HEIGHT,
  detailBox,
  faceBox,
  hexPrism,
  hexSlab,
  roofWedge,
  spire,
} from "./shapes";

/**
 * The procedural building geometry.
 *
 * Each tile is drawn according to what it is and what it touches. A wall
 * only draws the faces that meet another wall, so a building reads as a
 * solid volume with an outline, rather than as a stack of hexagons. A
 * window draws a frame and a pane. A roof caps the building below it.
 *
 * The geometry for a cell is built from parts, and the parts are merged
 * into a single mesh. A cell is therefore one draw call, which matters:
 * the town has thousands of cells.
 */

/** The height of a storey. */
export const STOREY = STOREY_HEIGHT;

/** The height of a flat ground tile. */
export const GROUND_HEIGHT = 0.1;

/** The thickness of a wall panel. */
export const WALL_THICKNESS = 0.18;

/** The height of a window. */
export const WINDOW_HEIGHT = 0.62;

/** The width of a window. */
export const WINDOW_WIDTH = 0.5;

/** The inset of a window pane from the wall face. */
export const WINDOW_INSET = 0.06;

/**
 * The parts of a single cell's geometry.
 *
 * The parts are split by material, because a building has several colours:
 * the wall itself, the roof, the window glass and the trim.
 */
export interface CellParts {
  /** The main body: walls, floors, roofs, towers. */
  readonly body: THREE.BufferGeometry[];
  /** The details: window panes, doors, trim. */
  readonly details: THREE.BufferGeometry[];
}

/**
 * Builds the geometry of a cell.
 *
 * @param tile the tile the cell holds.
 * @param links which sides of the cell connect to a neighbour.
 * @param supported whether the cell below can carry this one.
 */
export function buildCell(
  tile: string,
  links: NeighbourLinks,
  supported: boolean,
): CellParts {
  switch (tile) {
    case "wall":
      return buildWall(links, false);
    case "window":
      return buildWall(links, true);
    case "arch":
      return buildArch(links);
    case "tower":
      return buildTower();
    case "spire":
      return buildSpire();
    case "roof":
      return buildRoof(false);
    case "roof_edge":
      return buildRoof(true);
    case "floor":
      return buildFloor();
    case "garden":
      return buildGarden();
    case "stairs":
      return buildStairs();
    default:
      return buildEmpty(supported);
  }
}

/** An empty plot: a thin slab, or nothing if it is covered. */
function buildEmpty(supported: boolean): CellParts {
  if (supported) {
    // Something stands on this plot, so the plot itself is not drawn.
    return { body: [], details: [] };
  }
  return { body: [hexSlab(CELL_RADIUS * 0.96, GROUND_HEIGHT)], details: [] };
}

/** A floor: a flat slab. */
function buildFloor(): CellParts {
  return { body: [hexSlab(CELL_RADIUS * 0.96, GROUND_HEIGHT)], details: [] };
}

/** A garden: a flat slab with a low kerb. */
function buildGarden(): CellParts {
  const body: THREE.BufferGeometry[] = [
    hexSlab(CELL_RADIUS * 0.96, GROUND_HEIGHT),
  ];

  // A low kerb around the plot, so a garden reads as a garden.
  for (let i = 0; i < 6; i++) {
    body.push(faceBox(i, CELL_RADIUS * 0.9, 0.1, 0.12, CELL_RADIUS * 0.96));
  }

  return { body, details: [] };
}

/** Stairs: a flight of steps rising to the north. */
function buildStairs(): CellParts {
  const body: THREE.BufferGeometry[] = [];
  const steps = 4;
  for (let i = 0; i < steps; i++) {
    const step = new THREE.BoxGeometry(
      CELL_RADIUS * 1.1,
      0.12,
      CELL_RADIUS * 0.4,
    );
    step.translate(
      0,
      0.06 + i * 0.12,
      CELL_RADIUS * 0.7 - i * CELL_RADIUS * 0.35,
    );
    body.push(step);
  }
  return { body, details: [] };
}

/**
 * A wall.
 *
 * The wall draws a face on each side that meets another wall. The faces
 * are the full width of the cell, so a run of walls forms a continuous
 * surface.
 *
 * A window is a wall with a glazed panel on its north face.
 */
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

  if (glazed && links.linked[0]) {
    // The window faces north. The pane sits just inside the wall face.
    const pane = detailBox(WINDOW_WIDTH, WINDOW_HEIGHT, 0.06);
    pane.translate(0, STOREY * 0.55, 0);
    const z = -CELL_RADIUS * Math.cos(Math.PI / 6);
    pane.translate(0, 0, z + WINDOW_INSET);
    details.push(pane);
  }

  return { body, details };
}

/**
 * An arch: a passage through a wall.
 *
 * The arch's wall faces north and south, so it spans two cells along
 * that axis. The opening runs through the cell from side to side, with a
 * pier on each side and a lintel over the top.
 *
 * The passage is drawn with the same walls that link to the neighbours, so
 * an arch between two walls reads as part of the building.
 */
function buildArch(links: NeighbourLinks): CellParts {
  const body: THREE.BufferGeometry[] = [];

  const width = CELL_RADIUS * Math.sqrt(3);
  const pierWidth = width * 0.26;

  // The piers stand on either side of the passage.
  for (const side of [-1, 1]) {
    const pier = faceBox(0, pierWidth, STOREY, WALL_THICKNESS);
    pier.translate(side * (width - pierWidth) * 0.5, 0, 0);
    body.push(pier);
  }

  // The lintel closes the top of the passage.
  const lintel = faceBox(0, width, 0.36, WALL_THICKNESS);
  lintel.translate(0, STOREY - 0.36, 0);
  body.push(lintel);

  // The arch also faces whatever it links to on its other sides.
  for (let i = 0; i < 6; i++) {
    if (i === 0 || i === 3) {
      continue;
    }
    if (!links.linked[i]) {
      continue;
    }
    body.push(faceBox(i, width, STOREY, WALL_THICKNESS));
  }

  return { body, details: [] };
}

/**
 * A tower: a solid block that rises above the storey.
 *
 * A tower stands on the ground and supports a roof, so it is drawn full
 * height whatever it touches.
 */
function buildTower(): CellParts {
  return { body: [hexPrism(CELL_RADIUS * 0.86, STOREY)], details: [] };
}

/**
 * A spire: a tower with a conical roof.
 */
function buildSpire(): CellParts {
  const body: THREE.BufferGeometry[] = [
    hexPrism(CELL_RADIUS * 0.86, STOREY),
    spire(CELL_RADIUS * 0.8, STOREY * 0.9),
  ];
  return { body, details: [] };
}

/**
 * A roof.
 *
 * A pitched roof is drawn over the building below it. The edge of a roof
 * is the band that overhangs the wall below, so it is drawn as a flat
 * lip that is a little wider than the cell.
 *
 * The roof's ridge runs along the axis that the roof edges face, so a row
 * of pitched roofs forms a continuous ridge.
 */
function buildRoof(edge: boolean): CellParts {
  const body: THREE.BufferGeometry[] = [];

  if (edge) {
    // The edge of a roof: a flat band that overhangs the wall below.
    const slab = hexSlab(CELL_RADIUS * 1.02, 0.2);
    slab.translate(0, STOREY, 0);
    body.push(slab);
  } else {
    // The pitched part of the roof.
    const roof = roofWedge(CELL_RADIUS * 0.98, 0.66);
    roof.translate(0, STOREY, 0);
    body.push(roof);
  }

  return { body, details: [] };
}

/**
 * Merges a cell's parts into a single geometry.
 *
 * Returns undefined when there is nothing to draw.
 */
export function mergeParts(
  parts: THREE.BufferGeometry[],
): THREE.BufferGeometry | undefined {
  if (parts.length === 0) {
    return undefined;
  }

  if (parts.length === 1) {
    return parts[0];
  }

  const merged = mergeGeometries(parts, false);
  if (merged === null) {
    return undefined;
  }

  for (const part of parts) {
    part.dispose();
  }
  return merged;
}
