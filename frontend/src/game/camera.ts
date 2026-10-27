import * as THREE from "three";

/**
 * The orbit camera.
 *
 * The camera orbits a focus point above the grid. Left-drag orbits, right-drag
 * or shift-drag pans, and the wheel zooms. All three are damped so the
 * movement feels soft rather than twitchy.
 */
export class Camera {
  /** The camera being driven. */
  public readonly camera: THREE.PerspectiveCamera;

  /** The point the camera orbits. */
  public readonly target = new THREE.Vector3();

  private readonly canvas: HTMLCanvasElement;

  private distance = 24;
  private targetDistance = 24;

  private azimuth = Math.PI / 4;
  private targetAzimuth = Math.PI / 4;

  private polar = Math.PI / 5;
  private targetPolar = Math.PI / 5;

  private readonly minDistance = 6;
  private readonly maxDistance = 80;
  private readonly minPolar = 0.15;
  private readonly maxPolar = Math.PI / 2.6;

  private dragging: "orbit" | "pan" | null = null;
  private lastX = 0;
  private lastY = 0;

  constructor(canvas: HTMLCanvasElement, camera: THREE.PerspectiveCamera) {
    this.canvas = canvas;
    this.camera = camera;

    this.attach();
    this.apply();
  }

  /**
   * Frames the camera on a hex island of the given radius.
   *
   * The camera starts high enough to see the whole island and close enough
   * that the cells are readable.
   */
  public frameIsland(radius: number): void {
    const span = radius * 2 + 4;
    this.targetDistance = Math.min(Math.max(span * 1.6, 12), 70);
    this.distance = this.targetDistance;
    this.targetPolar = Math.PI / 4.2;
    this.polar = this.targetPolar;
    this.target.set(0, 0, 0);
    this.target.y = 0;
    this.apply();
  }

  /** Attaches the pointer and wheel handlers. */
  private attach(): void {
    this.canvas.addEventListener("pointerdown", this.onPointerDown);
    this.canvas.addEventListener("pointermove", this.onPointerMove);
    this.canvas.addEventListener("pointerup", this.onPointerUp);
    this.canvas.addEventListener("pointercancel", this.onPointerUp);
    this.canvas.addEventListener("contextmenu", this.onContextMenu);
    this.canvas.addEventListener("wheel", this.onWheel, { passive: false });
  }

  /** Starts a drag. */
  private readonly onPointerDown = (event: PointerEvent): void => {
    this.dragging = event.button === 2 || event.shiftKey ? "pan" : "orbit";
    this.lastX = event.clientX;
    this.lastY = event.clientY;
    this.canvas.setPointerCapture(event.pointerId);
  };

  /** Continues a drag. */
  private readonly onPointerMove = (event: PointerEvent): void => {
    if (this.dragging === null) {
      return;
    }

    const dx = event.clientX - this.lastX;
    const dy = event.clientY - this.lastY;
    this.lastX = event.clientX;
    this.lastY = event.clientY;

    if (this.dragging === "orbit") {
      this.targetAzimuth -= dx * 0.005;
      this.targetPolar = clamp(
        this.targetPolar - dy * 0.005,
        this.minPolar,
        this.maxPolar,
      );
    } else {
      this.pan(dx, dy);
    }
  };

  /** Ends a drag. */
  private readonly onPointerUp = (event: PointerEvent): void => {
    this.dragging = null;
    if (this.canvas.hasPointerCapture(event.pointerId)) {
      this.canvas.releasePointerCapture(event.pointerId);
    }
  };

  /** Suppresses the right-click menu so the right button can pan. */
  private readonly onContextMenu = (event: MouseEvent): void => {
    event.preventDefault();
  };

  /** Zooms on the wheel. */
  private readonly onWheel = (event: WheelEvent): void => {
    event.preventDefault();
    const factor = Math.exp(event.deltaY * 0.001);
    this.targetDistance = clamp(
      this.targetDistance * factor,
      this.minDistance,
      this.maxDistance,
    );
  };

  /** Moves the focus point across the ground plane. */
  private pan(dx: number, dy: number): void {
    const scale = this.distance * 0.002;

    // The pan direction is relative to the camera's current azimuth, so
    // dragging always moves the world with the pointer.
    const right = new THREE.Vector3(
      Math.cos(this.azimuth),
      0,
      -Math.sin(this.azimuth),
    );
    const forward = new THREE.Vector3(
      Math.sin(this.azimuth),
      0,
      Math.cos(this.azimuth),
    );

    this.target.addScaledVector(right, -dx * scale);
    this.target.addScaledVector(forward, -dy * scale);
  }

  /**
   * Advances the damping and repositions the camera.
   *
   * @param dt the time since the last frame, in seconds.
   */
  public update(dt: number): void {
    const smoothing = 1 - Math.exp(-12 * dt);

    this.azimuth += (this.targetAzimuth - this.azimuth) * smoothing;
    this.polar += (this.targetPolar - this.polar) * smoothing;
    this.distance += (this.targetDistance - this.distance) * smoothing;

    this.apply();
  }

  /** Positions the camera from the current orbit parameters. */
  private apply(): void {
    const sinPolar = Math.sin(this.polar);

    this.camera.position.set(
      this.target.x + this.distance * sinPolar * Math.sin(this.azimuth),
      this.target.y + this.distance * Math.cos(this.polar),
      this.target.z + this.distance * sinPolar * Math.cos(this.azimuth),
    );
    this.camera.lookAt(this.target);
  }

  /** Resizes the camera to match the canvas. */
  public resize(width: number, height: number): void {
    this.camera.aspect = width / Math.max(height, 1);
    this.camera.updateProjectionMatrix();
  }

  /** Moves the focus point to a world position. */
  public focusOn(point: THREE.Vector3): void {
    this.target.set(point.x, 0, point.z);
  }
}

/** Clamps a value to a range. */
function clamp(value: number, min: number, max: number): number {
  if (value < min) {
    return min;
  }
  if (value > max) {
    return max;
  }
  return value;
}
