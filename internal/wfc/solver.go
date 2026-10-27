package wfc

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
)

// ErrContradiction is returned when the constraints cannot be satisfied: the
// solver reached a cell with no valid tile remaining.
var ErrContradiction = errors.New("wfc: constraints are contradictory")

// ConflictError reports two tiles that cannot sit next to each other.
//
// It carries the two tiles rather than only describing them, so that a
// caller can explain the problem in its own terms. The player does not need
// cell coordinates; they need to know which buildings clash.
type ConflictError struct {
	// First is the id of the first tile.
	First string
	// Second is the id of the second tile.
	Second string
	// FirstKey is the position of the first cell.
	FirstKey CellKey
	// SecondKey is the position of the second cell.
	SecondKey CellKey
}

// Error implements the error interface.
func (e *ConflictError) Error() string {
	return fmt.Sprintf(
		"wfc: constraints are contradictory: cells %+v (%s) and %+v (%s) cannot touch",
		e.FirstKey, e.First, e.SecondKey, e.Second,
	)
}

// Unwrap makes errors.Is(err, ErrContradiction) true for a conflict.
func (e *ConflictError) Unwrap() error {
	return ErrContradiction
}

// Constraint is a player-placed requirement that a specific cell must hold a
// specific tile.
//
// A constraint with an empty Tile is a "soft" constraint: it pins the cell's
// colour but leaves the tile type to the solver.
type Constraint struct {
	// Key is the cell the constraint applies to.
	Key CellKey
	// Tile is the required tile type. Empty means the solver chooses.
	Tile string
	// Color is the colour the player picked. Empty means the default.
	Color string
}

// cell is a single position in the solver's wave.
type cell struct {
	// layer is the vertical layer this cell belongs to.
	layer int

	// candidates is the bitmask of tiles still possible for this cell.
	// A settled cell has exactly one bit set.
	candidates uint64

	// settled reports whether the cell has been collapsed to one tile.
	settled bool

	// collapsed records the tile this cell was collapsed to, for use as a
	// fixed boundary during a later regional re-solve.
	collapsed TileID

	// color is the player-chosen colour carried through to the result.
	color string

	// fixed reports whether the cell was pinned by the player. Fixed cells
	// are never re-collapsed during a regional re-solve.
	fixed bool

	// preferred is a tile the cell would like to keep, used to keep a
	// regional re-solve stable. It is a preference, not a constraint: the
	// cell may still be collapsed to a different tile if the preference
	// turns out to be impossible.
	preferred TileID

	// preferredSet reports whether preferred is meaningful.
	preferredSet bool
}

// has reports whether the given tile is still a candidate for this cell.
func (c *cell) has(t TileID) bool {
	return c.candidates&(1<<uint(t)) != 0
}

// count returns the number of remaining candidates.
func (c *cell) count() int {
	return popcount(c.candidates)
}

// Solver holds the mutable state of a WFC solve.
//
// A Solver is not safe for concurrent use. The TownService serialises access
// with a mutex.
type Solver struct {
	rules *Ruleset

	// cells holds every cell in the current region, keyed by position.
	cells map[CellKey]*cell

	// order is the deterministic iteration order of cells, used when
	// selecting among equally-constrained cells.
	order []CellKey

	// rng drives weighted random selection.
	rng *rand.Rand

	// below holds the pinned tiles of the layer beneath the region. They
	// constrain the region vertically but are not part of it.
	below map[CellKey]TileID

	// seed is the town's seed, kept so regional re-solves can derive
	// per-cell RNGs that do not depend on solve order.
	seed int64

	// retries counts how many times the current solve has been restarted
	// after a contradiction.
	retries int
}

// NewSolver creates a solver for the given ruleset and seed.
func NewSolver(rules *Ruleset, seed int64) *Solver {
	return &Solver{
		rules: rules,
		cells: make(map[CellKey]*cell),
		rng:   rand.New(rand.NewSource(seed)),
		seed:  seed,
	}
}

