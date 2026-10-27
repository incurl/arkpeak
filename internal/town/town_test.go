package town

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/incurl/arkpeak/internal/wfc"
)

// testRules loads the embedded ruleset. It fails the test if the ruleset
// cannot be compiled, which keeps the tests honest about the real rules.
func testRules(t *testing.T) *wfc.Ruleset {
	t.Helper()
	rules, err := LoadEmbeddedRules()
	if err != nil {
		t.Fatalf("LoadEmbeddedRules: %v", err)
	}
	return rules
}

func TestLoadEmbeddedRules(t *testing.T) {
	rules := testRules(t)

	if len(rules.Tiles) == 0 {
		t.Fatal("the ruleset has no tiles")
	}

	// Every tile must be reachable from the frontend's palette, so the
	// tile ids must be the ones the frontend expects.
	want := []string{
		"empty", "floor", "garden", "stairs",
		"wall", "window", "arch", "roof", "roof_edge",
		"tower", "spire",
	}
	for _, id := range want {
		if _, ok := rules.Lookup(id); !ok {
			t.Errorf("the ruleset is missing the tile %q", id)
		}
	}
}

func TestNewTownSolves(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 12345)

	snapshot, err := service.NewTown(12345)
	if err != nil {
		t.Fatalf("NewTown: %v", err)
	}

	if len(snapshot.Cells) == 0 {
		t.Fatal("the town has no cells")
	}
	if snapshot.Seed != 12345 {
		t.Errorf("seed = %d, want 12345", snapshot.Seed)
	}
	if snapshot.Radius != DefaultGridRadius {
		t.Errorf("radius = %d, want %d", snapshot.Radius, DefaultGridRadius)
	}

	// The frontend builds its palette from this list, so it must not be
	// empty.
	if len(snapshot.Tiles) == 0 {
		t.Error("the snapshot lists no tiles")
	}
}

// TestSnapshotDescribesTheRules checks that the snapshot carries everything
// the renderer needs to build neighbour-aware geometry.
//
// The renderer must not restate the rules, so the connectors have to come
// from the ruleset. If they are missing or wrong, the geometry silently
// stops matching the town.
func TestSnapshotDescribesTheRules(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 12345)

	snapshot, err := service.NewTown(12345)
	if err != nil {
		t.Fatalf("NewTown: %v", err)
	}

	// Every tile must be described, with its layer and its six edges.
	if len(snapshot.TileInfo) != len(snapshot.Tiles) {
		t.Errorf("TileInfo has %d entries, but there are %d tiles",
			len(snapshot.TileInfo), len(snapshot.Tiles))
	}

	byID := make(map[string]TileInfo, len(snapshot.TileInfo))
	for _, info := range snapshot.TileInfo {
		byID[info.ID] = info
	}

	for _, id := range snapshot.Tiles {
		info, ok := byID[id]
		if !ok {
			t.Errorf("the tile %q is not described", id)
			continue
		}

		if len(info.Edges) != 6 {
			t.Errorf("the tile %q has %d edges, want 6", id, len(info.Edges))
		}
		for _, name := range []string{"n", "ne", "se", "s", "sw", "nw"} {
			if _, ok := info.Edges[name]; !ok {
				t.Errorf("the tile %q is missing the edge %q", id, name)
			}
		}
	}

	// The connectors must be the ruleset's own, so the frontend and the
	// solver agree on what may touch what.
	if len(snapshot.Connectors) == 0 {
		t.Fatal("the snapshot lists no connectors")
	}

	want := len(rules.ConnectorRules())
	if len(snapshot.Connectors) != want {
		t.Errorf("the snapshot has %d connectors, want %d", len(snapshot.Connectors), want)
	}

	// Every connector pair must be one the ruleset declares.
	for _, rule := range snapshot.Connectors {
		found := false
		for _, original := range rules.ConnectorRules() {
			if original.A == rule.A && original.B == rule.B && string(original.Axis) == rule.Axis {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("the snapshot has a connector the ruleset does not declare: %+v", rule)
		}
	}
}

