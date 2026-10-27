# Chapter 8 — Wails: a Go program with a web page inside

---

## The problem Wails solves

You want a desktop application. It should have a real window, a real
menu bar, real file dialogs, and it should install like a normal
application.

You also want to write the interface in modern web technology, because
that is where you can get a GPU-accelerated 3D scene without writing a
graphics engine.

Those two things used to be incompatible. The usual answers were:

- **Electron.** Ship a whole browser. Great for the interface, awful for
  the footprint — hundreds of megabytes, and you ship a browser's worth of
  security updates.
- **Native toolkits.** Qt, GTK, WinUI, AppKit. A real application, but
  now you are writing your renderer in four languages and your UI in
  another.
- **Games engines.** Unity and Unreal. Excellent for 3D, but you are
  writing your application logic in C# or C++, and your text handling is
  whatever the engine decided it would be.

**Wails** takes a third option: a Go binary that owns the window and
hosts a web view, with the frontend written in TypeScript and the
backend written in Go.

## The shape of it

ArkPeak's `main.go` is the whole application setup:

```go
app := application.New(application.Options{
    Name:        "ArkPeak",
    Description: "A cozy procedural town-builder",
    Services: []application.Service{
        application.NewService(service),
    },
    Assets: application.AssetOptions{
        Handler: application.AssetFileServerFS(assets),
    },
    ...
})

app.Window.NewWithOptions(application.WebviewWindowOptions{
    Title:  "ArkPeak",
    Width:  1280,
    Height: 800,
    BackgroundColour: application.NewRGB(150, 180, 190),
    URL:              "/",
})

if err := app.Run(); err != nil {
    log.Fatal(err)
}
```

That is the whole ceremony. There is no `main` function that calls into
JavaScript, and no JavaScript that calls into Go by hand. You register a
**service** and the plumbing appears.

### The three parts

**1. The service.** A plain Go struct with exported methods. Wails
inspects it, by reflection, at startup.

```go
service := town.NewService(rules, seed)
```

**2. The assets.** The frontend is compiled into `frontend/dist` and
embedded into the binary with Go's `embed`:

```go
//go:embed all:frontend/dist
var assets embed.FS
```

So the shipped application is **one file**. There is no `resources`
directory to lose, no "it works on my machine" asset path problem.

**3. The window.** A web view. On Linux it is WebKitGTK, on macOS it is
WKWebView, on Windows it is WebView2. You write HTML and CSS and it
behaves like a normal web page in a native window.

## The generated bindings

Here is the magic. You write a Go method:

```go
func (s *Service) PlaceConstraint(q, r, layer int, tileType string, color string) (GridDiff, error) {
```

You run:

```sh
wails3 generate bindings -ts -i
```

And a TypeScript file appears:

```typescript
/**
 * PlaceConstraint places a tile constraint on a cell and re-solves the
 * affected region.
 */
export function PlaceConstraint(q: number, r: number, layer: number, tileType: string, color: string): $CancellablePromise<GridDiff> {
    return $Call.ByID(3847291036, q, r, layer, tileType, color);
}
```

Note the shape of it: a normal TypeScript function, with normal types,
that returns a promise. The `3847291036` is a hash of the method's
signature — it is how the runtime knows which Go method to call.

The types are generated too. Your Go struct:

```go
type GridDiff struct {
	// Settled lists cells that were added or changed.
	Settled []Cell `json:"settled"`
	// Removed lists cells that were cleared.
	Removed []Cell `json:"removed"`
	// Seed is the town's seed, included so the frontend can correlate
	// diffs with the town they belong to.
	Seed int64 `json:"seed"`
}
```

becomes:

```typescript
export class GridDiff {
    "settled": Cell[] | null;
    "removed": Cell[] | null;
    "seed": number;
}
```

The JSON tags become the property names. The Go types become TypeScript
types. You do not write any of this by hand, and you cannot get it
out of sync.

### Why the bindings are gitignored

ArkPeak's `.gitignore` has:

```
frontend/bindings/
```

Because the generated file's *path* depends on your Go module path. If
your module is `github.com/incurl/arkpeak`, the bindings live at
`frontend/bindings/github.com/incurl/arkpeak/internal/town/`. If you fork
the project, the path changes, and a checked-in binding file would be
wrong.

So `wails3 dev` regenerates them on every start, and a fresh clone runs
`wails3 task bindings` once.

This is a small annoyance with a real benefit: **you cannot accidentally
commit stale bindings.** The bindings are always exactly as fresh as the
Go code they came from.