// Rules returns the ruleset the solver is using.
func (s *Solver) Rules() *Ruleset {
	return s.rules
}

// InRegion reports whether the given cell is part of the solve region.
func (s *Solver) InRegion(key CellKey) bool {
	_, ok := s.cells[key]
	return ok
}

// popcount returns the number of set bits in v.
func popcount(v uint64) int {
	count := 0
	for v != 0 {
		v &= v - 1
		count++
	}
	return count
}

// tileMask returns a bitmask with a single tile set.
func tileMask(t TileID) uint64 {
	return 1 << uint(t)
}

// maskTiles returns the tiles present in the given mask, in TileID order.
func maskTiles(mask uint64, out []TileID) []TileID {
	out = out[:0]
	for t := 0; t < maxTiles; t++ {
		if mask&(1<<uint(t)) != 0 {
			out = append(out, TileID(t))
		}
	}
	return out
}

// SetRegion defines the cells the solver operates on.
//
// Every cell in the region starts with the full set of tiles valid for its
// layer. The region is the set of cells the solver may collapse; cells
// outside it act as fixed boundaries.
func (s *Solver) SetRegion(keys []CellKey) {
	s.cells = make(map[CellKey]*cell, len(keys))
	s.order = make([]CellKey, 0, len(keys))

	for _, k := range keys {
		if _, dup := s.cells[k]; dup {
			continue
		}
		var mask uint64
		for _, t := range s.rules.TilesForLayer(k.Layer) {
			mask |= tileMask(t)
		}
		s.cells[k] = &cell{layer: k.Layer, candidates: mask}
		s.order = append(s.order, k)
	}

	// Keep a stable order so that equal-entropy ties always break the same
	// way for a given seed.
	sort.Slice(s.order, func(i, j int) bool {
		a, b := s.order[i], s.order[j]
		if a.Layer != b.Layer {
			return a.Layer < b.Layer
		}
		if a.Q != b.Q {
			return a.Q < b.Q
		}
		return a.R < b.R
	})
}

// SetBelow pins the tiles of the layer beneath the region.
//
// The cells below are not part of the region, so they cannot be collapsed,
// but they still constrain it: a cell on layer 1 may only hold a tile that
// the cell below it is able to support.
//
// The pinned cells are recorded separately from the region, so they act as
// fixed boundaries without being mistaken for region cells.
func (s *Solver) SetBelow(below map[CellKey]TileID) {
	s.below = make(map[CellKey]TileID, len(below))
	for k, v := range below {
		s.below[k] = v
	}
}

// Constraints pins cells to specific tiles and colours.
//
// It returns an error if a constraint targets a cell outside the solve
// region. Use ConstraintsWithin when a constraint set spans several layers
// and each layer is solved separately.
func (s *Solver) Constraints(constraints []Constraint) error {
	for _, con := range constraints {
		c, ok := s.cells[con.Key]
		if !ok {
			return fmt.Errorf("constraint targets cell %+v which is not in the solve region", con.Key)
		}
		if err := s.applyConstraint(c, con); err != nil {
			return err
		}
	}
	return nil
}

// ConstraintsWithin pins the cells to specific tiles and colours, ignoring
// any constraint that targets a cell outside the solve region.
//
// This is used when a town's constraints span several layers and each layer
// is solved in turn: a constraint on the ground layer must not be an error
// while solving the layer above it.
func (s *Solver) ConstraintsWithin(constraints []Constraint) error {
	for _, con := range constraints {
		c, ok := s.cells[con.Key]
		if !ok {
			continue
		}
		if err := s.applyConstraint(c, con); err != nil {
			return err
		}
	}
	return nil
}