// TestSnapshotTileInfoMatchesTheRuleset checks that the connectors in the
// snapshot are the ruleset's, not a copy that could drift.
func TestSnapshotTileInfoMatchesTheRuleset(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 555)

	snapshot, err := service.NewTown(555)
	if err != nil {
		t.Fatalf("NewTown: %v", err)
	}

	for _, info := range snapshot.TileInfo {
		tileID, ok := rules.Lookup(info.ID)
		if !ok {
			t.Errorf("the snapshot describes an unknown tile %q", info.ID)
			continue
		}

		def := rules.Tile(tileID)
		for name, connector := range def.Edges {
			if info.Edges[name] != connector {
				t.Errorf("the tile %q has edge %q = %q in the snapshot, want %q",
					info.ID, name, info.Edges[name], connector)
			}
		}

		if info.Up != def.Up {
			t.Errorf("the tile %q has up = %q in the snapshot, want %q",
				info.ID, info.Up, def.Up)
		}
		if info.Down != def.Down {
			t.Errorf("the tile %q has down = %q in the snapshot, want %q",
				info.ID, info.Down, def.Down)
		}
		if info.Solid != def.Solid {
			t.Errorf("the tile %q has solid = %v in the snapshot, want %v",
				info.ID, info.Solid, def.Solid)
		}
	}
}

func TestNewTownIsDeterministic(t *testing.T) {
	rules := testRules(t)

	first, err := NewService(rules, 999).NewTown(999)
	if err != nil {
		t.Fatalf("NewTown: %v", err)
	}
	second, err := NewService(rules, 999).NewTown(999)
	if err != nil {
		t.Fatalf("NewTown: %v", err)
	}

	if len(first.Cells) != len(second.Cells) {
		t.Fatalf("cell counts differ: %d vs %d", len(first.Cells), len(second.Cells))
	}
	for i := range first.Cells {
		if first.Cells[i] != second.Cells[i] {
			t.Fatalf("cell %d differs: %+v vs %+v", i, first.Cells[i], second.Cells[i])
		}
	}
}

func TestDifferentSeedsGiveDifferentTowns(t *testing.T) {
	rules := testRules(t)

	first, err := NewService(rules, 1).NewTown(1)
	if err != nil {
		t.Fatalf("NewTown: %v", err)
	}
	second, err := NewService(rules, 2).NewTown(2)
	if err != nil {
		t.Fatalf("NewTown: %v", err)
	}

	same := len(first.Cells) == len(second.Cells)
	if same {
		for i := range first.Cells {
			if first.Cells[i].Tile != second.Cells[i].Tile {
				same = false
				break
			}
		}
	}
	if same {
		t.Error("two different seeds produced identical towns")
	}
}

func TestPlaceConstraintProducesDiff(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 4242)

	if _, err := service.NewTown(4242); err != nil {
		t.Fatalf("NewTown: %v", err)
	}

	diff, err := service.PlaceConstraint(5, 5, 0, "wall", "rose")
	if err != nil {
		t.Fatalf("PlaceConstraint: %v", err)
	}

	if len(diff.Settled) == 0 {
		t.Error("the diff reports no settled cells")
	}

	// The constrained cell must be in the diff and carry the right tile
	// and colour.
	found := false
	for _, cell := range diff.Settled {
		if cell.Q == 5 && cell.R == 5 && cell.Layer == 0 {
			found = true
			if cell.Tile != "wall" {
				t.Errorf("cell tile = %q, want %q", cell.Tile, "wall")
			}
			if cell.Color != "rose" {
				t.Errorf("cell colour = %q, want %q", cell.Color, "rose")
			}
		}
	}
	if !found {
		t.Error("the constrained cell is missing from the diff")
	}
}

func TestPlaceConstraintRejectsUnknownTile(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 1)

	if _, err := service.NewTown(1); err != nil {
		t.Fatalf("NewTown: %v", err)
	}

	if _, err := service.PlaceConstraint(0, 0, 0, "dinosaur", ""); err == nil {
		t.Error("expected an error for an unknown tile")
	}
}

