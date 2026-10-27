package wfc

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// testRuleset is a small, explicit ruleset used by the solver tests.
//
// Connector values:
//   - "any":    matches any neighbour (empty plots, floors, gardens)
//   - "wall":   a solid wall segment; meets other wall segments
//   - "window": a glazed wall segment; meets wall and window segments
//   - "roof":   a roof surface; meets other roof surfaces
//   - "nothing": matches nothing (a roof cannot support anything)
//
// The tiles are:
//   - empty:  indifferent on every edge, supports nothing
//   - wall:   wall connector on every edge, supports a wall or a tower
//   - window: window connector facing north, wall connector elsewhere
//   - roof:   roof connector on every edge, supports nothing
//   - tower:  layer 1 only, wall connectors, needs a wall below
func testRuleset(t *testing.T) *Ruleset {
	t.Helper()

	anyEdges := map[string]string{
		"n": ConnectorAny, "ne": ConnectorAny, "se": ConnectorAny,
		"s": ConnectorAny, "sw": ConnectorAny, "nw": ConnectorAny,
	}
	wallEdges := map[string]string{
		"n": "wall", "ne": "wall", "se": "wall", "s": "wall", "sw": "wall", "nw": "wall",
	}
	windowEdges := map[string]string{
		"n": "window", "ne": "wall", "se": "wall", "s": "wall", "sw": "wall", "nw": "wall",
	}
	roofEdges := map[string]string{
		"n": "roof", "ne": "roof", "se": "roof", "s": "roof", "sw": "roof", "nw": "roof",
	}

	schema := RulesetSchema{
		Tiles: []TileDef{
			{ID: "empty", Weight: 10, Layer: "0", Edges: anyEdges, Up: ConnectorNothing, Down: ConnectorAny},
			{ID: "wall", Weight: 10, Layer: "0", Edges: wallEdges, Up: "wall", Down: ConnectorAny},
			{ID: "window", Weight: 10, Layer: "0", Edges: windowEdges, Up: ConnectorNothing, Down: ConnectorAny},
			{ID: "roof", Weight: 10, Layer: "0", Edges: roofEdges, Up: ConnectorNothing, Down: ConnectorAny},
			{ID: "tower", Weight: 10, Layer: "1", Edges: wallEdges, Up: ConnectorNothing, Down: "wall"},
		},
		Connectors: []ConnectorRule{
			// Hex adjacency. The connector names carry the meaning, so
			// "window" may meet "wall" but a roof only meets a roof.
			{A: "wall", B: "wall", Axis: AxisHex},
			{A: "wall", B: "window", Axis: AxisHex},
			{A: "roof", B: "roof", Axis: AxisHex},
			// Vertical adjacency: what a tile offers upwards must meet
			// what the tile above it requires downwards.
			{A: "wall", B: "wall", Axis: AxisUp},
			{A: "wall", B: "tower", Axis: AxisUp},
		},
	}

	rules, err := CompileRuleset(schema)
	if err != nil {
		t.Fatalf("CompileRuleset: %v", err)
	}
	return rules
}

// rectRegion builds a rectangular block of hex cells of the given size,
// starting at the origin.
func rectRegion(width, height, layer int) []CellKey {
	keys := make([]CellKey, 0, width*height)
	for q := 0; q < width; q++ {
		for r := 0; r < height; r++ {
			keys = append(keys, CellKey{Q: q, R: r, Layer: layer})
		}
	}
	return keys
}

// assertValidConfiguration checks the fundamental invariant of a WFC
// solution: every pair of adjacent cells must be compatible.
func assertValidConfiguration(t *testing.T, rules *Ruleset, result map[CellKey]ResultCell) {
	t.Helper()

	for key, cell := range result {
		for d := Direction(0); d < dirCount; d++ {
			n := key.Neighbor(d)
			other, ok := result[n]
			if !ok {
				continue
			}

			a, okA := rules.Lookup(cell.Tile)
			b, okB := rules.Lookup(other.Tile)
			if !okA || !okB {
				t.Fatalf("result contains unknown tile %q or %q", cell.Tile, other.Tile)
			}

			// The facing connectors must match. b lies in direction d
			// from a, so a's facing edge is its d edge and b's facing
			// edge is d's opposite.
			aConn := rules.EdgeConnector(a, d)
			bConn := rules.EdgeConnector(b, d.Opposite())
			if !rules.HexConnectorsMatch(aConn, bConn) {
				t.Errorf("cells %+v (%s) and %+v (%s) have incompatible connectors: %q vs %q",
					key, cell.Tile, n, other.Tile, aConn, bConn)
			}
		}

		// The cell above must be compatible.
		above, ok := result[key.Above()]
		if !ok {
			continue
		}
		lower, _ := rules.Lookup(cell.Tile)
		upper, _ := rules.Lookup(above.Tile)
		if !rules.UpConnectorsMatch(rules.UpConnector(lower), rules.DownConnector(upper)) {
			t.Errorf("cell %+v (%s) is not supported by %+v (%s)",
				key, cell.Tile, key.Above(), above.Tile)
		}
	}
}

