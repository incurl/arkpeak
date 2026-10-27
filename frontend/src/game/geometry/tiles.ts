import * as THREE from "three";

import type { TownState } from "../state";
import { colorByName } from "../../render/palette";
import { HEX_SIZE, hexJitter, hexToWorld, hexPrismGeometry } from "./grid";

/**
 * The procedural tile renderer.
 *
 * The WFC solver decides *what* each cell is; this module decides *how* to
 * draw it given the settled tile and its neighbours. The two are kept
 * separate so the solver stays pure and the renderer stays replaceable.
 *
 * For now every tile is drawn as a simple coloured prism. Milestone 5
 * replaces this with neighbour-aware geometry: walls with windows, roofs
 * that cap towers, arches that span two cells.
 */
export class TownRenderer {
  /** The group holding every rendered cell. */
  public readonly group = new THREE.Group();

  private readonly geometries = new Map<string, THREE.BufferGeometry>();
  private readonly meshes = new Map<string, THREE.Mesh>();

  /** Reuses a geometry for a given tile and colour, creating it on demand. */
  private geometryFor(tile: string, colorName: string): THREE.BufferGeometry {
    const key = `${tile}:${colorName}`;
    let geometry = this.geometries.get(key);
    if (geometry === undefined) {
      geometry = hexPrismGeometry(HEX_SIZE * 0.92, this.heightFor(tile));
      this.geometries.set(key, geometry);
    }
    return geometry;
  }

  /** The height of a tile's prism. */
  private heightFor(tile: string): number {
    switch (tile) {
      case "wall":
      case "window":
      case "tower":
        return 1.6;
      case "spire":
        return 2.4;
      case "roof":
      case "roof_edge":
        return 0.5;
      case "floor":
      case "garden":
      case "stairs":
        return 0.12;
      default:
        return 0.08;
    }
  }

  /** The colour a tile is drawn in. */
  private colorFor(tile: string, colorName: string): THREE.Color {
    const def = colorByName(colorName);

    switch (tile) {
      case "roof":
      case "roof_edge":
      case "spire":
        return new THREE.Color(def.solid).lerp(new THREE.Color(0x8c6a5a), 0.35);
      case "floor":
      case "stairs":
        return new THREE.Color(def.flat);
      case "garden":
        return new THREE.Color(def.flat).lerp(new THREE.Color(0x6f8f5f), 0.5);
      default:
        return new THREE.Color(def.solid);
    }
  }

  /**
   * Rebuilds the town geometry from the mirrored state.
   *
   * Only cells whose tile or colour changed are rebuilt, so a diff of a few
   * cells does not mean redrawing the whole town.
   */
  public rebuild(state: TownState): void {
    const seen = new Set<string>();

    for (const entry of state.entries()) {
      const { q, r, layer } = entry.key;
      const key = `${q},${r},${layer}`;
      seen.add(key);

      const existing = this.meshes.get(key);
      const signature = `${entry.tile}:${entry.color}`;

      if (existing !== undefined && existing.userData.signature === signature) {
        continue;
      }

      const mesh = this.buildMesh(entry.tile, entry.color, q, r, layer);
      mesh.userData.signature = signature;
      this.meshes.set(key, mesh);
      this.group.add(mesh);

      if (existing !== undefined) {
        this.group.remove(existing);
      }
    }

    // Drop cells that are no longer settled.
    for (const [key, mesh] of this.meshes) {
      if (!seen.has(key)) {
        this.group.remove(mesh);
        this.meshes.delete(key);
      }
    }
  }

  /** Builds the mesh for a single cell. */
  private buildMesh(tile: string, colorName: string, q: number, r: number, layer: number): THREE.Mesh {
    const geometry = this.geometryFor(tile, colorName);
    const material = new THREE.MeshStandardMaterial({
      color: this.colorFor(tile, colorName),
      roughness: 0.85,
      metalness: 0.0,
      flatShading: true,
    });

    const mesh = new THREE.Mesh(geometry, material);
    mesh.castShadow = true;
    mesh.receiveShadow = true;

    // Position the cell, adding its deterministic jitter.
    const position = hexToWorld(q, r, layer);
    const jitter = hexJitter(q, r, layer);
    mesh.position.set(position.x + jitter.x, position.y, position.z + jitter.y);
    mesh.rotation.y = (jitter.x * jitter.y) % (Math.PI / 3);

    return mesh;
  }
}