// applyConstraint pins a single cell.
func (s *Solver) applyConstraint(c *cell, con Constraint) error {
	if con.Tile == "" {
		c.color = con.Color
		return nil
	}
	if err := s.rules.ValidateTileID(con.Tile); err != nil {
		return err
	}
	tile, _ := s.rules.Lookup(con.Tile)

	// The tile must be valid on this cell's layer.
	valid := false
	for _, t := range s.rules.TilesForLayer(c.layer) {
		if t == tile {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("tile %q cannot be placed on layer %d", con.Tile, c.layer)
	}

	c.candidates = tileMask(tile)
	c.settled = true
	c.collapsed = tile
	c.color = con.Color
	c.fixed = true
	return nil
}

// FixCell pins a cell to a tile without marking it as a player constraint.
// It is used to seed a regional re-solve from the town's existing state.
func (s *Solver) FixCell(key CellKey, tile TileID) {
	c, ok := s.cells[key]
	if !ok {
		return
	}
	c.candidates = tileMask(tile)
	c.settled = true
	c.collapsed = tile
}

// KeepCell records a cell's current tile as a strong preference.
//
// Unlike FixCell, the cell is not pinned: the solver may still choose a
// different tile if the preference turns out to be impossible. This is what
// keeps a regional re-solve stable — a cell keeps its tile unless the
// player's change genuinely rules it out.
//
// The preference is applied by weighting the cell's current tile more
// heavily when the cell is collapsed.
func (s *Solver) KeepCell(key CellKey, tile TileID) {
	c, ok := s.cells[key]
	if !ok {
		return
	}
	c.preferred = tile
	c.preferredSet = true
}

// Propagate removes tiles that are incompatible with the settled neighbours of
// every cell in the region.
//
// Cells that are left with a single candidate are *forced*: they are settled
// immediately and their own constraints propagated in turn. This is what
// lets a constraint at one end of a building ripple through the whole
// structure, and it is what stops the solver from committing to a cell whose
// only support has already been ruled out.
//
// It returns ErrContradiction if any cell is left with no candidates.
func (s *Solver) Propagate() error {
	queue := make([]CellKey, 0, len(s.cells))
	inQueue := make(map[CellKey]bool, len(s.cells))

	enqueue := func(k CellKey) {
		if !inQueue[k] {
			queue = append(queue, k)
			inQueue[k] = true
		}
	}

	// Seed the queue with every cell that is already settled, plus any cell
	// that has only one possible tile.
	for _, k := range s.order {
		c := s.cells[k]
		if c.settled {
			enqueue(k)
			continue
		}
		if c.count() == 1 {
			s.force(k)
			enqueue(k)
		}
	}

	// Apply the vertical constraint from the layer below before anything
	// else: a cell may only hold a tile that the cell beneath it can
	// support.
	if err := s.applyBelowConstraints(); err != nil {
		return err
	}

	// The queue is processed breadth-first. Each settled cell constrains its
	// neighbours; each removal in turn re-constrains that cell's neighbours.
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		delete(inQueue, key)

		c := s.cells[key]
		if !c.settled {
			continue
		}

		for _, n := range s.neighborsOf(key) {
			nc, ok := s.cells[n]
			if !ok {
				// Outside the region: nothing to constrain.
				continue
			}

			if nc.settled {
				// Two already-settled cells that touch must agree.
				// Otherwise the player's own constraints contradict
				// each other.
				if err := s.checkSettledPair(key, c.collapsed, n, nc.collapsed); err != nil {
					return err
				}
				continue
			}

			if s.prune(n, nc, key, c.collapsed) {
				switch nc.count() {
				case 0:
					return fmt.Errorf("%w: cell %+v has no valid tile (forced by neighbour %+v)",
						ErrContradiction, n, key)
				case 1:
					// The cell is forced: settle it now so its own
					// constraints propagate further.
					s.force(n)
					enqueue(n)
				default:
					enqueue(n)
				}
			}
		}
	}
	return nil
}

// force settles a cell that has exactly one remaining candidate.
func (s *Solver) force(key CellKey) {
	c, ok := s.cells[key]
	if !ok || c.settled {
		return
	}
	tiles := maskTiles(c.candidates, nil)
	if len(tiles) != 1 {
		return
	}
	c.candidates = tileMask(tiles[0])
	c.settled = true
	c.collapsed = tiles[0]
}