## Calling across the boundary

From the frontend, a service call is just an async function:

```ts
const diff = await TownService.PlaceConstraint(
  q,
  r,
  layer,
  this.palette.tile,
  this.palette.color,
);
this.applyDiff(this.state, diff);
```

That is the entire call. It looks like `fetch`, it behaves like `fetch`,
and it is a direct call into a Go method running in the same process.

### Errors cross the boundary

If the Go method returns an error, the promise rejects. ArkPeak's input
handler catches it:

```ts
try {
  const diff = await TownService.PlaceConstraint(...);
  this.applyDiff(this.state, diff);
  this.hud.setStatus(`Placed ${this.palette.tile} at ${q},${r} · ...`);
} catch (err) {
  this.hud.setError(String(err));
}
```

The error string is the Go error's `Error()` method, verbatim. So a solver
contradiction arrives in the browser as:

```
Error: RuntimeError: solve layer 1: wfc: constraints are contradictory: ...
```

Which is why Chapter 6's `CanPlace` exists. The precise error is
unreachable *as a first-class experience*, so the frontend pre-checks and
shows a plain reason instead.

## Events: the other direction

So far we have Go calling into TypeScript. The other direction — the Go
side telling the frontend something happened — uses events:

```go
app.Event.Emit("town-updated", diff)
```

ArkPeak does **not** use this, and the reason is worth understanding.

> ### State updates: pull, not push
>
> Mutating methods return a `GridDiff` describing exactly which cells
> changed, so the frontend applies it immediately without a second round
> trip. A push model (`app.Event.Emit("town-updated", diff)`) is
> available if a second window ever needs to follow the first, but a
> single window does not need it.

The reasoning: if the Go side emitted an event *and* returned a diff, the
frontend would have two paths for the same update, and they could
disagree. With pull-only, there is exactly one path.

Events earn their keep when there are **multiple observers** — a second
window, a system tray, a file watcher. ArkPeak has one window, so it
does not need them.

## Threading

Wails v3 runs the frontend in a web view, which has its own thread and
its own event loop. Go code called from the frontend runs on a goroutine
that Wails manages.

ArkPeak's service serialises access with a mutex:

```go
type Service struct {
	mu sync.Mutex

	// town is the authoritative town state.
	town *Town

	// rules is the compiled tile ruleset.
	rules *wfc.Ruleset
}
```

Every method that touches the town takes the lock:

```go
func (s *Service) PlaceConstraint(q, r, layer int, tileType string, color string) (GridDiff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	...
}
```

This is not optional. The frontend can call the service as fast as the
player clicks, and each call mutates shared state. Without the mutex, two
overlapping calls would corrupt the town.

It is also why `currentTown` is unexported:

```go
// CurrentTown returns the service's current town. It is used by the tests.
//
// It is deliberately unexported so Wails does not bind it: the Town struct
// has no exported fields, so binding it would only produce a meaningless
// empty type in the frontend.
func (s *Service) currentTown() *Town {
```

An unexported method is invisible to Wails's reflection. A small trick,
but it keeps the public API exactly as small as it should be.

## The frontend's own module system

The frontend is a normal Vite project. `frontend/package.json`:

```json
{
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vite build --mode production",
    "typecheck": "tsc --noEmit",
    "lint": "eslint src --ext .ts"
  },
  "dependencies": {
    "@wailsio/runtime": "3.0.0",
    "three": "^0.180.0"
  }
}
```

Two runtime dependencies: `@wailsio/runtime` (the bridge) and `three` (the
renderer). Everything else is dev tooling.

Vite does the bundling, TypeScript does the type-checking, ESLint does the
linting. There is no framework. The frontend is:

```
frontend/src/
  main.ts           the entry point: boot, wire, run the loop
  types.ts          re-exports the generated types
  game/
    state.ts        the mirror of the backend's grid
    camera.ts       orbit, pan, zoom
    input.ts        picking and placing
    geometry/
      grid.ts         axial coordinates, world conversion, jitter
      neighbours.ts   the compatible/linked distinction
      buildings.ts    procedural walls, windows, arches, roofs
      tiles.ts        the chunked renderer and the geometry cache
      chunks.ts       the chunk partition
      props.ts        prop selection by weight
      propShapes.ts   the prop meshes
      shapes.ts       shared primitives
      water.ts        the animated water surface
  render/
    scene.ts        the Three.js scene
    palette.ts      the colours
  ui/
    hud.ts          the status line
    palette.ts      the tile and colour buttons
```

