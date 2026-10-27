import * as THREE from "three";
import { mergeGeometries } from "three/examples/jsm/utils/BufferGeometryUtils.js";

/**
 * The geometry of the decorative props.
 *
 * A prop is built from parts and merged into one mesh, so a tree is a
 * single draw call. The parts are split by material: a tree has a trunk
 * and leaves, which are different colours.
 *
 * The shapes are deliberately simple and low-poly: they are small on
 * screen, and a town of a few thousand cells adds up.
 */

/** The parts of a prop's geometry. */
export interface PropParts {
  /** The main body: the trunk, the post, the barrel. */
  readonly body: THREE.BufferGeometry[];
  /** The details: the leaves, the flowers, the light. */
  readonly details: THREE.BufferGeometry[];
}

/**
 * Builds the geometry of a prop.
 *
 * @param id the prop's id, e.g. "tree".
 * @param height the prop's height in cells.
 */
export function buildProp(id: string, height: number): PropParts {
  switch (id) {
    case "tree":
      return buildTree(height);
    case "shrub":
      return buildShrub(height);
    case "flower":
      return buildFlower(height);
    case "lamp":
      return buildLamp(height);
    case "barrel":
      return buildBarrel(height);
    case "rock":
      return buildRock(height);
    default:
      return { body: [], details: [] };
  }
}

/** A tree: a trunk with a canopy on top. */
function buildTree(height: number): PropParts {
  const body: THREE.BufferGeometry[] = [];
  const details: THREE.BufferGeometry[] = [];

  const trunkHeight = height * 0.42;
  const trunk = new THREE.CylinderGeometry(0.045, 0.07, trunkHeight, 6);
  trunk.translate(0, trunkHeight / 2, 0);
  body.push(trunk);

  // The canopy is two stacked cones, which reads as a rounded crown.
  const canopyHeight = height - trunkHeight;
  const lower = new THREE.ConeGeometry(height * 0.3, canopyHeight * 0.68, 6);
  lower.translate(0, trunkHeight + canopyHeight * 0.34, 0);
  details.push(lower);

  const upper = new THREE.ConeGeometry(height * 0.2, canopyHeight * 0.6, 6);
  upper.translate(0, trunkHeight + canopyHeight * 0.68, 0);
  details.push(upper);

  return { body, details };
}

/** A shrub: a low, rounded blob. */
function buildShrub(height: number): PropParts {
  const details: THREE.BufferGeometry[] = [];

  const blob = new THREE.IcosahedronGeometry(height * 0.5, 0);
  blob.translate(0, height * 0.45, 0);
  details.push(blob);

  return { body: [], details };
}

/** A flower: a short stem with a bloom on top. */
function buildFlower(height: number): PropParts {
  const body: THREE.BufferGeometry[] = [];
  const details: THREE.BufferGeometry[] = [];

  const stem = new THREE.CylinderGeometry(0.012, 0.016, height * 0.7, 4);
  stem.translate(0, height * 0.35, 0);
  body.push(stem);

  const bloom = new THREE.IcosahedronGeometry(height * 0.3, 0);
  bloom.translate(0, height * 0.82, 0);
  details.push(bloom);

  return { body, details };
}

/** A lamp: a post with a lantern on top. */
function buildLamp(height: number): PropParts {
  const body: THREE.BufferGeometry[] = [];
  const details: THREE.BufferGeometry[] = [];

  const post = new THREE.CylinderGeometry(0.028, 0.038, height * 0.82, 6);
  post.translate(0, height * 0.41, 0);
  body.push(post);

  // The lantern is a small box, which catches the light.
  const lantern = new THREE.BoxGeometry(height * 0.2, height * 0.22, height * 0.2);
  lantern.translate(0, height * 0.9, 0);
  details.push(lantern);

  return { body, details };
}

/** A barrel: a short, slightly tapered cylinder. */
function buildBarrel(height: number): PropParts {
  const body: THREE.BufferGeometry[] = [];

  const barrel = new THREE.CylinderGeometry(
    height * 0.3,
    height * 0.26,
    height * 0.9,
    8,
  );
  barrel.translate(0, height * 0.45, 0);
  body.push(barrel);

  return { body, details: [] };
}

/** A rock: a low, angular lump. */
function buildRock(height: number): PropParts {
  const details: THREE.BufferGeometry[] = [];

  const rock = new THREE.DodecahedronGeometry(height * 0.5, 0);
  rock.scale(1, 0.65, 1);
  rock.translate(0, height * 0.3, 0);
  details.push(rock);

  return { body: [], details };
}

/**
 * Merges a prop's parts into a single geometry.
 *
 * Returns undefined when there is nothing to draw.
 */
export function mergePropParts(
  parts: readonly THREE.BufferGeometry[],
): THREE.BufferGeometry | undefined {
  if (parts.length === 0) {
    return undefined;
  }

  if (parts.length === 1) {
    return parts[0];
  }

  const merged = mergeGeometries([...parts], false);
  if (merged === null) {
    return undefined;
  }

  for (const part of parts) {
    part.dispose();
  }
  return merged;
}