// TestSolve5x5WithConstraints is the Milestone 2 acceptance test: a small
// grid with a few constraints must solve to a valid configuration that
// honours every constraint.
func TestSolve5x5WithConstraints(t *testing.T) {
	rules := testRuleset(t)
	solver := NewSolver(rules, 42)

	solver.SetRegion(rectRegion(5, 5, 0))

	constraints := []Constraint{
		{Key: CellKey{Q: 2, R: 2, Layer: 0}, Tile: "wall", Color: "red"},
		{Key: CellKey{Q: 0, R: 0, Layer: 0}, Tile: "empty"},
		{Key: CellKey{Q: 4, R: 4, Layer: 0}, Tile: "roof"},
	}
	if err := solver.Constraints(constraints); err != nil {
		t.Fatalf("Constraints: %v", err)
	}

	if err := solver.Solve(); err != nil {
		t.Fatalf("Solve: %v", err)
	}

	result := solver.Result()
	if len(result) != 25 {
		t.Fatalf("expected 25 settled cells, got %d", len(result))
	}

	assertValidConfiguration(t, rules, result)

	for _, con := range constraints {
		got, ok := result[con.Key]
		if !ok {
			t.Errorf("constrained cell %+v is missing from the result", con.Key)
			continue
		}
		if got.Tile != con.Tile {
			t.Errorf("cell %+v has tile %q, want %q", con.Key, got.Tile, con.Tile)
		}
		if con.Color != "" && got.Color != con.Color {
			t.Errorf("cell %+v has colour %q, want %q", con.Key, got.Color, con.Color)
		}
	}
}

// TestPropagationRemovesIncompatible checks that a settled cell removes
// incompatible tiles from its neighbours.
func TestPropagationRemovesIncompatible(t *testing.T) {
	rules := testRuleset(t)
	solver := NewSolver(rules, 1)

	solver.SetRegion(rectRegion(3, 3, 0))

	// Pin the centre to "wall". Its neighbours may be wall, window or
	// empty, but not roof (a roof's connectors are "roof", which do not
	// match "wall").
	err := solver.Constraints([]Constraint{
		{Key: CellKey{Q: 1, R: 1, Layer: 0}, Tile: "wall"},
	})
	if err != nil {
		t.Fatalf("Constraints: %v", err)
	}

	if err := solver.Propagate(); err != nil {
		t.Fatalf("Propagate: %v", err)
	}

	for d := Direction(0); d < dirCount; d++ {
		n := CellKey{Q: 1, R: 1, Layer: 0}.Neighbor(d)
		c := solver.cells[n]
		if c == nil {
			t.Fatalf("neighbour %+v missing from region", n)
		}
		if c.has(rules.MustLookup("roof")) {
			t.Errorf("neighbour %+v still allows roof after a wall was placed", n)
		}
	}
}

