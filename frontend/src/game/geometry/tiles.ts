import * as THREE from "three";

import type { TownState } from "../state";
import type { TileInfo } from "../../types";
import { colorByName } from "../../render/palette";
import { HEX_DIRECTIONS, HEX_RADIUS, hexJitter, hexToWorld } from "./grid";
import { Chunk } from "./chunks";
import { TileRules, neighbourLinks } from "./neighbours";
import type { NeighbourLinks } from "./neighbours";
import { buildCell, mergeParts } from "./buildings";

/**
 * The tile renderer.
 *
 * The WFC solver decides *what* each cell is; this module decides *how* to
 * draw it given the settled tile and its neighbours. The two are kept
 * separate so the solver stays pure and the renderer stays replaceable.
 *
 * The geometry is neighbour-aware: a wall only draws the faces that meet
 * another wall, so a building reads as a volume rather than as a stack of
 * hexagons. The connectors that decide this come from the backend's
 * ruleset, so the rules are not restated here.
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

  /** The meshes currently drawn, keyed by "q,r,layer". */
  private readonly meshes = new Map<string, THREE.Mesh>();

  /** The shared geometry for each tile and neighbour combination. */
  private readonly geometries = new Map<string, THREE.BufferGeometry>();

  /** The shared material for each tile and colour combination. */
  private readonly materials = new Map<string, THREE.Material>();

  /** The rules the geometry is built from. */
  private rules: TileRules | null = null;

  /** The radius of the island, in cells. */
  private radius = 0;

  /**
   * Sets the rules the geometry is built from.
   *
   * The rules come from the backend's ruleset, so the connectors that
   * decide which cells connect are not restated in the frontend.
   */
  public setRules(tiles: readonly TileInfo[], connectors: readonly { a: string; b: string; axis: string }[]): void {
    this.rules = new TileRules(tiles, connectors);
  }

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
   *
   * The chunk's meshes are rebuilt too, because a cell's geometry depends
   * on its neighbours, and a neighbour may have changed.
   */
  private rebuildChunk(key: string, state: TownState): void {
    const [cq, cr] = key.split(",").map(Number);

    let chunk = this.chunks.get(key);
    if (chunk === undefined) {
      chunk = new Chunk({ cq, cr });
      this.chunks.set(key, chunk);
      this.group.add(chunk.group);
    }

    chunk.rebuild(state.cellsInChunk(cq, cr));
    this.buildChunkMeshes(chunk, state);
  }

  /**
   * Builds the meshes for a chunk's cells.
   *
   * A cell that has nothing to draw, such as an upper storey over open
   * ground, has no mesh: the one it had is removed.
   */
  private buildChunkMeshes(chunk: Chunk, state: TownState): void {
    for (const cell of chunk.cellList) {
      const key = `${cell.key.q},${cell.key.r},${cell.key.layer}`;
      const mesh = this.buildMesh(cell.tile, cell.color, cell.key, state);

      const existing = this.meshes.get(key);
      if (existing !== undefined) {
        this.group.remove(existing);
        // The geometry is shared between cells, so it is not disposed
        // here; the cache owns it.
        this.meshes.delete(key);
      }

      if (mesh === undefined) {
        continue;
      }

      this.meshes.set(key, mesh);
      this.group.add(mesh);
    }
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

  /**
   * Returns the shared geometry for a tile and its neighbour pattern.
   *
   * The geometry only depends on the tile and on which sides connect, so
   * cells with the same pattern share one geometry. A town of a few
   * thousand cells has only a handful of distinct patterns.
   */
  private geometryFor(
    tile: string,
    links: NeighbourLinks,
    supported: boolean,
  ): THREE.BufferGeometry | undefined {
    const pattern = links.linked.map((c) => (c ? "1" : "0")).join("");
    const key = `${tile}:${pattern}:${supported ? "s" : "-"}`;

    let geometry = this.geometries.get(key);
    if (geometry === undefined) {
      const parts = buildCell(tile, links, supported);
      const merged = mergeParts(parts.body);
      if (merged === undefined) {
        return undefined;
      }
      geometry = merged;
      this.geometries.set(key, geometry);
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
   * Builds the mesh for a single cell.
   *
   * The cell's neighbours decide which faces it draws, so the neighbours
   * are looked up in the state before the geometry is built.
   */
  private buildMesh(
    tile: string,
    colorName: string,
    key: { q: number; r: number; layer: number },
    state: TownState,
  ): THREE.Mesh | undefined {
    const rules = this.rules;
    if (rules === null) {
      return undefined;
    }

    // The neighbours on the same layer decide the cell's faces.
    const neighbours: (string | undefined)[] = [];
    for (const [dq, dr] of HEX_DIRECTIONS) {
      neighbours.push(state.tile(key.q + dq, key.r + dr, key.layer));
    }

    const links = neighbourLinks(tile, neighbours, rules);

    // The cell below decides whether this cell is drawn at all.
    //
    // A storey is only built on top of something solid, so the upper
    // storey of a plot is not drawn unless the ground beneath it is a
    // building. Without this the whole island would be covered by a
    // sheet of roof.
    const below = state.tile(key.q, key.r, key.layer - 1);
    const belowIsSolid =
      below !== undefined && rules.isSolid(below);

    if (key.layer > 0 && !belowIsSolid) {
      return undefined;
    }

    // Whether the cell below can carry this one decides if the ground
    // underneath is drawn.
    const supported =
      key.layer > 0 &&
      below !== undefined &&
      rules.upConnect(rules.upOf(below), rules.downOf(tile));

    const geometry = this.geometryFor(tile, links, supported);
    if (geometry === undefined) {
      return undefined;
    }

    const mesh = new THREE.Mesh(geometry, this.materialFor(tile, colorName));
    mesh.castShadow = true;
    mesh.receiveShadow = true;

    // Remember which cell this mesh is, so it can be traced back to the
    // grid.
    mesh.userData.cell = { q: key.q, r: key.r, layer: key.layer };
    mesh.userData.tile = tile;

    // Position the cell, adding its deterministic jitter.
    const position = hexToWorld(key.q, key.r, key.layer);
    const jitter = hexJitter(key.q, key.r, key.layer);
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
    return this.meshes.size;
  }

  /**
   * The number of distinct geometries in the cache.
   *
   * Cells with the same tile and the same neighbour pattern share one
   * geometry, so this is far smaller than the number of cells.
   */
  public get geometryCount(): number {
    return this.geometries.size;
  }
}