## The type boundary, and how it stays honest

Here is the discipline that keeps the two halves from drifting apart:

**The Go structs are the source of truth.** They live in
`internal/town/types.go`, and their JSON tags are the contract.

`frontend/src/types.ts` re-exports them:

```ts
import type {
    Cell,
    GridDiff,
    PropInfo,
    TileInfo,
    TownSnapshot,
} from "../bindings/github.com/incurl/arkpeak/internal/town/models";

export type { Cell, GridDiff, PropInfo, TileInfo, TownSnapshot };
```

The rest of the frontend imports from `../types`, never from the
generated path directly. The generated path is long and
module-path-dependent; the re-export means it appears in exactly one
file.

**And there is a check that the mirror is not drifting.** In the frontend:

```ts
// The chunk partition must be exact, or cells are silently dropped. The
// check is cheap and runs once, so it happens at startup.
const chunkProblem = verifyChunkPartition();
if (chunkProblem !== null) {
  hud.setStatus(`Chunk partition is broken: ${chunkProblem}`);
  throw new Error(chunkProblem);
}
```

And for the rules:

```ts
// The geometry is built from the ruleset's connectors, so the renderer
// knows which cells connect without restating the rules.
const connectorProblem = verifyConnectorTables(
  snapshot.tileInfo ?? [],
  snapshot.connectors ?? [],
);
if (connectorProblem !== null) {
  hud.setStatus(`The ruleset is incomplete: ${connectorProblem}`);
  throw new Error(connectorProblem);
}
```

These are startup assertions. They are not unit tests — the frontend has
no test runner — but they catch the class of bug where the frontend's
understanding of the grid or the rules drifts away from the backend's.

## The build and dev loop

```sh
task dev
```

which runs:

```sh
wails3 dev -config ./build/config.yml -port {{.VITE_PORT}}
```

This starts a Vite dev server, watches for changes, rebuilds the frontend,
restarts the Go binary, and opens the window. It is a tight loop:
editing a `.go` file restarts the app; editing a `.ts` file hot-reloads
the frontend.

For a production build:

```sh
task build
```

which ends up at a plain `go build` with a few flags:

```
go build -tags production -trimpath -buildvcs=false -ldflags="-w -s" -o bin/arkpeak
```

Note `-tags production`, `-trimpath`, and `-ldflags="-w -s"`. The last
one strips the symbol table and DWARF data, which is most of why ArkPeak's
release binary is 9.7 MB rather than 30 MB. The Linux `.deb` that
`task package` produces is 4.0 MB.

The build also regenerates the icons and the desktop entry, so the
packaged application never carries a stale icon:

```yaml
deps:
  - task: common:go:mod:tidy
  - task: common:build:frontend
  - task: common:generate:icons
  - task: generate:dotdesktop
```

## What Wails is not

Two things Wails is *not*, which are worth stating because they shape
the architecture:

**It is not a browser.** The frontend cannot reach the network unless you
let it. There is no `fetch` to arbitrary URLs, no `npm install` at
runtime, no eval of remote code. For a game, that is a feature.

**It is not a single-page-app framework.** There is no router, no
component tree, no virtual DOM. You have a web view and some TypeScript.
If you want React, you can use React — but ArkPeak does not, because a
3D game with one canvas and a handful of buttons does not need one.

## A note on the Wails v3 template

If you are starting a Wails v3 project yourself, note that the template
is `vanilla`, not `vanilla-ts`:

```sh
wails3 init -n myproject -t vanilla
```

And that the runtime package is `@wailsio/runtime`, not the older
`wailsjs/runtime`. ArkPeak's `frontend/src/main.ts` imports from the
generated bindings, which import from `@wailsio/runtime`:

```ts
import { Call as $Call, CancellablePromise as $CancellablePromise } from "@wailsio/runtime";
```

---

## Summary

| Piece | What it does |
|---|---|
| `application.NewService(s)` | Registers a Go struct for the frontend to call |
| `wails3 generate bindings -ts -i` | Writes the TypeScript for the service and its types |
| `$Call.ByID(hash, ...)` | The actual bridge call |
| `//go:embed all:frontend/dist` | Puts the frontend inside the binary |
| `app.Event.Emit(name, data)` | The other direction; unused here on purpose |
| `sync.Mutex` in the service | Makes concurrent calls safe |

---

*Previous: [Chapter 7 — The solver in Go](07-solver.md) · Next: [Chapter 9 — Drawing a town in Three.js](09-rendering.md)*