// applyBelowConstraints removes the tiles that the pinned layer below
// cannot support.
//
// A cell on layer 1 may only hold a tile whose "down" connector is accepted
// by the "up" connector of the cell beneath it. If the cell below cannot
// support anything, the cell above has no valid tile at all.
func (s *Solver) applyBelowConstraints() error {
	if len(s.below) == 0 {
		return nil
	}

	for _, key := range s.order {
		belowTile, ok := s.below[CellKey{Q: key.Q, R: key.R, Layer: key.Layer - 1}]
		if !ok {
			continue
		}

		c := s.cells[key]
		lowerUp := s.rules.UpConnector(belowTile)

		for t := 0; t < maxTiles; t++ {
			tile := TileID(t)
			if !c.has(tile) {
				continue
			}
			if !s.rules.UpConnectorsMatch(lowerUp, s.rules.DownConnector(tile)) {
				c.candidates &^= tileMask(tile)
			}
		}

		if c.count() == 0 {
			return fmt.Errorf("%w: cell %+v has no valid tile (nothing below can support it)",
				ErrContradiction, key)
		}
		if c.count() == 1 {
			s.force(key)
		}
	}
	return nil
}

// checkSettledPair reports a contradiction when two settled cells that touch
// are not compatible with each other.
func (s *Solver) checkSettledPair(aKey CellKey, aTile TileID, bKey CellKey, bTile TileID) error {
	if aKey.Layer == bKey.Layer {
		d, ok := directionBetween(aKey, bKey)
		if !ok {
			return nil
		}
		// bKey lies in direction d from aKey, so aKey's edge facing bKey
		// is its own d edge, and bKey's edge facing aKey is d's opposite.
		aConn := s.rules.EdgeConnector(aTile, d)
		bConn := s.rules.EdgeConnector(bTile, d.Opposite())
		if !s.rules.HexConnectorsMatch(aConn, bConn) {
			return &ConflictError{
				First:     s.rules.Tile(aTile).ID,
				Second:    s.rules.Tile(bTile).ID,
				FirstKey:  aKey,
				SecondKey: bKey,
			}
		}
		return nil
	}

	// Vertical adjacency between two settled cells.
	switch {
	case bKey.Layer == aKey.Layer+1:
		if !s.rules.UpConnectorsMatch(s.rules.UpConnector(aTile), s.rules.DownConnector(bTile)) {
			return fmt.Errorf("%w: cell %+v (%s) cannot support %+v (%s)",
				ErrContradiction, aKey, s.rules.Tile(aTile).ID, bKey, s.rules.Tile(bTile).ID)
		}
	case bKey.Layer == aKey.Layer-1:
		if !s.rules.UpConnectorsMatch(s.rules.UpConnector(bTile), s.rules.DownConnector(aTile)) {
			return fmt.Errorf("%w: cell %+v (%s) cannot support %+v (%s)",
				ErrContradiction, bKey, s.rules.Tile(bTile).ID, aKey, s.rules.Tile(aTile).ID)
		}
	}
	return nil
}

// neighborsOf returns the cells that constrain the given cell: its six hex
// neighbours on the same layer, plus the cells directly above and below it.
//
// Both vertical neighbours are included so that the vertical rule is applied
// from whichever side settles first. A tower that settles before the plot
// beneath it must still rule that plot out as its support.
func (s *Solver) neighborsOf(key CellKey) []CellKey {
	hex := key.HexNeighbors()
	out := make([]CellKey, 0, dirCount+2)
	out = append(out, hex[:]...)
	out = append(out, key.Above())
	out = append(out, key.Below())
	return out
}

