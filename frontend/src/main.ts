/**
 * ArkPeak frontend entry point.
 *
 * Boot order:
 *   1. Load the Wails bindings and ask the backend for a town.
 *   2. Build the renderer and the camera.
 *   3. Wire the palette and the input handlers.
 *   4. Start the render loop.
 */

import "./styles.css";

import { Service as TownService } from "../bindings/github.com/incurl/arkpeak/internal/town";
import type { GridDiff, TownSnapshot } from "./types";

import { Scene } from "./render/scene";
import { TownState } from "./game/state";
import { Camera } from "./game/camera";
import { Input } from "./game/input";
import { Palette } from "./ui/palette";
import { Hud } from "./ui/hud";
import { verifyChunkPartition } from "./game/geometry/chunks";

/** The canvas the game renders into. */
function requireCanvas(): HTMLCanvasElement {
  const canvas = document.getElementById("viewport");
  if (!(canvas instanceof HTMLCanvasElement)) {
    throw new Error("arkpeak: #viewport canvas is missing from the page");
  }
  return canvas;
}

/**
 * Fetches the initial town from the backend.
 *
 * The backend owns the authoritative state, so the frontend never generates
 * anything itself: it asks for a snapshot and renders what it is given.
 */
async function loadInitialTown(): Promise<TownSnapshot> {
  // A fresh town is generated on the backend with a random seed, so two
  // launches of the application produce different towns.
  const fresh = await TownService.NewTown(Date.now() % 2147483647);
  if (fresh !== null && fresh !== undefined && (fresh.cells ?? []).length > 0) {
    return fresh;
  }
  return TownService.GetSnapshot();
}

/** Applies a diff from the backend to the local mirror. */
function applyDiff(state: TownState, diff: GridDiff): void {
  state.applyDiff(diff);
}

async function main(): Promise<void> {
  const canvas = requireCanvas();
  const hud = new Hud();
  const state = new TownState();

  // The chunk partition must be exact, or cells are silently dropped. The
  // check is cheap and runs once, so it happens at startup.
  const chunkProblem = verifyChunkPartition();
  if (chunkProblem !== null) {
    hud.setStatus(`Chunk partition is broken: ${chunkProblem}`);
    throw new Error(chunkProblem);
  }

  hud.showWorking("Generating town…");

  let snapshot: TownSnapshot;
  try {
    snapshot = await loadInitialTown();
  } catch (err) {
    hud.setStatus(`Could not generate a town: ${String(err)}`);
    throw err;
  }

  state.load(snapshot);

  const scene = new Scene(canvas);
  const camera = new Camera(canvas, scene.camera);
  camera.frameIsland(snapshot.radius);

  hud.setStatus(
    `${snapshot.cells?.length ?? 0} cells · seed ${snapshot.seed}`,
  );

  // Build the whole town once, before the first frame. After this the
  // renderer only touches the chunks that change.
  scene.update(0, state);

  // The palette drives what gets placed when the player clicks.
  const palette = new Palette(
    document.getElementById("palette")!,
    snapshot.tiles ?? [],
    snapshot.tileInfo ?? [],
  );
  palette.showLayer(0);
  const input = new Input(canvas, scene, state, palette, hud, applyDiff);

  // The render loop.
  let last = performance.now();
  const frame = (now: number): void => {
    const dt = Math.min((now - last) / 1000, 0.1);
    last = now;

    camera.update(dt);
    scene.update(dt, state);
    scene.render();

    requestAnimationFrame(frame);
  };
  requestAnimationFrame(frame);

  // Expose a small handle for debugging from the devtools console.
  Object.assign(window, { arkpeak: { state, scene, camera, palette, input } });
}

main().catch((err: unknown) => {
  console.error("arkpeak: failed to start", err);
  showStartupError(err);
});

/**
 * Surfaces a startup failure on screen.
 *
 * A blank canvas is impossible to diagnose, and the developer console is not
 * always visible, so a failure is reported where it will actually be seen.
 */
function showStartupError(err: unknown): void {
  const status = document.getElementById("hud-status");
  if (status !== null) {
    status.textContent = `Failed to start: ${String(err)}`;
    status.style.color = "#b03a2e";
  }
}
