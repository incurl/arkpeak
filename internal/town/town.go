package town

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/incurl/arkpeak/internal/wfc"
)

// DefaultGridRadius is the radius of a new town's hex grid, in cells.
//
// The grid is a hexagon: 24 cells across, which gives 3*24² + 3*24 + 1 =
// 1801 cells per layer.
const DefaultGridRadius = 24

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

	// radius is the radius of the town's hex grid, in cells.
	radius int

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
		radius:      DefaultGridRadius,
		rules:       rules,
		cells:       make(map[wfc.CellKey]Cell),
		constraints: make(map[wfc.CellKey]wfc.Constraint),
	}
}

// Seed returns the town's seed.
func (t *Town) Seed() int64 { return t.seed }

// Rules returns the town's ruleset.
func (t *Town) Rules() *wfc.Ruleset { return t.rules }

// Radius returns the radius of the town's hex grid.
func (t *Town) Radius() int { return t.radius }

// Cells returns a copy of the town's settled cells.
func (t *Town) Cells() map[wfc.CellKey]Cell {
	out := make(map[wfc.CellKey]Cell, len(t.cells))
	for k, v := range t.cells {
		out[k] = v
	}
	return out
}

// gridKeys returns every position in the town's hex grid on the given layer.
//
// The grid is a hexagon centred on the origin, so that it reads as an island
// rather than a skewed slab.
func (t *Town) gridKeys(layer int) []wfc.CellKey {
	base := wfc.HexRegion(t.radius)
	keys := make([]wfc.CellKey, len(base))
	for i, k := range base {
		keys[i] = wfc.CellKey{Q: k.Q, R: k.R, Layer: layer}
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
//
// Only a constrained cell is coloured: the colour comes from the constraint
// the player placed, and the solver carries it through the solve. A cell
// without a constraint takes the default colour, which is what a cell whose
// constraint has just been removed falls back to.
//
// A cell that is not part of the solved region is not touched at all, so
// its colour survives untouched.
func (t *Town) applyResult(result map[wfc.CellKey]wfc.ResultCell) {
	for key, res := range result {
		t.cells[key] = Cell{
			Q:     key.Q,
			R:     key.R,
			Layer: key.Layer,
			Tile:  res.Tile,
			Color: res.Color,
		}
	}
}

// CanPlace reports whether a tile could be placed on a cell, and explains
// why not.
//
// The frontend calls this before sending a placement, so the player gets a
// plain explanation rather than a solver error. The checks mirror the ones
// the solver makes: the tile must belong to the layer, and the cell below
// must be able to support it.
//
// This is a necessary check, not a sufficient one. The solver may still
// fail to satisfy a placement, because the tile also has to fit with its
// neighbours.
func (t *Town) CanPlace(key wfc.CellKey, tile string) (bool, string) {
	tileID, ok := t.rules.Lookup(tile)
	if !ok {
		return false, fmt.Sprintf("there is no %q tile", tile)
	}

	if key.Layer < 0 || key.Layer >= t.layerCount() {
		return false, fmt.Sprintf("there is no layer %d", key.Layer)
	}

	if !wfc.InHexRegion(t.radius, key) {
		return false, "that is outside the island"
	}

	// The tile must belong to the layer.
	valid := false
	for _, t2 := range t.rules.TilesForLayer(key.Layer) {
		if t2 == tileID {
			valid = true
			break
		}
	}
	if !valid {
		return false, fmt.Sprintf("a %s does not belong on layer %d", tile, key.Layer)
	}

	// The cell below must be able to support the tile. Only a tile on
	// layer 1 or above has something below it.
	if key.Layer > 0 {
		below, ok := t.cells[wfc.CellKey{Q: key.Q, R: key.R, Layer: key.Layer - 1}]
		if ok {
			lowerTile, ok := t.rules.Lookup(below.Tile)
			if ok {
				lowerUp := t.rules.UpConnector(lowerTile)
				tileDown := t.rules.DownConnector(tileID)
				if !t.rules.UpConnectorsMatch(lowerUp, tileDown) {
					return false, fmt.Sprintf("a %s needs something to stand on", tile)
				}
			}
		}
	}

	return true, ""
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
	if !wfc.InHexRegion(t.radius, key) {
		return GridDiff{}, fmt.Errorf("cell %+v is outside the %d-cell-radius grid", key, t.radius)
	}

	// Record the constraint, remembering the order it was placed in so the
	// town can be rebuilt exactly.
	_, replacing := t.constraints[key]
	previous, hadPrevious := t.constraints[key]
	t.constraints[key] = wfc.Constraint{Key: key, Tile: tile, Color: color}
	if !replacing {
		t.order = append(t.order, key)
	}

	diff, err := t.resolveAround(key, DefaultSolveRadius)
	if err != nil {
		// The placement could not be satisfied. Roll the constraint
		// back, so a failed attempt does not poison the town: it must
		// not survive into a save file or into the next re-solve.
		if hadPrevious {
			t.constraints[key] = previous
		} else {
			delete(t.constraints, key)
			t.removeFromOrder(key)
		}
		return GridDiff{}, err
	}

	return diff, nil
}

// removeFromOrder removes a key from the order the constraints were placed
// in.
func (t *Town) removeFromOrder(key wfc.CellKey) {
	for i, k := range t.order {
		if k == key {
			t.order = append(t.order[:i], t.order[i+1:]...)
			return
		}
	}
}

// RemoveConstraint removes a player constraint and re-solves the affected
// region. It returns the cells that changed.
func (t *Town) RemoveConstraint(key wfc.CellKey) (GridDiff, error) {
	delete(t.constraints, key)
	t.removeFromOrder(key)
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
	for q := -t.radius; q <= t.radius; q++ {
		for r := -t.radius; r <= t.radius; r++ {
			k := wfc.CellKey{Q: q, R: r, Layer: layer}
			if !wfc.InHexRegion(t.radius, k) {
				continue
			}
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

// layerOf converts a tile's layer declaration into a layer index.
//
// A tile declares its layer as a string, because the ruleset is data and
// the wildcard "*" means "any layer". A wildcard tile is reported as
// layer -1, which the frontend reads as "offer it everywhere".
func layerOf(layer string) int {
	if layer == "" || layer == "*" {
		return -1
	}

	n, err := strconv.Atoi(layer)
	if err != nil {
		return -1
	}
	return n
}

// Snapshot returns the full state of the town.
func (t *Town) Snapshot() TownSnapshot {
	cells := make([]Cell, 0, len(t.cells))
	for _, cell := range t.cells {
		cells = append(cells, cell)
	}
	sortCells(cells)

	// Describe every tile with the layer it belongs to, so the frontend
	// can offer only the tiles that are valid where the player clicks.
	tileIDs := t.rules.TileIDs()
	info := make([]TileInfo, 0, len(tileIDs))
	for _, id := range tileIDs {
		tileID, ok := t.rules.Lookup(id)
		if !ok {
			continue
		}
		def := t.rules.Tile(tileID)
		info = append(info, TileInfo{ID: def.ID, Layer: layerOf(def.Layer)})
	}

	return TownSnapshot{
		Seed:     t.seed,
		Radius:   t.radius,
		Cells:    cells,
		Tiles:    tileIDs,
		TileInfo: info,
	}
}
