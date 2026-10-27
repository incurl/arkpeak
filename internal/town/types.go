// Package town owns ArkPeak's authoritative town state and exposes it to the
// frontend through the Wails service API.
//
// The town delegates generation to the wfc package: this package owns the
// grid, the player constraints, and the saved state, while wfc owns the
// solving algorithm and the tile rules.
package town

import (
	"github.com/incurl/arkpeak/internal/wfc"
)

// Cell is a single settled cell in the town.
//
// The JSON tags here are mirrored in the generated TypeScript bindings, and
// re-exported from frontend/src/types.ts.
type Cell struct {
	// Q is the axial hex coordinate of the cell.
	Q int `json:"q"`
	// R is the axial hex coordinate of the cell.
	R int `json:"r"`
	// Layer is the vertical axis: 0 is the ground, 1 is the storey above.
	Layer int `json:"layer"`
	// Tile is the settled tile type, e.g. "wall" or "roof".
	Tile string `json:"tile"`
	// Color is the player-chosen colour, or empty for the default.
	Color string `json:"color"`
}

// Key returns the cell's position as a solver key.
func (c Cell) Key() wfc.CellKey {
	return wfc.CellKey{Q: c.Q, R: c.R, Layer: c.Layer}
}

// GridDiff describes exactly what changed in the town.
//
// The frontend applies a diff to rebuild only the affected render chunks
// rather than redrawing the whole town.
type GridDiff struct {
	// Settled lists cells that were added or changed.
	Settled []Cell `json:"settled"`
	// Removed lists cells that were cleared.
	Removed []Cell `json:"removed"`
	// Seed is the town's seed, included so the frontend can correlate
	// diffs with the town they belong to.
	Seed int64 `json:"seed"`
}

// TileInfo describes a tile type the player can place.
type TileInfo struct {
	// ID is the stable string identifier of the tile, e.g. "wall".
	ID string `json:"id"`
	// Layer is the layer the tile belongs to, e.g. 0 for the ground.
	Layer int `json:"layer"`
	// Edges holds the connector on each of the tile's six hex sides, keyed
	// by direction: "n", "ne", "se", "s", "sw", "nw".
	//
	// The renderer needs these to build neighbour-aware geometry: a wall
	// only draws a face towards the neighbours it actually meets, and a
	// roof only caps the cells that are roofed.
	//
	// The connectors come from the ruleset rather than being restated in
	// the frontend, so the rules stay the single source of truth.
	Edges map[string]string `json:"edges"`
	// Up is the connector the tile offers to the cell above it.
	Up string `json:"up"`
	// Down is the connector the tile requires from the cell below it.
	Down string `json:"down"`
	// Solid reports whether the tile occludes the cell above it.
	Solid bool `json:"solid"`
}

// ConnectorRule declares that two connector values may meet on a given
// axis.
//
// It mirrors the ruleset's own rule, and is sent to the frontend so that
// the renderer can decide which cells connect without restating the
// rules.
type ConnectorRule struct {
	// A is the connector on the first side of the meeting.
	A string `json:"a"`
	// B is the connector on the second side of the meeting.
	B string `json:"b"`
	// Axis is "hex" for a meeting across a hex edge, or "up" for a
	// meeting between a cell and the cell above it.
	Axis string `json:"axis"`
}

// TownSnapshot is the complete state of the town, used for the initial load.
type TownSnapshot struct {
	// Seed is the town's deterministic seed.
	Seed int64 `json:"seed"`
	// Radius is the radius of the town's hex grid, in cells.
	Radius int `json:"radius"`
	// Cells lists every settled cell in the town.
	Cells []Cell `json:"cells"`
	// Tiles lists the tile types available in the ruleset, so the frontend
	// can build its palette without hard-coding tile names.
	Tiles []string `json:"tiles"`
	// TileInfo lists the tile types together with the layer each belongs
	// to, so the frontend can offer only the tiles that are valid on the
	// layer the player is pointing at.
	TileInfo []TileInfo `json:"tileInfo"`
	// Connectors lists the connector pairs that may meet, so the renderer
	// can decide which cells connect without restating the rules.
	Connectors []ConnectorRule `json:"connectors"`
}
