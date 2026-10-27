package town

import (
	"fmt"
	"sort"

	"github.com/incurl/arkpeak/internal/wfc"
)

// DefaultGridWidth is the width of a new town in hex cells.
const DefaultGridWidth = 24

// DefaultGridHeight is the height of a new town in hex cells.
const DefaultGridHeight = 24

// DefaultSolveRadius is how many cells around a change are re-solved when
// the player places or removes a constraint.
const DefaultSolveRadius = 4

// Town is the authoritative state of a single ArkPeak town.
//
// A Town is not safe for concurrent use. The TownService serialises access
// with a mutex.
type Town struct {
	// seed is the town's deterministic seed. The same seed and the same
	// constraints always produce the same town.
	seed int64

	// width and height are the grid dimensions in hex cells.
	width  int
	height int

	// rules is the compiled tile ruleset.
	rules *wfc.Ruleset

	// cells holds every settled cell, keyed by position.
	cells map[wfc.CellKey]Cell

	// constraints holds the player's placed requirements, keyed by position.
	// These are the inputs that make a town reproducible.
	constraints map[wfc.CellKey]wfc.Constraint

	// order records the sequence in which constraints were placed. A town
	// is built incrementally, so replaying the constraints in the same
	// order reproduces it exactly.
	order []wfc.CellKey
}

// NewTown creates an empty town with the given seed and ruleset.
func NewTown(seed int64, rules *wfc.Ruleset) *Town {
	return &Town{
		seed:        seed,
		width:       DefaultGridWidth,
		height:      DefaultGridHeight,
		rules:       rules,
		cells:       make(map[wfc.CellKey]Cell),
		constraints: make(map[wfc.CellKey]wfc.Constraint),
	}
}

// Seed returns the town's seed.
func (t *Town) Seed() int64 { return t.seed }

// Rules returns the town's ruleset.
func (t *Town) Rules() *wfc.Ruleset { return t.rules }

// Size returns the town's grid dimensions.
func (t *Town) Size() (width, height int) { return t.width, t.height }

// Cells returns a copy of the town's settled cells.
func (t *Town) Cells() map[wfc.CellKey]Cell {
	out := make(map[wfc.CellKey]Cell, len(t.cells))
	for k, v := range t.cells {
		out[k] = v
	}
	return out
}

// gridKeys returns every position in the town's grid on the given layer.
//
// The grid is a rectangle in axial coordinates, which is not a true hexagon
// shape, but it is a simple and predictable region to solve.
func (t *Town) gridKeys(layer int) []wfc.CellKey {
	keys := make([]wfc.CellKey, 0, t.width*t.height)
	for q := 0; q < t.width; q++ {
		for r := 0; r < t.height; r++ {
			keys = append(keys, wfc.CellKey{Q: q, R: r, Layer: layer})
		}
	}
	return keys
}

// layerCount returns the number of layers the ruleset defines.
func (t *Town) layerCount() int {
	count := 1
	for i := 0; i < 8; i++ {
		if len(t.rules.TilesForLayer(i)) == 0 {
			break
		}
		count = i + 1
	}
	return count
}

// SolveAll solves the whole town from its current constraints.
//
// The town is solved layer by layer, from the ground up. Each layer is
// solved against the layer below it: a cell on layer 1 can only hold a tile
// that its counterpart on layer 0 is able to support. Solving the layers
// separately would let a roof float above an empty plot, so the layers are
// chained.
func (t *Town) SolveAll() error {
	previous := map[wfc.CellKey]wfc.ResultCell{}

	for layer := 0; layer < t.layerCount(); layer++ {
		keys := t.gridKeys(layer)

		solver := wfc.NewSolver(t.rules, t.seed)
		solver.SetRegion(keys)

		// Pin the layer below to what it settled on, so this layer is
		// solved in the context of what supports it.
		solver.SetBelow(tileMap(previous, t.rules))

		if err := solver.ConstraintsWithin(t.constraintList()); err != nil {
			return err
		}
		if err := solver.Solve(); err != nil {
			return fmt.Errorf("solve layer %d: %w", layer, err)
		}

		previous = solver.Result()
		t.applyResult(previous)
	}

	return nil
}