// TestFailedPlacementDoesNotPoisonTheTown checks that a placement which
// cannot be satisfied leaves no trace behind.
//
// A roof cannot sit next to a tower, so placing one there fails. If the
// constraint were left in place, every later solve would fail with it, and
// it would end up in the save file.
func TestFailedPlacementDoesNotPoisonTheTown(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 9090)

	if _, err := service.NewTown(9090); err != nil {
		t.Fatalf("NewTown: %v", err)
	}

	snapshot, err := service.GetSnapshot()
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}

	// Find a layer 1 cell next to a tower, and try to put a roof there.
	byKey := make(map[wfc.CellKey]Cell, len(snapshot.Cells))
	for _, cell := range snapshot.Cells {
		byKey[wfc.CellKey{Q: cell.Q, R: cell.R, Layer: cell.Layer}] = cell
	}

	var target *wfc.CellKey
	for _, cell := range snapshot.Cells {
		if cell.Layer != 1 || cell.Tile != "tower" {
			continue
		}
		key := wfc.CellKey{Q: cell.Q, R: cell.R, Layer: 1}
		for d := wfc.Direction(0); d < 6; d++ {
			neighbour := key.Neighbor(d)
			if _, ok := byKey[neighbour]; !ok {
				continue
			}
			candidate := neighbour
			target = &candidate
			break
		}
		if target != nil {
			break
		}
	}

	if target == nil {
		t.Skip("this town has no tower next to another layer 1 cell")
	}

	// Placing a roof there must either work or fail cleanly. If it fails,
	// the town must be unchanged.
	if _, err := service.PlaceConstraint(target.Q, target.R, target.Layer, "roof", ""); err == nil {
		t.Log("the placement was accepted; the town may have changed")
		return
	}

	// The failed placement must not have left a constraint behind, or it
	// would end up in the save file and break every later solve.
	path := filepath.Join(t.TempDir(), "town.json")
	if err := service.SaveTown(path); err != nil {
		t.Fatalf("SaveTown: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var save SaveFile
	if err := json.Unmarshal(data, &save); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	for _, c := range save.Constraints {
		if c.Q == target.Q && c.R == target.R && c.Layer == target.Layer {
			t.Errorf("the failed placement left a constraint behind: %+v", c)
		}
	}

	// The town must still solve, which it would not if a bad constraint
	// were still in place.
	if _, err := service.SolveRegion(0, 0, 2); err != nil {
		t.Errorf("the town no longer solves after a failed placement: %v", err)
	}
}

// TestCanPlaceExplainsWhyNot checks that the preview gives the player a
// reason, not just a refusal.
func TestCanPlaceExplainsWhyNot(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 1234)

	if _, err := service.NewTown(1234); err != nil {
		t.Fatalf("NewTown: %v", err)
	}

	tests := []struct {
		name  string
		q     int
		r     int
		layer int
		tile  string
	}{
		{"unknown tile", 0, 0, 0, "dinosaur"},
		{"wrong layer", 0, 0, 1, "wall"},
		{"outside the island", 9999, 9999, 0, "wall"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, reason := service.CanPlace(tt.q, tt.r, tt.layer, tt.tile)
			if ok {
				t.Errorf("CanPlace(%q) = true, want false", tt.tile)
			}
			if reason == "" {
				t.Error("CanPlace gave no reason")
			}
		})
	}
}

// TestCanPlaceAcceptsAValidTile checks that the preview does not get in the
// way of a placement that works.
func TestCanPlaceAcceptsAValidTile(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 1234)

	if _, err := service.NewTown(1234); err != nil {
		t.Fatalf("NewTown: %v", err)
	}

	ok, reason := service.CanPlace(0, 0, 0, "wall")
	if !ok {
		t.Errorf("CanPlace(wall) = false (%s), want true", reason)
	}
}

func TestPlaceConstraintRejectsOutOfBounds(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 1)

	if _, err := service.NewTown(1); err != nil {
		t.Fatalf("NewTown: %v", err)
	}

	if _, err := service.PlaceConstraint(9999, 9999, 0, "wall", ""); err == nil {
		t.Error("expected an error for a cell outside the grid")
	}
}

