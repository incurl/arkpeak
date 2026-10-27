import * as THREE from "three";

import { Service as TownService } from "../../bindings/github.com/incurl/arkpeak/internal/town";
import type { GridDiff } from "../types";
import type { TownState } from "./state";
import type { Palette } from "../ui/palette";
import type { Hud } from "../ui/hud";
import type { Scene } from "../render/scene";
import { HEX_RADIUS, LAYER_HEIGHT, hexToWorld, inHexRegion, worldToHex } from "./geometry/grid";

/**
 * Handles mouse input: picking the cell under the cursor and placing the
 * player's chosen tile there.
 */
export class Input {
  private readonly canvas: HTMLCanvasElement;
  private readonly scene: Scene;
  private readonly state: TownState;
  private readonly palette: Palette;
  private readonly hud: Hud;
  private readonly applyDiff: (state: TownState, diff: GridDiff) => void;

  private readonly raycaster = new THREE.Raycaster();
  private readonly pointer = new THREE.Vector2();

  private readonly hoverMarker: THREE.Mesh;
  private dragging = false;
  private dragMoved = false;
  private button = 0;
  private lastX = 0;
  private lastY = 0;
  private busy = false;

  constructor(
    canvas: HTMLCanvasElement,
    scene: Scene,
    state: TownState,
    palette: Palette,
    hud: Hud,
    applyDiff: (state: TownState, diff: GridDiff) => void,
  ) {
    this.canvas = canvas;
    this.scene = scene;
    this.state = state;
    this.palette = palette;
    this.hud = hud;
    this.applyDiff = applyDiff;

    this.hoverMarker = this.buildHoverMarker();

    this.canvas.addEventListener("pointerdown", this.onPointerDown);
    this.canvas.addEventListener("pointermove", this.onPointerMove);
    this.canvas.addEventListener("pointerup", this.onPointerUp);
    this.canvas.addEventListener("pointerleave", this.onPointerLeave);
  }

  /** Builds the marker that shows the hovered cell. */
  private buildHoverMarker(): THREE.Mesh {
    const geometry = new THREE.RingGeometry(HEX_RADIUS * 0.86, HEX_RADIUS * 0.98, 6);
    geometry.rotateX(-Math.PI / 2);
    geometry.rotateY(Math.PI / 6);

    const material = new THREE.MeshBasicMaterial({
      color: 0xffffff,
      transparent: true,
      opacity: 0.7,
      depthTest: false,
    });

    const marker = new THREE.Mesh(geometry, material);
    marker.renderOrder = 999;
    marker.visible = false;
    this.scene.add(marker);
    return marker;
  }

  /** Converts a pointer event to normalised device coordinates. */
  private updatePointer(event: PointerEvent): void {
    const rect = this.canvas.getBoundingClientRect();
    this.pointer.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
    this.pointer.y = -((event.clientY - rect.top) / rect.height) * 2 + 1;
  }

  /**
   * Finds the cell under the pointer on the active layer.
   *
   * The active layer is the player's choice, so a click always lands
   * where they are building. The pointer is projected onto that layer's
   * plane, which means the ground stays reachable even when the storey
   * above is full of roofs.
   */
  private pick(): { q: number; r: number; layer: number } | null {
    const layer = this.palette.layer;

    this.raycaster.setFromCamera(this.pointer, this.scene.camera);

    // The layer's plane, in world space.
    const plane = new THREE.Plane(
      new THREE.Vector3(0, 1, 0),
      -layer * LAYER_HEIGHT,
    );
    const point = new THREE.Vector3();
    if (!this.raycaster.ray.intersectPlane(plane, point)) {
      return null;
    }

    const { q, r } = worldToHex(point);
    if (!inHexRegion(this.state.radius, q, r)) {
      return null;
    }

    return { q, r, layer };
  }