// tileMap converts a solver result into a map of pinned tiles.
func tileMap(result map[wfc.CellKey]wfc.ResultCell, rules *wfc.Ruleset) map[wfc.CellKey]wfc.TileID {
	out := make(map[wfc.CellKey]wfc.TileID, len(result))
	for key, res := range result {
		if tile, ok := rules.Lookup(res.Tile); ok {
			out[key] = tile
		}
	}
	return out
}

// Rebuild replays the town's constraints from scratch, in the order they
// were placed.
//
// A town is built incrementally: each constraint triggers a regional re-solve
// that leaves the rest of the town alone. Replaying the constraints in the
// same order therefore reproduces the town exactly, which is what makes
// loading a saved town faithful to the town that was saved.
//
// The base town is solved *without* any constraints first, because that is
// the state the town was in before the first constraint was placed.
func (t *Town) Rebuild() error {
	saved := t.constraints
	t.constraints = make(map[wfc.CellKey]wfc.Constraint)

	if err := t.SolveAll(); err != nil {
		t.constraints = saved
		return err
	}

	t.constraints = saved
	for _, key := range t.order {
		if _, err := t.resolveAround(key, DefaultSolveRadius); err != nil {
			return err
		}
	}
	return nil
}

// constraintList returns the town's constraints in a deterministic order.
func (t *Town) constraintList() []wfc.Constraint {
	out := make([]wfc.Constraint, 0, len(t.constraints))
	for _, c := range t.constraints {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Key, out[j].Key
		if a.Layer != b.Layer {
			return a.Layer < b.Layer
		}
		if a.Q != b.Q {
			return a.Q < b.Q
		}
		return a.R < b.R
	})
	return out
}

// applyResult writes a solver result into the town, preserving the player's
// colours.
func (t *Town) applyResult(result map[wfc.CellKey]wfc.ResultCell) {
	for key, res := range result {
		color := res.Color
		if color == "" {
			if existing, ok := t.cells[key]; ok {
				color = existing.Color
			}
		}
		t.cells[key] = Cell{
			Q:     key.Q,
			R:     key.R,
			Layer: key.Layer,
			Tile:  res.Tile,
			Color: color,
		}
	}
}

// PlaceConstraint places a player constraint and re-solves the affected
// region. It returns the cells that changed.
func (t *Town) PlaceConstraint(key wfc.CellKey, tile, color string) (GridDiff, error) {
	if err := t.rules.ValidateTileID(tile); err != nil {
		return GridDiff{}, err
	}
	if key.Layer < 0 || key.Layer >= t.layerCount() {
		return GridDiff{}, fmt.Errorf("layer %d is out of range (0..%d)", key.Layer, t.layerCount()-1)
	}
	if key.Q < 0 || key.Q >= t.width || key.R < 0 || key.R >= t.height {
		return GridDiff{}, fmt.Errorf("cell %+v is outside the %dx%d grid", key, t.width, t.height)
	}

	// Record the constraint, remembering the order it was placed in so the
	// town can be rebuilt exactly.
	_, replacing := t.constraints[key]
	t.constraints[key] = wfc.Constraint{Key: key, Tile: tile, Color: color}
	if !replacing {
		t.order = append(t.order, key)
	}
	return t.resolveAround(key, DefaultSolveRadius)
}

// RemoveConstraint removes a player constraint and re-solves the affected
// region. It returns the cells that changed.
func (t *Town) RemoveConstraint(key wfc.CellKey) (GridDiff, error) {
	delete(t.constraints, key)
	for i, k := range t.order {
		if k == key {
			t.order = append(t.order[:i], t.order[i+1:]...)
			break
		}
	}
	return t.resolveAround(key, DefaultSolveRadius)
}