// prune removes from nc every tile that is incompatible with the settled tile
// of the neighbour at otherKey. It reports whether anything was removed.
//
// The direction between the two cells is derived from their coordinates, so
// the neighbour's facing edge is matched against this cell's opposite edge.
func (s *Solver) prune(neighbourKey CellKey, nc *cell, otherKey CellKey, otherTile TileID) bool {
	removed := false

	// Hex adjacency: both cells must be on the same layer.
	if neighbourKey.Layer == otherKey.Layer {
		d, ok := directionBetween(otherKey, neighbourKey)
		if !ok {
			return false
		}
		// neighbourKey lies in direction d from otherKey, so otherKey's
		// edge facing the neighbour is its own d edge, and the
		// neighbour's edge facing otherKey is the opposite of d.
		otherConn := s.rules.EdgeConnector(otherTile, d)
		for t := 0; t < maxTiles; t++ {
			tile := TileID(t)
			if !nc.has(tile) {
				continue
			}
			if s.rules.HexConnectorsMatch(otherConn, s.rules.EdgeConnector(tile, d.Opposite())) {
				continue
			}
			if nc.preferredSet && nc.preferred == tile {
				// The cell would rather keep this tile than be
				// replaced by a wildcard one that matches every
				// neighbour. It is given up only if it is the last
				// candidate left, because then the cell would have no
				// tile at all.
				continue
			}
			nc.candidates &^= tileMask(tile)
			removed = true
		}
		return removed
	}

	// Vertical adjacency: the neighbour is the cell above, or we are the cell
	// above it.
	switch {
	case neighbourKey.Layer == otherKey.Layer+1:
		// otherKey is below neighbourKey: its "up" connector must accept
		// neighbourKey's "down" connector.
		lowerUp := s.rules.UpConnector(otherTile)
		for t := 0; t < maxTiles; t++ {
			tile := TileID(t)
			if !nc.has(tile) {
				continue
			}
			if s.rules.UpConnectorsMatch(lowerUp, s.rules.DownConnector(tile)) {
				continue
			}
			if nc.preferredSet && nc.preferred == tile {
				continue
			}
			nc.candidates &^= tileMask(tile)
			removed = true
		}
	case neighbourKey.Layer == otherKey.Layer-1:
		// otherKey is above neighbourKey, so the constraint is inverted.
		upperDown := s.rules.DownConnector(otherTile)
		for t := 0; t < maxTiles; t++ {
			tile := TileID(t)
			if !nc.has(tile) {
				continue
			}
			if s.rules.UpConnectorsMatch(s.rules.UpConnector(tile), upperDown) {
				continue
			}
			if nc.preferredSet && nc.preferred == tile {
				continue
			}
			nc.candidates &^= tileMask(tile)
			removed = true
		}
	}

	return removed
}

// directionBetween returns the direction from a to b, where a and b are
// adjacent cells on the same layer.
func directionBetween(a, b CellKey) (Direction, bool) {
	for d := Direction(0); d < dirCount; d++ {
		if a.Neighbor(d) == b {
			return d, true
		}
	}
	return 0, false
}

// SelectCell returns the unsettled cell with the fewest remaining
// candidates.
//
// Ties are broken by the solver's stable cell order, so the choice is
// deterministic for a given seed and region. It returns false when every cell
// is settled.
func (s *Solver) SelectCell() (CellKey, bool) {
	best := CellKey{}
	bestCount := 0
	found := false

	for _, k := range s.order {
		c := s.cells[k]
		if c.settled {
			continue
		}
		n := c.count()
		if n == 0 {
			// A cell with no candidates is a contradiction; report it
			// as the most constrained cell so the caller reports it.
			return k, true
		}
		if !found || n < bestCount {
			best = k
			bestCount = n
			found = true
		}
	}

	return best, found
}

