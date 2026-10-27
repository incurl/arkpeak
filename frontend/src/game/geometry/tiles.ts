import * as THREE from "three";

import type { TownState } from "../state";
import { colorByName } from "../../render/palette";
import { HEX_RADIUS, hexJitter, hexToWorld, hexPrismGeometry } from "./grid";
import { Chunk } from "./chunks";

/**
 * The tile renderer.
 *
 * The WFC solver decides *what* each cell is; this module decides *how* to
 * draw it given the settled tile and its neighbours. The two are kept
 * separate so the solver stays pure and the renderer stays replaceable.
 *
 * For Milestone 3 every tile is drawn as a simple coloured prism. Milestone
 * 5 replaces this with neighbour-aware geometry: walls with windows, roofs
 * that cap towers, arches that span two cells.
 *
 * The grid is split into chunks, so a change to a few cells only rebuilds
 * the chunks that contain them.
 */
export class TownRenderer {
  /** The group holding every rendered cell. */
  public readonly group = new THREE.Group();

  /** The group holding the island's ground. */
  public readonly ground = new THREE.Group();

  /** The chunks, keyed by their chunk coordinate. */
  private readonly chunks = new Map<string, Chunk>();

  /** The shared geometry for each tile type. */
  private readonly geometries = new Map<string, THREE.BufferGeometry>();

  /** The shared material for each tile and colour combination. */
  private readonly materials = new Map<string, THREE.Material>();

  /** The radius of the island, in cells. */
  private radius = 0;

  /**
   * Rebuilds the chunks that have changed.
   *
   * @param state the mirrored state.
   * @returns the number of chunks that were rebuilt.
   */
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

  /**
   * Rebuilds a single chunk from the state's current cells.
   */
  private rebuildChunk(key: string, state: TownState): void {
    const [cq, cr] = key.split(",").map(Number);

    let chunk = this.chunks.get(key);
    if (chunk === undefined) {
      chunk = new Chunk({ cq, cr });
      this.chunks.set(key, chunk);
      this.group.add(chunk.group);
    }

    chunk.rebuild(state.cellsInChunk(cq, cr), this.buildMesh.bind(this));
  }

  /**
   * Removes the chunks that hold no cells any more.
   */
  private dropEmptyChunks(state: TownState): void {
    for (const [key, chunk] of this.chunks) {
      if (chunk.size > 0) {
        continue;
      }
      if (state.hasCellsInChunk(key)) {
        continue;
      }
      this.group.remove(chunk.group);
      this.chunks.delete(key);
    }
  }

  /**
   * Rebuilds the island's ground.
   *
   * The ground is a single hexagon under the whole town, so the island
   * reads as a single landmass rather than a field of tiles.
   */
  private rebuildGround(): void {
    this.ground.clear();

    if (this.radius <= 0) {
      return;
    }

    const radius = this.radius * HEX_RADIUS * 1.02;
    const geometry = new THREE.CircleGeometry(radius, 6);
    geometry.rotateX(-Math.PI / 2);
    geometry.rotateY(Math.PI / 6);

    const material = new THREE.MeshStandardMaterial({
      color: 0xd7cdb6,
      roughness: 1.0,
      metalness: 0.0,
    });

    const mesh = new THREE.Mesh(geometry, material);
    mesh.receiveShadow = true;
    mesh.position.y = -0.06;
    this.ground.add(mesh);
  }

  /** Returns the shared geometry for a tile type. */
  private geometryFor(tile: string): THREE.BufferGeometry {
    let geometry = this.geometries.get(tile);
    if (geometry === undefined) {
      geometry = hexPrismGeometry(HEX_RADIUS, this.heightFor(tile));
      this.geometries.set(tile, geometry);
    }
    return geometry;
  }

  /** Returns the shared material for a tile and colour combination. */
  private materialFor(tile: string, colorName: string): THREE.Material {
    const key = `${tile}:${colorName}`;
    let material = this.materials.get(key);
    if (material === undefined) {
      material = new THREE.MeshStandardMaterial({
        color: this.colorFor(tile, colorName),
        roughness: 0.85,
        metalness: 0.0,
        flatShading: true,
      });
      this.materials.set(key, material);
    }
    return material;
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

  /** Builds the mesh for a single cell. */
  private buildMesh(
    tile: string,
    colorName: string,
    q: number,
    r: number,
    layer: number,
  ): THREE.Mesh {
    const mesh = new THREE.Mesh(
      this.geometryFor(tile),
      this.materialFor(tile, colorName),
    );
    mesh.castShadow = true;
    mesh.receiveShadow = true;

    // Position the cell, adding its deterministic jitter.
    const position = hexToWorld(q, r, layer);
    const jitter = hexJitter(q, r, layer);
    mesh.position.set(
      position.x + jitter.x,
      position.y + jitter.y,
      position.z + jitter.z,
    );

    return mesh;
  }

  /** The number of chunks currently rendered. */
  public get chunkCount(): number {
    return this.chunks.size;
  }

  /** The number of cells currently rendered. */
  public get cellCount(): number {
    let total = 0;
    for (const chunk of this.chunks.values()) {
      total += chunk.size;
    }
    return total;
  }
}