// resolveAround re-solves the cells within radius of center and reports what
// changed.
//
// Cells outside the re-solved region are treated as fixed boundaries, so a
// local change cannot disturb the rest of the town. The layers are solved
// from the ground up, so a change on layer 0 correctly affects layer 1.
//
// The cells already in the region keep their current tile unless the change
// makes it impossible. This keeps a re-solve stable: placing a block in one
// spot does not reshuffle the whole neighbourhood.
func (t *Town) resolveAround(center wfc.CellKey, radius int) (GridDiff, error) {
	before := t.Cells()

	for layer := 0; layer < t.layerCount(); layer++ {
		keys := t.regionKeysOnLayer(center, radius, layer)

		solver := wfc.NewSolver(t.rules, t.seed)
		solver.SetRegion(keys)

		// Pin the cells outside the region to their current tile, so the
		// region solves consistently with the rest of the town. Only
		// cells on the same layer are pinned: the vertical relationship
		// is handled by SetBelow.
		for key, cell := range t.cells {
			if key.Layer != layer || solver.InRegion(key) {
				continue
			}
			if tile, ok := t.rules.Lookup(cell.Tile); ok {
				solver.FixCell(key, tile)
			}
		}

		// The layer below constrains this layer vertically.
		solver.SetBelow(t.tilesBelowLayer(layer))

		// Keep the cells that already have a tile, unless the player has
		// placed a constraint on them. The constraint is applied after,
		// so it wins.
		for key, cell := range t.cells {
			if key.Layer != layer || !solver.InRegion(key) {
				continue
			}
			if _, pinned := t.constraints[key]; pinned {
				continue
			}
			if tile, ok := t.rules.Lookup(cell.Tile); ok {
				solver.KeepCell(key, tile)
			}
		}

		if err := solver.ConstraintsWithin(t.constraintList()); err != nil {
			return GridDiff{}, err
		}
		if err := solver.Solve(); err != nil {
			return GridDiff{}, fmt.Errorf("solve layer %d: %w", layer, err)
		}

		t.applyResult(solver.Result())
	}

	return t.diff(before), nil
}

// tilesBelowLayer returns the current tiles of the layer below the given one.
func (t *Town) tilesBelowLayer(layer int) map[wfc.CellKey]wfc.TileID {
	out := make(map[wfc.CellKey]wfc.TileID)
	if layer <= 0 {
		return out
	}
	for key, cell := range t.cells {
		if key.Layer != layer-1 {
			continue
		}
		if tile, ok := t.rules.Lookup(cell.Tile); ok {
			out[key] = tile
		}
	}
	return out
}

// regionKeysOnLayer returns the cells on one layer within radius of center.
func (t *Town) regionKeysOnLayer(center wfc.CellKey, radius, layer int) []wfc.CellKey {
	var keys []wfc.CellKey
	for q := 0; q < t.width; q++ {
		for r := 0; r < t.height; r++ {
			k := wfc.CellKey{Q: q, R: r, Layer: layer}
			if wfc.InRadius(center, k, radius) {
				keys = append(keys, k)
			}
		}
	}
	return keys
}

// diff compares the town's cells with a previous snapshot and reports the
// changes.
func (t *Town) diff(before map[wfc.CellKey]Cell) GridDiff {
	diff := GridDiff{Seed: t.seed}

	for key, cell := range t.cells {
		old, existed := before[key]
		if !existed {
			diff.Settled = append(diff.Settled, cell)
			continue
		}
		if old.Tile != cell.Tile || old.Color != cell.Color {
			diff.Settled = append(diff.Settled, cell)
		}
	}

	for key, cell := range before {
		if _, still := t.cells[key]; !still {
			diff.Removed = append(diff.Removed, cell)
		}
	}

	sortCells(diff.Settled)
	sortCells(diff.Removed)
	return diff
}

// sortCells sorts cells into a deterministic order.
func sortCells(cells []Cell) {
	sort.Slice(cells, func(i, j int) bool {
		a, b := cells[i], cells[j]
		if a.Layer != b.Layer {
			return a.Layer < b.Layer
		}
		if a.Q != b.Q {
			return a.Q < b.Q
		}
		return a.R < b.R
	})
}

// Snapshot returns the full state of the town.
func (t *Town) Snapshot() TownSnapshot {
	cells := make([]Cell, 0, len(t.cells))
	for _, cell := range t.cells {
		cells = append(cells, cell)
	}
	sortCells(cells)

	return TownSnapshot{
		Seed:   t.seed,
		Width:  t.width,
		Height: t.height,
		Cells:  cells,
		Tiles:  t.rules.TileIDs(),
	}
}