// TestWindowConnectorPrunesNorthNeighbour checks that edge connectors are
// honoured: a window faces north with a "window" connector, so the cell to
// its north may not be a plain wall.
// TestWindowConnectorPrunesNorthNeighbour checks that edge connectors are
// honoured rather than only tile identity.
//
// The window tile declares the "window" connector on its north edge only;
// its other five edges use "wall". The connector table allows "window" to
// meet "wall", so a plain wall may sit north of a window. But the window's
// *south* edge is "wall", so the cell to the south may not be a roof, whose
// edges are all "roof".
func TestWindowConnectorPrunesNorthNeighbour(t *testing.T) {
	rules := testRuleset(t)
	solver := NewSolver(rules, 1)

	solver.SetRegion(rectRegion(3, 3, 0))

	// The window's south edge is "wall". The cell to its south must meet
	// "wall", so it may be a wall, a window or an empty plot, but not a
	// roof.
	err := solver.Constraints([]Constraint{
		{Key: CellKey{Q: 1, R: 1, Layer: 0}, Tile: "window"},
	})
	if err != nil {
		t.Fatalf("Constraints: %v", err)
	}

	if err := solver.Propagate(); err != nil {
		t.Fatalf("Propagate: %v", err)
	}

	// (1,2) is the window's southern neighbour: it faces the window with
	// its own north edge, which must meet the window's south connector.
	south := CellKey{Q: 1, R: 2, Layer: 0}
	c := solver.cells[south]
	if c == nil {
		t.Fatalf("southern neighbour %+v missing", south)
	}
	if c.has(rules.MustLookup("roof")) {
		t.Error("southern neighbour of a window still allows a roof")
	}
	if !c.has(rules.MustLookup("wall")) {
		t.Error("southern neighbour of a window should still allow a wall")
	}

	// The window's north edge is "window", so the cell to the north may
	// be a window or a wall but not a roof.
	north := CellKey{Q: 1, R: 0, Layer: 0}
	nc := solver.cells[north]
	if nc == nil {
		t.Fatalf("northern neighbour %+v missing", north)
	}
	if nc.has(rules.MustLookup("roof")) {
		t.Error("northern neighbour of a window still allows a roof")
	}
}

// TestContradictoryConstraints checks that conflicting requirements are
// reported as an error rather than producing a corrupt town.
func TestContradictoryConstraints(t *testing.T) {
	rules := testRuleset(t)
	solver := NewSolver(rules, 1)

	solver.SetRegion(rectRegion(3, 3, 0))

	// A window's north connector is "window" while a roof's south
	// connector is "roof", so a window directly south of a roof is
	// contradictory.
	err := solver.Constraints([]Constraint{
		{Key: CellKey{Q: 1, R: 1, Layer: 0}, Tile: "roof"},
		{Key: CellKey{Q: 1, R: 2, Layer: 0}, Tile: "window"},
	})
	if err != nil {
		t.Fatalf("Constraints: %v", err)
	}

	err = solver.Solve()
	if err == nil {
		t.Fatal("expected a contradiction error, got nil")
	}
	if !errors.Is(err, ErrContradiction) {
		t.Errorf("expected ErrContradiction, got %v", err)
	}
}

// TestIsolatedCell checks a cell whose neighbours are all pinned to tiles it
// cannot touch.
func TestIsolatedCell(t *testing.T) {
	rules := testRuleset(t)
	solver := NewSolver(rules, 1)

	solver.SetRegion(rectRegion(3, 3, 0))

	// Pin the centre to a roof and every hex neighbour to a window. The
	// roof's connectors are all "roof" while the window's are "window"
	// (north) and "wall" (the rest), so every neighbour clashes.
	if err := solver.Constraints([]Constraint{
		{Key: CellKey{Q: 1, R: 1, Layer: 0}, Tile: "roof"},
	}); err != nil {
		t.Fatalf("Constraints: %v", err)
	}
	for d := Direction(0); d < dirCount; d++ {
		n := CellKey{Q: 1, R: 1, Layer: 0}.Neighbor(d)
		if err := solver.Constraints([]Constraint{{Key: n, Tile: "window"}}); err != nil {
			t.Fatalf("Constraints: %v", err)
		}
	}

	err := solver.Solve()
	if err == nil {
		t.Fatal("expected a contradiction error for the isolated cell, got nil")
	}
	if !errors.Is(err, ErrContradiction) {
		t.Errorf("expected ErrContradiction, got %v", err)
	}
}

// TestDeterminism checks that the same seed and constraints always produce
// exactly the same town.
func TestDeterminism(t *testing.T) {
	rules := testRuleset(t)

	solve := func() map[CellKey]ResultCell {
		solver := NewSolver(rules, 1234)
		solver.SetRegion(rectRegion(6, 6, 0))
		if err := solver.Constraints([]Constraint{
			{Key: CellKey{Q: 3, R: 3, Layer: 0}, Tile: "wall"},
		}); err != nil {
			t.Fatalf("Constraints: %v", err)
		}
		if err := solver.Solve(); err != nil {
			t.Fatalf("Solve: %v", err)
		}
		return solver.Result()
	}

	first := solve()
	second := solve()

	if !reflect.DeepEqual(first, second) {
		t.Error("two solves with the same seed produced different results")
	}
}