  /** Updates the hover marker. */
  private updateHover(): void {
    const hit = this.pick();
    if (hit === null) {
      this.hoverMarker.visible = false;
      return;
    }

    const position = hexToWorld(hit.q, hit.r, hit.layer);
    this.hoverMarker.position.set(position.x, position.y + 0.02, position.z);
    this.hoverMarker.visible = true;
  }

  /** Starts a click. */
  private readonly onPointerDown = (event: PointerEvent): void => {
    // The left button places tiles; the right button removes one.
    this.button = event.button;
    this.dragging = true;
    this.dragMoved = false;
    this.lastX = event.clientX;
    this.lastY = event.clientY;
  };

  /** Tracks the pointer and updates the hover marker. */
  private readonly onPointerMove = (event: PointerEvent): void => {
    if (this.dragging) {
      const moved =
        Math.abs(event.clientX - this.lastX) +
        Math.abs(event.clientY - this.lastY);
      if (moved > 4) {
        this.dragMoved = true;
      }
    }

    this.updatePointer(event);
    this.updateHover();
  };

  /** Finishes a click and places or removes a tile if the pointer did not drag. */
  private readonly onPointerUp = (event: PointerEvent): void => {
    const wasDragging = this.dragging;
    const didMove = this.dragMoved;
    const button = this.button;
    this.dragging = false;
    this.dragMoved = false;

    if (!wasDragging || didMove || button !== event.button) {
      return;
    }

    this.updatePointer(event);
    const hit = this.pick();
    if (hit === null) {
      return;
    }

    if (button === 2) {
      void this.remove(hit.q, hit.r, hit.layer);
      return;
    }

    void this.place(hit.q, hit.r, hit.layer);
  };

  /** Hides the hover marker when the pointer leaves the canvas. */
  private readonly onPointerLeave = (): void => {
    this.hoverMarker.visible = false;
  };

  /**
   * Asks the backend to place the selected tile and applies the diff.
   */
  private async place(q: number, r: number, layer: number): Promise<void> {
    if (this.busy) {
      this.hud.setStatus("Busy — wait for the current change to finish");
      return;
    }

    // The tile has to belong to the layer being built on. This is
    // checked locally, because the palette already knows the layers.
    if (!this.palette.tileFitsLayer(layer)) {
      this.hud.setStatus(
        `A ${this.palette.tile} does not belong on layer ${layer}`,
      );
      return;
    }

    // Ask the backend whether the placement makes sense before making it.
    // The solver's own errors are precise but technical, and the player
    // deserves a plain explanation.
    const [canPlace, reason] = await TownService.CanPlace(
      q,
      r,
      layer,
      this.palette.tile,
    );
    if (!canPlace) {
      this.hud.setStatus(reason || "That cannot be placed here");
      return;
    }
    this.busy = true;

    try {
      const diff = await TownService.PlaceConstraint(
        q,
        r,
        layer,
        this.palette.tile,
        this.palette.color,
      );
      this.applyDiff(this.state, diff);
      this.hud.setStatus(
        `Placed ${this.palette.tile} at ${q},${r} · ${diff.settled?.length ?? 0} cells changed`,
      );
    } catch (err) {
      this.hud.setError(String(err));
    } finally {
      this.busy = false;
    }
  }

  /**
   * Asks the backend to remove the constraint on a cell and applies the
   * diff.
   *
   * Removing a constraint hands the cell back to the solver, which may
   * settle on a different tile than the one the player placed.
   */
  private async remove(q: number, r: number, layer: number): Promise<void> {
    if (this.busy) {
      this.hud.setStatus("Busy — wait for the current change to finish");
      return;
    }
    this.busy = true;

    try {
      const diff = await TownService.RemoveConstraint(q, r, layer);
      this.applyDiff(this.state, diff);
      this.hud.setStatus(
        `Removed at ${q},${r} · ${diff.settled?.length ?? 0} cells changed`,
      );
    } catch (err) {
      this.hud.setError(String(err));
    } finally {
      this.busy = false;
    }
  }
}
