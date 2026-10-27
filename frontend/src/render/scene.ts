import * as THREE from "three";

import type { TownState } from "../game/state";
import {
  AMBIENT_COLOR,
  FOG_COLOR,
  SKY_COLOR,
  SUN_COLOR,
  WATER_COLOR,
} from "./palette";
import { TownRenderer } from "../game/geometry/tiles";
import { Water } from "../game/geometry/water";

/**
 * The Three.js scene: lights, water, and the town itself.
 */
export class Scene {
  /** The camera the scene is rendered from. */
  public readonly camera: THREE.PerspectiveCamera;

  private readonly renderer: THREE.WebGLRenderer;
  private readonly scene: THREE.Scene;
  private readonly water: Water;
  private readonly town: TownRenderer;

  private width = 1;
  private height = 1;
  private readonly resizeObserver: ResizeObserver;

  constructor(canvas: HTMLCanvasElement) {
    this.renderer = new THREE.WebGLRenderer({
      canvas,
      antialias: true,
      powerPreference: "high-performance",
    });
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    this.renderer.shadowMap.enabled = true;
    this.renderer.shadowMap.type = THREE.PCFSoftShadowMap;
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping;
    this.renderer.toneMappingExposure = 1.05;

    this.scene = new THREE.Scene();
    this.scene.background = new THREE.Color(SKY_COLOR);
    this.scene.fog = new THREE.Fog(FOG_COLOR, 40, 140);

    this.camera = new THREE.PerspectiveCamera(45, 1, 0.1, 500);
    this.camera.position.set(20, 20, 20);

    this.addLights();
    this.water = new Water();
    this.scene.add(this.water.mesh);

    this.town = new TownRenderer();

    // The canvas may not have been laid out when the scene is created, so
    // the size is tracked with an observer rather than read once.
    this.resizeObserver = new ResizeObserver(() => this.resize());
    this.resizeObserver.observe(canvas);

    this.resize();
    window.addEventListener("resize", () => this.resize());
  }

  /** Adds the sun, the ambient light and the hemisphere light. */
  private addLights(): void {
    const sun = new THREE.DirectionalLight(SUN_COLOR, 2.1);
    sun.position.set(18, 30, 12);
    sun.castShadow = true;
    sun.shadow.mapSize.set(2048, 2048);
    sun.shadow.camera.near = 1;
    sun.shadow.camera.far = 120;
    sun.shadow.camera.left = -40;
    sun.shadow.camera.right = 40;
    sun.shadow.camera.top = 40;
    sun.shadow.camera.bottom = -40;
    sun.shadow.bias = -0.0008;
    this.scene.add(sun);
    this.scene.add(sun.target);

    const ambient = new THREE.AmbientLight(AMBIENT_COLOR, 1.1);
    this.scene.add(ambient);

    const hemi = new THREE.HemisphereLight(SKY_COLOR, 0x8a9a8f, 0.7);
    this.scene.add(hemi);
  }

  /** Adds the town to the scene. */
  public addTown(town: TownRenderer): void {
    this.scene.add(town.group);
  }

  /** Adds an object to the scene. */
  public add(object: THREE.Object3D): void {
    this.scene.add(object);
  }

  /**
   * Rebuilds the town geometry from the mirrored state.
   */
  public update(_dt: number, state: TownState): void {
    this.town.rebuild(state);
    if (this.town.group.parent !== this.scene) {
      this.scene.add(this.town.group);
    }
    if (this.town.ground.parent !== this.scene) {
      this.scene.add(this.town.ground);
    }
    this.water.update(_dt);
  }

  /** The number of chunks currently rendered. */
  public get chunkCount(): number {
    return this.town.chunkCount;
  }

  /** The number of cells currently rendered. */
  public get cellCount(): number {
    return this.town.cellCount;
  }

  /** Renders a single frame. */
  public render(): void {
    this.renderer.render(this.scene, this.camera);
  }

  /** Resizes the renderer and camera to match the canvas. */
  public resize(): void {
    const canvas = this.renderer.domElement;
    this.width = canvas.clientWidth || window.innerWidth;
    this.height = canvas.clientHeight || window.innerHeight;

    this.renderer.setSize(this.width, this.height, false);
    this.camera.aspect = this.width / Math.max(this.height, 1);
    this.camera.updateProjectionMatrix();
  }

  /** The water colour, exposed for the ground material. */
  public static get waterColor(): number {
    return WATER_COLOR;
  }
}