// TestDifferentSeedsProduceDifferentTowns checks the "organic" property: the
// same constraints with different seeds should generally yield different
// towns.
func TestDifferentSeedsProduceDifferentTowns(t *testing.T) {
	rules := testRuleset(t)

	solve := func(seed int64) map[CellKey]ResultCell {
		solver := NewSolver(rules, seed)
		solver.SetRegion(rectRegion(8, 8, 0))
		if err := solver.Constraints([]Constraint{
			{Key: CellKey{Q: 4, R: 4, Layer: 0}, Tile: "wall"},
		}); err != nil {
			t.Fatalf("Constraints: %v", err)
		}
		if err := solver.Solve(); err != nil {
			t.Fatalf("Solve: %v", err)
		}
		return solver.Result()
	}

	a := solve(1)
	b := solve(2)

	if reflect.DeepEqual(a, b) {
		t.Error("two solves with different seeds produced identical results")
	}
}

// TestUpperLayerConstraints checks the vertical adjacency rule: a wall may
// support a tower.
func TestUpperLayerConstraints(t *testing.T) {
	rules := testRuleset(t)
	solver := NewSolver(rules, 7)

	keys := append(rectRegion(3, 3, 0), rectRegion(3, 3, 1)...)
	solver.SetRegion(keys)

	// Pin the whole ground layer to walls so the tower has support. An
	// empty plot cannot carry a tower, which is the point of the rule.
	err := solver.Constraints([]Constraint{
		{Key: CellKey{Q: 1, R: 1, Layer: 0}, Tile: "wall"},
		{Key: CellKey{Q: 1, R: 1, Layer: 1}, Tile: "tower"},
	})
	if err != nil {
		t.Fatalf("Constraints: %v", err)
	}

	if err := solver.Solve(); err != nil {
		t.Fatalf("Solve: %v", err)
	}

	result := solver.Result()
	assertValidConfiguration(t, rules, result)

	if got := result[CellKey{Q: 1, R: 1, Layer: 1}].Tile; got != "tower" {
		t.Errorf("upper cell has tile %q, want %q", got, "tower")
	}
}

// TestUnsupportedUpperLayerIsRejected checks that a tile which cannot support
// anything is rejected when something is placed on it.
func TestUnsupportedUpperLayerIsRejected(t *testing.T) {
	rules := testRuleset(t)
	solver := NewSolver(rules, 7)

	keys := append(rectRegion(3, 3, 0), rectRegion(3, 3, 1)...)
	solver.SetRegion(keys)

	err := solver.Constraints([]Constraint{
		{Key: CellKey{Q: 1, R: 1, Layer: 0}, Tile: "roof"},
		{Key: CellKey{Q: 1, R: 1, Layer: 1}, Tile: "tower"},
	})
	if err != nil {
		t.Fatalf("Constraints: %v", err)
	}

	if err := solver.Solve(); err == nil {
		t.Fatal("expected a contradiction: a roof cannot support a tower")
	}
}

// TestUnknownTileConstraintIsRejected checks that an invalid tile name is
// reported cleanly.
func TestUnknownTileConstraintIsRejected(t *testing.T) {
	rules := testRuleset(t)
	solver := NewSolver(rules, 1)

	solver.SetRegion(rectRegion(2, 2, 0))

	err := solver.Constraints([]Constraint{
		{Key: CellKey{Q: 0, R: 0, Layer: 0}, Tile: "dinosaur"},
	})
	if err == nil {
		t.Fatal("expected an error for an unknown tile")
	}
	if !strings.Contains(err.Error(), "dinosaur") {
		t.Errorf("error %q does not mention the offending tile", err)
	}
}

// TestConstraintOutsideRegionIsRejected checks the error for a constraint
// that targets a cell outside the solve region.
func TestConstraintOutsideRegionIsRejected(t *testing.T) {
	rules := testRuleset(t)
	solver := NewSolver(rules, 1)

	solver.SetRegion(rectRegion(2, 2, 0))

	err := solver.Constraints([]Constraint{
		{Key: CellKey{Q: 9, R: 9, Layer: 0}, Tile: "wall"},
	})
	if err == nil {
		t.Fatal("expected an error for a constraint outside the region")
	}
}