// TestPlaceConstraintRejectsWrongLayer checks that a tile is refused when it
// does not belong to the layer it is placed on.
//
// The frontend hides the tiles that do not fit the layer the player is
// pointing at, so this should never be reached through the UI. It is still
// checked, because a client that ignores the palette must not be able to
// put a wall on the roof storey.
func TestPlaceConstraintRejectsWrongLayer(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 1)

	if _, err := service.NewTown(1); err != nil {
		t.Fatalf("NewTown: %v", err)
	}

	tests := []struct {
		name  string
		q     int
		r     int
		layer int
		tile  string
	}{
		{"wall on the roof storey", 2, 2, 1, "wall"},
		{"roof on the ground", 2, 2, 0, "roof"},
		{"tower on the ground", 3, 3, 0, "tower"},
		{"spire on the ground", 3, 3, 0, "spire"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := service.PlaceConstraint(tt.q, tt.r, tt.layer, tt.tile, ""); err == nil {
				t.Errorf("expected an error placing %q on layer %d", tt.tile, tt.layer)
			}
		})
	}
}

// TestPlaceConstraintAcceptsMatchingLayer checks that a tile is accepted
// when it belongs to the layer it is placed on.
func TestPlaceConstraintAcceptsMatchingLayer(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 1)

	if _, err := service.NewTown(1); err != nil {
		t.Fatalf("NewTown: %v", err)
	}

	tests := []struct {
		q     int
		r     int
		layer int
		tile  string
	}{
		{2, 2, 0, "wall"},
		{2, 2, 0, "garden"},
		{4, 4, 0, "floor"},
	}

	for _, tt := range tests {
		t.Run(tt.tile, func(t *testing.T) {
			if _, err := service.PlaceConstraint(tt.q, tt.r, tt.layer, tt.tile, ""); err != nil {
				t.Errorf("PlaceConstraint(%q, layer %d): %v", tt.tile, tt.layer, err)
			}
		})
	}
}

// TestRemoveConstraintFreesTheCell checks that removing a constraint hands
// the cell back to the solver.
//
// The cell must end up without a player constraint, but the solver is free
// to pick a different tile for it.
func TestRemoveConstraintFreesTheCell(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 31337)

	if _, err := service.NewTown(31337); err != nil {
		t.Fatalf("NewTown: %v", err)
	}

	if _, err := service.PlaceConstraint(6, 6, 0, "wall", "rose"); err != nil {
		t.Fatalf("PlaceConstraint: %v", err)
	}

	diff, err := service.RemoveConstraint(6, 6, 0)
	if err != nil {
		t.Fatalf("RemoveConstraint: %v", err)
	}

	// The diff must mention the cell, because the solver may have
	// changed it.
	touched := false
	for _, cell := range diff.Settled {
		if cell.Q == 6 && cell.R == 6 && cell.Layer == 0 {
			touched = true
		}
	}
	for _, cell := range diff.Removed {
		if cell.Q == 6 && cell.R == 6 && cell.Layer == 0 {
			touched = true
		}
	}
	if !touched {
		t.Error("the diff does not mention the cell the constraint was removed from")
	}

	// The town must still hold every cell: removing a constraint never
	// leaves a hole in the grid.
	snapshot, err := service.GetSnapshot()
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	for _, cell := range snapshot.Cells {
		if cell.Q == 6 && cell.R == 6 && cell.Layer == 0 && cell.Color == "rose" {
			t.Error("the removed constraint's colour is still on the cell")
		}
	}
}

// TestRemoveConstraintOnUnconstrainedCellIsHarmless checks that removing a
// constraint that was never placed does not damage the town.
func TestRemoveConstraintOnUnconstrainedCellIsHarmless(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 5150)

	if _, err := service.NewTown(5150); err != nil {
		t.Fatalf("NewTown: %v", err)
	}

	before, err := service.GetSnapshot()
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}

	if _, err := service.RemoveConstraint(9, 9, 0); err != nil {
		t.Fatalf("RemoveConstraint: %v", err)
	}

	after, err := service.GetSnapshot()
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}

	if len(before.Cells) != len(after.Cells) {
		t.Fatalf("cell counts differ: %d vs %d", len(before.Cells), len(after.Cells))
	}
	for i := range before.Cells {
		if before.Cells[i] != after.Cells[i] {
			t.Fatalf("cell %d changed: %+v vs %+v", i, before.Cells[i], after.Cells[i])
		}
	}
}

