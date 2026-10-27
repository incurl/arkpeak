import * as THREE from "three";

import { Service as TownService } from "../../bindings/github.com/incurl/arkpeak/internal/town";
import type { GridDiff } from "../types";
import type { TownState } from "./state";
import type { Palette } from "../ui/palette";
import type { Hud } from "../ui/hud";
import type { Scene } from "../render/scene";
import { HEX_RADIUS, hexToWorld, inHexRegion, worldToHex } from "./geometry/grid";

/** The number of layers the player can build on. */
const LAYERS = 2;

/** The vertical distance between layers, matching grid.ts. */
const LAYER_HEIGHT = 1.6;

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
   * Finds the cell under the pointer.
   *
   * The pointer is intersected with the ground plane of each layer, and the
   * topmost hit wins. Layers are checked from the top down so a tower is
   * picked before the ground below it.
   */
  private pick(): { q: number; r: number; layer: number } | null {
    this.raycaster.setFromCamera(this.pointer, this.scene.camera);

    for (let layer = LAYERS - 1; layer >= 0; layer--) {
      const plane = new THREE.Plane(
        new THREE.Vector3(0, 1, 0),
        -layer * LAYER_HEIGHT,
      );
      const point = new THREE.Vector3();
      if (!this.raycaster.ray.intersectPlane(plane, point)) {
        continue;
      }

      const { q, r } = worldToHex(point);
      if (!inHexRegion(this.state.radius, q, r)) {
        continue;
      }

      return { q, r, layer };
    }

    return null;
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

    // Only the tiles that belong to the hovered layer can be placed
    // there, so the palette follows the pointer.
    this.palette.setLayer(hit.layer);
  }

  /** Starts a click. */
  private readonly onPointerDown = (event: PointerEvent): void => {
    // Only the left button places tiles; the camera uses the right button.
    if (event.button !== 0) {
      return;
    }
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

  /** Finishes a click and places a tile if the pointer did not drag. */
  private readonly onPointerUp = (event: PointerEvent): void => {
    const wasDragging = this.dragging;
    const didMove = this.dragMoved;
    this.dragging = false;
    this.dragMoved = false;

    if (!wasDragging || didMove || event.button !== 0) {
      return;
    }

    this.updatePointer(event);
    const hit = this.pick();
    if (hit === null) {
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
    } catch (err) {
      this.hud.setError(String(err));
    } finally {
      this.busy = false;
    }
  }
}
