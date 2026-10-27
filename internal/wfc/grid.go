// Package wfc implements the Wave Function Collapse solver that generates
// ArkPeak's towns.
//
// The solver is deliberately pure: it knows nothing about rendering, the
// frontend, or Wails. It consumes a Ruleset and a set of Constraints and
// produces a settled grid. See Ruleset for the tile rule schema and Solver
// for the collapse loop.
package wfc

// Direction identifies one of the six sides of a hexagonal cell.
type Direction int

// The six hexagonal directions, in clockwise order starting from north.
// The order is significant: it is used to index the per-tile edge connector
// arrays, so it must not be permuted without updating the ruleset loader.
const (
	DirN Direction = iota
	DirNE
	DirSE
	DirS
	DirSW
	DirNW

	// dirCount is the number of hex directions. It is not a Direction.
	dirCount = 6
)

// DirectionNames maps a direction to its human-readable name. Used in error
// messages and test failures.
var DirectionNames = [dirCount]string{"N", "NE", "SE", "S", "SW", "NW"}

// Opposite returns the direction facing the given one.
func (d Direction) Opposite() Direction {
	return (d + dirCount/2) % dirCount
}

// String returns the human-readable name of the direction.
func (d Direction) String() string {
	if d < 0 || d >= dirCount {
		return "invalid"
	}
	return DirectionNames[d]
}

// HexDirs are the six axial direction vectors, indexed by Direction.
//
// These are the standard "pointy-top" axial coordinates: E, SE, SW, W, NW, NE.
// The exact ordering matches the Direction constants above.
var HexDirs = [dirCount][2]int{
	DirN:  {0, -1},
	DirNE: {1, -1},
	DirSE: {1, 0},
	DirS:  {0, 1},
	DirSW: {-1, 1},
	DirNW: {-1, 0},
}

// CellKey uniquely identifies a cell in the town grid.
//
// Q and R are axial hex coordinates within a layer; Layer is the vertical
// (storey) axis, where 0 is the ground layer.
type CellKey struct {
	Q     int `json:"q"`
	R     int `json:"r"`
	Layer int `json:"layer"`
}

// Neighbor returns the key of the cell in direction d within the same layer.
func (k CellKey) Neighbor(d Direction) CellKey {
	return CellKey{
		Q:     k.Q + HexDirs[d][0],
		R:     k.R + HexDirs[d][1],
		Layer: k.Layer,
	}
}

// HexNeighbors returns the six keys adjacent to k within the same layer, in
// Direction order.
func (k CellKey) HexNeighbors() [dirCount]CellKey {
	var out [dirCount]CellKey
	for d := Direction(0); d < dirCount; d++ {
		out[d] = k.Neighbor(d)
	}
	return out
}

// Above returns the key of the cell directly above k in the next layer.
func (k CellKey) Above() CellKey {
	return CellKey{Q: k.Q, R: k.R, Layer: k.Layer + 1}
}

// Below returns the key of the cell directly below k in the previous layer.
func (k CellKey) Below() CellKey {
	return CellKey{Q: k.Q, R: k.R, Layer: k.Layer - 1}
}

// AxialDistance returns the hex distance between two cells, ignoring the
// layer axis. The distance is the number of steps between the cells.
//
// This is the standard cube-coordinate distance formula: the third cube
// coordinate is s = -q - r, so the distance is
// (|dq| + |dr| + |dq+dr|) / 2.
func AxialDistance(a, b CellKey) int {
	dq := a.Q - b.Q
	dr := a.R - b.R
	ds := dq + dr

	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}

	return (abs(dq) + abs(dr) + abs(ds)) / 2
}

// InRadius reports whether target lies within radius steps of center, ignoring
// the layer axis. A radius of 0 means the cell itself.
func InRadius(center, target CellKey, radius int) bool {
	return AxialDistance(center, target) <= radius
}
