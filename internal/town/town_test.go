package town

import (
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
	if snapshot.Width <= 0 || snapshot.Height <= 0 {
		t.Errorf("grid size = %dx%d, want a positive size", snapshot.Width, snapshot.Height)
	}

	// The frontend builds its palette from this list, so it must not be
	// empty.
	if len(snapshot.Tiles) == 0 {
		t.Error("the snapshot lists no tiles")
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