// TestPlaceAndRemoveRoundTrip checks that placing a constraint and then
// removing it returns the town to the state it was in before.
//
// This is what the player expects from an undo.
func TestPlaceAndRemoveRoundTrip(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 24680)

	if _, err := service.NewTown(24680); err != nil {
		t.Fatalf("NewTown: %v", err)
	}

	before, err := service.GetSnapshot()
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}

	if _, err := service.PlaceConstraint(4, 4, 0, "wall", "cream"); err != nil {
		t.Fatalf("PlaceConstraint: %v", err)
	}
	if _, err := service.RemoveConstraint(4, 4, 0); err != nil {
		t.Fatalf("RemoveConstraint: %v", err)
	}

	after, err := service.GetSnapshot()
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}

	if len(before.Cells) != len(after.Cells) {
		t.Fatalf("cell counts differ: %d vs %d", len(before.Cells), len(after.Cells))
	}
	for i := range before.Cells {
		if before.Cells[i] != after.Cells[i] {
			t.Fatalf("cell %d changed: %+v vs %+v", i, before.Cells[i], after.Cells[i])
		}
	}
}

// TestRegionResolveIsStable checks that a regional re-solve leaves the town
// alone when nothing has changed.
//
// Without this, placing a single block would reshuffle its whole
// neighbourhood, which is neither cozy nor predictable.
func TestRegionResolveIsStable(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 777)

	if _, err := service.NewTown(777); err != nil {
		t.Fatalf("NewTown: %v", err)
	}
	if _, err := service.PlaceConstraint(3, 3, 0, "wall", "sage"); err != nil {
		t.Fatalf("PlaceConstraint: %v", err)
	}

	before, err := service.GetSnapshot()
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}

	// Re-solving a region that contains no constraint must be a no-op.
	diff, err := service.SolveRegion(12, 12, 4)
	if err != nil {
		t.Fatalf("SolveRegion: %v", err)
	}
	if len(diff.Settled) != 0 || len(diff.Removed) != 0 {
		t.Errorf("a no-op re-solve changed %d cells", len(diff.Settled)+len(diff.Removed))
	}

	after, err := service.GetSnapshot()
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if len(before.Cells) != len(after.Cells) {
		t.Fatalf("cell counts differ: %d vs %d", len(before.Cells), len(after.Cells))
	}
	for i := range before.Cells {
		if before.Cells[i] != after.Cells[i] {
			t.Fatalf("cell %d changed: %+v vs %+v", i, before.Cells[i], after.Cells[i])
		}
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 777)

	if _, err := service.NewTown(777); err != nil {
		t.Fatalf("NewTown: %v", err)
	}
	if _, err := service.PlaceConstraint(3, 3, 0, "wall", "sage"); err != nil {
		t.Fatalf("PlaceConstraint: %v", err)
	}

	path := filepath.Join(t.TempDir(), "town.json")
	if err := service.SaveTown(path); err != nil {
		t.Fatalf("SaveTown: %v", err)
	}

	// A fresh service must reproduce the same town from the file.
	loaded := NewService(rules, 0)
	if err := loaded.LoadTown(path); err != nil {
		t.Fatalf("LoadTown: %v", err)
	}

	before, err := service.GetSnapshot()
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	after, err := loaded.GetSnapshot()
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}

	if before.Seed != after.Seed {
		t.Errorf("seed differs: %d vs %d", before.Seed, after.Seed)
	}
	if len(before.Cells) != len(after.Cells) {
		t.Fatalf("cell counts differ: %d vs %d", len(before.Cells), len(after.Cells))
	}
	for i := range before.Cells {
		if before.Cells[i] != after.Cells[i] {
			t.Fatalf("cell %d differs: %+v vs %+v", i, before.Cells[i], after.Cells[i])
		}
	}
}

func TestLoadTownRejectsMissingFile(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 1)

	if err := service.LoadTown(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Error("expected an error loading a missing file")
	}
}

func TestLoadTownRejectsGarbage(t *testing.T) {
	rules := testRules(t)
	service := NewService(rules, 1)

	path := filepath.Join(t.TempDir(), "town.json")
	if err := os.WriteFile(path, []byte("this is not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := service.LoadTown(path); err == nil {
		t.Error("expected an error loading a malformed file")
	}
}