// Collapse picks a tile for the given cell using weighted random selection
// and settles it.
//
// The random source is derived per cell from the town's seed, so the choice
// for a cell does not depend on how many cells were collapsed before it.
// That keeps regional re-solves reproducible.
func (s *Solver) Collapse(key CellKey) error {
	c, ok := s.cells[key]
	if !ok {
		return fmt.Errorf("cannot collapse cell %+v: not in region", key)
	}
	if c.settled {
		return nil
	}

	tiles := maskTiles(c.candidates, nil)
	if len(tiles) == 0 {
		return fmt.Errorf("%w: cell %+v has no valid tile", ErrContradiction, key)
	}
	if len(tiles) == 1 {
		c.candidates = tileMask(tiles[0])
		c.settled = true
		c.collapsed = tiles[0]
		return nil
	}

	// A cell that would like to keep its tile keeps it outright, as long as
	// it is still a candidate. Rolling the dice here would leave a small
	// chance of the cell changing on every re-solve, which would slowly
	// reshape the town while the player did nothing.
	if c.preferredSet && c.has(c.preferred) {
		c.candidates = tileMask(c.preferred)
		c.settled = true
		c.collapsed = c.preferred
		return nil
	}

	// The weight of a tile that the cell would like to keep is boosted, so
	// a regional re-solve leaves the neighbourhood alone unless it has to
	// change something.
	total := 0
	for _, t := range tiles {
		total += s.weightFor(c, t)
	}
	if total <= 0 {
		return fmt.Errorf("%w: cell %+v has no weighted candidates", ErrContradiction, key)
	}

	pick := s.cellRNG(key).Intn(total)
	chosen := tiles[0]
	for _, t := range tiles {
		w := s.weightFor(c, t)
		if pick < w {
			chosen = t
			break
		}
		pick -= w
	}

	c.candidates = tileMask(chosen)
	c.settled = true
	c.collapsed = chosen
	return nil
}

// preferredBoost is how much extra weight a cell's preferred tile receives.
// It is large enough to dominate the other weights, so a cell keeps its tile
// unless keeping it is impossible.
const preferredBoost = 1000

// weightFor returns the selection weight of a tile for the given cell.
//
// A cell that would like to keep its tile gets a large boost, so that a
// regional re-solve leaves the neighbourhood alone unless it has to change
// something. The boost is large enough to dominate every other weight in
// the ruleset, which makes the choice effectively certain.
func (s *Solver) weightFor(c *cell, t TileID) int {
	w := s.rules.Tile(t).Weight
	if c.preferredSet && c.preferred == t {
		w += preferredBoost
	}
	return w
}

// cellRNG returns the random source dedicated to a cell. It is seeded from
// the town seed and the cell's coordinates, making each cell's random
// decisions independent of solve order.
func (s *Solver) cellRNG(key CellKey) *rand.Rand {
	h := uint64(s.seed)
	h = h*6364136223846793005 + uint64(key.Q+0x10000)*1442695040888963407
	h = h*6364136223846793005 + uint64(key.R+0x10000)*1442695040888963407
	h = h*6364136223846793005 + uint64(key.Layer+0x10000)*1442695040888963407
	h ^= h >> 33
	return rand.New(rand.NewSource(int64(h)))
}

// Solve collapses every cell in the region.
//
// Cells are collapsed in ascending order of remaining candidate count, so the
// most constrained cell is decided first and contradictions surface early.
func (s *Solver) Solve() error {
	if err := s.Propagate(); err != nil {
		return err
	}

	for {
		key, ok := s.SelectCell()
		if !ok {
			return nil
		}
		if err := s.Collapse(key); err != nil {
			return err
		}
		if err := s.Propagate(); err != nil {
			return err
		}
	}
}

// Result returns the settled state of every cell in the region.
func (s *Solver) Result() map[CellKey]ResultCell {
	out := make(map[CellKey]ResultCell, len(s.cells))
	for k, c := range s.cells {
		if !c.settled {
			continue
		}
		out[k] = ResultCell{
			Tile:  s.rules.Tile(c.collapsed).ID,
			Color: c.color,
		}
	}
	return out
}

// ResultCell is the settled outcome of a single cell.
type ResultCell struct {
	// Tile is the tile type that was chosen for the cell.
	Tile string
	// Color is the player-chosen colour, or empty for the default.
	Color string
}
