package town

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/incurl/arkpeak/internal/wfc"
)

// RulesFile is the path to the tile ruleset, relative to the repository
// root. It is embedded at build time so the shipped binary carries its own
// rules.
const RulesFile = "internal/town/rules/town_rules.json"

// Service is the Wails service that exposes the town to the frontend.
//
// Wails v3 binds a service's exported methods automatically; the frontend
// calls them through the generated bindings. All methods are safe to call
// from the frontend thread: the service serialises access with a mutex.
type Service struct {
	mu sync.Mutex

	// town is the authoritative town state.
	town *Town

	// rules is the compiled tile ruleset.
	rules *wfc.Ruleset
}

// NewService creates a town service with the given ruleset and initial seed.
func NewService(rules *wfc.Ruleset, seed int64) *Service {
	return &Service{
		rules: rules,
		town:  NewTown(seed, rules),
	}
}

// LoadRules loads and compiles the ruleset from the given path.
func LoadRules(path string) (*wfc.Ruleset, error) {
	rules, err := wfc.LoadRuleset(path)
	if err != nil {
		return nil, err
	}
	return rules, nil
}

// CurrentTown returns the service's current town. It is used by the tests.
//
// It is deliberately unexported so Wails does not bind it: the Town struct
// has no exported fields, so binding it would only produce a meaningless
// empty type in the frontend.
func (s *Service) currentTown() *Town {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.town
}

// NewTown replaces the current town with a fresh one for the given seed and
// solves it.
func (s *Service) NewTown(seed int64) (TownSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t := NewTown(seed, s.rules)
	if err := t.SolveAll(); err != nil {
		return TownSnapshot{}, fmt.Errorf("solve town: %w", err)
	}
	s.town = t

	return s.town.Snapshot(), nil
}

// GetSnapshot returns the full state of the current town.
func (s *Service) GetSnapshot() (TownSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.town == nil {
		return TownSnapshot{}, fmt.Errorf("no town has been created yet")
	}
	return s.town.Snapshot(), nil
}

// PlaceConstraint places a tile constraint on a cell and re-solves the
// affected region.
func (s *Service) PlaceConstraint(q, r, layer int, tileType string, color string) (GridDiff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.town == nil {
		return GridDiff{}, fmt.Errorf("no town has been created yet")
	}

	key := wfc.CellKey{Q: q, R: r, Layer: layer}
	diff, err := s.town.PlaceConstraint(key, tileType, color)
	if err != nil {
		return GridDiff{}, err
	}
	return diff, nil
}

// RemoveConstraint removes the constraint on a cell and re-solves the
// affected region.
func (s *Service) RemoveConstraint(q, r, layer int) (GridDiff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.town == nil {
		return GridDiff{}, fmt.Errorf("no town has been created yet")
	}

	key := wfc.CellKey{Q: q, R: r, Layer: layer}
	diff, err := s.town.RemoveConstraint(key)
	if err != nil {
		return GridDiff{}, err
	}
	return diff, nil
}

// SolveRegion re-solves a region of the town around the given cell without
// placing any constraint. It is a manual trigger for the player to nudge a
// stubborn part of the town.
func (s *Service) SolveRegion(q, r, radius int) (GridDiff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.town == nil {
		return GridDiff{}, fmt.Errorf("no town has been created yet")
	}
	if radius <= 0 {
		return GridDiff{}, fmt.Errorf("radius must be positive, got %d", radius)
	}

	key := wfc.CellKey{Q: q, R: r, Layer: 0}
	diff, err := s.town.resolveAround(key, radius)
	if err != nil {
		return GridDiff{}, err
	}
	return diff, nil
}

// SaveFile is the on-disk representation of a saved town.
//
// Only the seed and the player's constraints are stored. The settled cells
// are derived from those by the solver, so a saved town is always exactly
// reproducible and the file stays small.
type SaveFile struct {
	// Version is the save format version, for future migrations.
	Version int `json:"version"`
	// Seed is the town's deterministic seed.
	Seed int64 `json:"seed"`
	// Width and Height are the grid dimensions in hex cells.
	Width  int `json:"width"`
	Height int `json:"height"`
	// Constraints are the player's placed requirements.
	Constraints []SavedConstraint `json:"constraints"`
}

// SavedConstraint is a single player constraint as stored on disk.
type SavedConstraint struct {
	Q     int    `json:"q"`
	R     int    `json:"r"`
	Layer int    `json:"layer"`
	Tile  string `json:"tile"`
	Color string `json:"color"`
}

// saveVersion is the current save format version.
const saveVersion = 1

// SaveTown writes the town to the given path.
func (s *Service) SaveTown(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.town == nil {
		return fmt.Errorf("no town has been created yet")
	}

	save := SaveFile{
		Version:     saveVersion,
		Seed:        s.town.Seed(),
		Width:       s.town.width,
		Height:      s.town.height,
		Constraints: make([]SavedConstraint, 0, len(s.town.constraints)),
	}

	// The constraints are saved in the order they were placed, because a
	// town is built incrementally and replaying that order is what
	// reproduces it exactly.
	for _, key := range s.town.order {
		c, ok := s.town.constraints[key]
		if !ok {
			continue
		}
		save.Constraints = append(save.Constraints, SavedConstraint{
			Q:     c.Key.Q,
			R:     c.Key.R,
			Layer: c.Key.Layer,
			Tile:  c.Tile,
			Color: c.Color,
		})
	}

	data, err := json.MarshalIndent(save, "", "  ")
	if err != nil {
		return fmt.Errorf("encode town: %w", err)
	}
	data = append(data, '\n')

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %q: %w", dir, err)
		}
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write town %q: %w", path, err)
	}
	return nil
}

// LoadTown reads a town from the given path and solves it.
func (s *Service) LoadTown(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read town %q: %w", path, err)
	}

	var save SaveFile
	if err := json.Unmarshal(data, &save); err != nil {
		return fmt.Errorf("parse town %q: %w", path, err)
	}
	if save.Version != saveVersion {
		return fmt.Errorf("unsupported save version %d (expected %d)", save.Version, saveVersion)
	}

	t := NewTown(save.Seed, s.rules)
	if save.Width > 0 {
		t.width = save.Width
	}
	if save.Height > 0 {
		t.height = save.Height
	}

	for _, c := range save.Constraints {
		key := wfc.CellKey{Q: c.Q, R: c.R, Layer: c.Layer}
		if c.Tile != "" {
			if err := s.rules.ValidateTileID(c.Tile); err != nil {
				return fmt.Errorf("saved constraint at %+v: %w", key, err)
			}
		}
		t.constraints[key] = wfc.Constraint{Key: key, Tile: c.Tile, Color: c.Color}
		t.order = append(t.order, key)
	}

	// Replay the constraints in the order they were saved, so the loaded
	// town matches the town that was saved exactly.
	if err := t.Rebuild(); err != nil {
		return fmt.Errorf("solve loaded town: %w", err)
	}

	s.town = t
	return nil
}