// TestTileOnWrongLayerIsRejected checks that a layer-1-only tile cannot be
// placed on the ground layer.
func TestTileOnWrongLayerIsRejected(t *testing.T) {
	rules := testRuleset(t)
	solver := NewSolver(rules, 1)

	solver.SetRegion(rectRegion(2, 2, 0))

	err := solver.Constraints([]Constraint{
		{Key: CellKey{Q: 0, R: 0, Layer: 0}, Tile: "tower"},
	})
	if err == nil {
		t.Fatal("expected an error placing a layer-1 tile on layer 0")
	}
}

// TestSelectCellPicksMostConstrained checks the entropy selection: the cell
// with the fewest remaining candidates must be chosen.
func TestSelectCellPicksMostConstrained(t *testing.T) {
	rules := testRuleset(t)
	solver := NewSolver(rules, 1)

	solver.SetRegion(rectRegion(3, 3, 0))

	// Settle the centre to a wall, which prunes its neighbours down to
	// fewer candidates than the corners retain.
	if err := solver.Constraints([]Constraint{
		{Key: CellKey{Q: 1, R: 1, Layer: 0}, Tile: "wall"},
	}); err != nil {
		t.Fatalf("Constraints: %v", err)
	}
	if err := solver.Propagate(); err != nil {
		t.Fatalf("Propagate: %v", err)
	}

	// Work out the true minimum so the assertion states the invariant
	// rather than a hard-coded number.
	minimum := -1
	for _, k := range solver.order {
		c := solver.cells[k]
		if c.settled {
			continue
		}
		if minimum < 0 || c.count() < minimum {
			minimum = c.count()
		}
	}
	if minimum < 0 {
		t.Fatal("no unsettled cells")
	}

	key, ok := solver.SelectCell()
	if !ok {
		t.Fatal("expected an unsettled cell")
	}

	c := solver.cells[key]
	if c.count() != minimum {
		t.Errorf("selected cell %+v has %d candidates, want the minimum (%d)",
			key, c.count(), minimum)
	}
}

// TestCollapseIsDeterministicPerCell checks that a cell's random choice
// depends only on the seed and the cell's position, not on solve order.
func TestCollapseIsDeterministicPerCell(t *testing.T) {
	rules := testRuleset(t)

	pick := func() TileID {
		solver := NewSolver(rules, 99)
		solver.SetRegion(rectRegion(4, 4, 0))
		key := CellKey{Q: 2, R: 2, Layer: 0}
		if err := solver.Collapse(key); err != nil {
			t.Fatalf("Collapse: %v", err)
		}
		return solver.cells[key].collapsed
	}

	first := pick()
	for i := 0; i < 5; i++ {
		if got := pick(); got != first {
			t.Errorf("collapse %d differed: got %v, want %v", i, got, first)
		}
	}
}

// TestWeightedSelectionFavoursHeavyTiles checks that weighted random
// selection respects the weights: a heavy tile should dominate over many
// samples.
func TestWeightedSelectionFavoursHeavyTiles(t *testing.T) {
	openEdges := map[string]string{
		"n": "open", "ne": "open", "se": "open", "s": "open", "sw": "open", "nw": "open",
	}
	rules, err := CompileRuleset(RulesetSchema{
		Tiles: []TileDef{
			{ID: "common", Weight: 90, Layer: "0", Edges: openEdges, Up: "nothing"},
			{ID: "rare", Weight: 10, Layer: "0", Edges: openEdges, Up: "nothing"},
		},
		Connectors: []ConnectorRule{
			{A: "open", B: "open", Axis: AxisHex},
		},
	})
	if err != nil {
		t.Fatalf("CompileRuleset: %v", err)
	}

	counts := map[string]int{}
	const samples = 2000
	for i := 0; i < samples; i++ {
		solver := NewSolver(rules, int64(i))
		key := CellKey{Q: 0, R: 0, Layer: 0}
		solver.SetRegion([]CellKey{key})
		if err := solver.Collapse(key); err != nil {
			t.Fatalf("Collapse: %v", err)
		}
		counts[rules.Tile(solver.cells[key].collapsed).ID]++
	}

	if counts["rare"] == 0 {
		t.Error("the rare tile was never selected")
	}
	if counts["common"] <= counts["rare"] {
		t.Errorf("the common tile (%d) should be selected more often than the rare tile (%d)",
			counts["common"], counts["rare"])
	}
}
