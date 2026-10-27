import * as THREE from "three";

import { HEX_DIRECTIONS } from "./grid";

/**
 * The primitive shapes the procedural buildings are built from.
 *
 * Every shape is built in a hex cell's local space: the cell is centred on
 * the origin in x and z, and stands on y = 0.
 *
 * The hex cell is "pointy-top", so its six corners point north and south
 * and its six flat sides face north-east, east, and so on. The side facing
 * a given direction is the one opposite that direction's corner.
 */

/** The radius of a cell, matching the grid's hex size. */
export const CELL_RADIUS = 1.0;

/** The thickness of a wall panel. */
export const WALL_THICKNESS = 0.16;

/** The height of a storey. */
export const STOREY_HEIGHT = 1.6;

/**
 * The unit vector pointing towards a hex side.
 *
 * Direction 0 is north. The side facing direction d is perpendicular to
 * the direction, so the sides are at 30°, 90°, 150° and so on around the
 * cell.
 */
export function sideDirection(index: number): THREE.Vector3 {
  // The first side faces north-east, and each following side turns by 60
  // degrees clockwise.
  const angle = (index * Math.PI) / 3 + Math.PI / 6;
  return new THREE.Vector3(Math.sin(angle), 0, -Math.cos(angle));
}

/**
 * The centre of a hex cell's side facing the given direction index.
 *
 * The side's midpoint lies at the apothem, which is the radius times
 * cos(30°).
 */
export function sideCentre(index: number, radius = CELL_RADIUS): THREE.Vector3 {
  return sideDirection(index).multiplyScalar(radius * Math.cos(Math.PI / 6));
}

/**
 * Builds a box that forms one face of a cell.
 *
 * The box is oriented to face the given direction and centred on that
 * side of the cell.
 */
export function faceBox(
  index: number,
  width: number,
  height: number,
  thickness: number,
  radius = CELL_RADIUS,
): THREE.BufferGeometry {
  const geometry = new THREE.BoxGeometry(width, height, thickness);
  geometry.translate(0, height / 2, 0);

  // The box faces along z by default; turn it to face the given side.
  const angle = (index * Math.PI) / 3 + Math.PI / 6;
  geometry.rotateY(angle);

  const centre = sideCentre(index, radius);
  geometry.translate(centre.x, 0, centre.z);
  return geometry;
}

/**
 * Builds a flat hexagonal slab, used for floors and roof surfaces.
 */
export function hexSlab(
  radius: number,
  height: number,
): THREE.BufferGeometry {
  const geometry = new THREE.CylinderGeometry(radius, radius, height, 6, 1, false);
  geometry.rotateY(Math.PI / 6);
  geometry.translate(0, height / 2, 0);
  return geometry;
}

/**
 * Builds a hexagonal prism.
 */
export function hexPrism(
  radius: number,
  height: number,
): THREE.BufferGeometry {
  return hexSlab(radius, height);
}

/**
 * Builds a wedge: a roof that slopes down from one side to the other.
 *
 * The wedge spans the whole cell and is highest along the ridge, which
 * runs between the two opposite corners.
 */
export function roofWedge(
  radius: number,
  height: number,
): THREE.BufferGeometry {
  // A wedge is a box whose top face is pulled down to a ridge line that
  // runs along the x axis through the middle of the cell.
  const halfWidth = radius * Math.sqrt(3);
  const halfDepth = radius;
  const geometry = new THREE.BoxGeometry(halfWidth, height, halfDepth * 2, 1, 1, 1);

  const position = geometry.attributes.position as THREE.BufferAttribute;

  for (let i = 0; i < position.count; i++) {
    // Only the top face is reshaped; the bottom stays where it is.
    if (position.getY(i) <= 0) {
      continue;
    }

    // How far the vertex is from the ridge, as a fraction of the half
    // depth: 0 at the ridge, 1 at the eaves.
    const z = position.getZ(i);
    const away = Math.abs(z) / halfDepth;

    // The eaves come in slightly, so the roof has an overhang.
    position.setZ(i, z * (1 - (1 - away) * 0.12));

    // The ridge is at the full height; the eaves sit at the bottom.
    position.setY(i, (height / 2) * (1 - away));
  }

  position.needsUpdate = true;
  geometry.computeVertexNormals();
  return geometry;
}

/**
 * Builds a cone, used for spires.
 */
export function spire(
  radius: number,
  height: number,
): THREE.BufferGeometry {
  const geometry = new THREE.ConeGeometry(radius, height, 6, 1, false);
  geometry.rotateY(Math.PI / 6);
  geometry.translate(0, height / 2, 0);
  return geometry;
}

/**
 * Builds a small box, used for windows, doors and other details.
 */
export function detailBox(
  width: number,
  height: number,
  depth: number,
): THREE.BufferGeometry {
  return new THREE.BoxGeometry(width, height, depth);
}

/**
 * The six neighbour offsets of a hex cell, in the same order as
 * HEX_DIRECTIONS.
 */
export { HEX_DIRECTIONS };
