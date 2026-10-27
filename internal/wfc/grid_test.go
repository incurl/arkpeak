package wfc

import (
	"fmt"
	"testing"
)

func TestHexDirectionsAreDistinct(t *testing.T) {
	seen := make(map[CellKey]bool, dirCount)
	for d := Direction(0); d < dirCount; d++ {
		k := CellKey{}.Neighbor(d)
		if seen[k] {
			t.Errorf("direction %s duplicates another direction", d)
		}
		seen[k] = true
	}
	if len(seen) != dirCount {
		t.Errorf("expected %d distinct neighbour offsets, got %d", dirCount, len(seen))
	}
}

func TestNeighborOpposite(t *testing.T) {
	for d := Direction(0); d < dirCount; d++ {
		if got := d.Opposite(); got == d {
			t.Errorf("direction %s is its own opposite", d)
		}
		if d.Opposite().Opposite() != d {
			t.Errorf("opposite of %s is not an involution", d)
		}
	}
}

func TestNeighborRoundTrip(t *testing.T) {
	origin := CellKey{Q: 3, R: -2, Layer: 1}
	for d := Direction(0); d < dirCount; d++ {
		n := origin.Neighbor(d)
		if n.Layer != origin.Layer {
			t.Errorf("hex neighbour changed layer: %d -> %d", origin.Layer, n.Layer)
		}
		// Stepping in direction d then in the opposite direction must
		// return to the origin.
		if back := n.Neighbor(d.Opposite()); back != origin {
			t.Errorf("round trip via %s gave %+v, want %+v", d, back, origin)
		}
	}
}

func TestAboveBelow(t *testing.T) {
	k := CellKey{Q: 4, R: 5, Layer: 2}
	if got := k.Above(); got != (CellKey{Q: 4, R: 5, Layer: 3}) {
		t.Errorf("Above() = %+v, want layer 3", got)
	}
	if got := k.Below(); got != (CellKey{Q: 4, R: 5, Layer: 1}) {
		t.Errorf("Below() = %+v, want layer 1", got)
	}
}

func TestAxialDistance(t *testing.T) {
	tests := []struct {
		name string
		a, b CellKey
		want int
	}{
		{"same cell", CellKey{Q: 0, R: 0}, CellKey{Q: 0, R: 0}, 0},
		{"one step north", CellKey{Q: 0, R: 0}, CellKey{Q: 0, R: -1}, 1},
		{"one step south", CellKey{Q: 0, R: 0}, CellKey{Q: 0, R: 1}, 1},
		{"one step ne", CellKey{Q: 0, R: 0}, CellKey{Q: 1, R: -1}, 1},
		{"one step se", CellKey{Q: 0, R: 0}, CellKey{Q: 1, R: 0}, 1},
		{"one step sw", CellKey{Q: 0, R: 0}, CellKey{Q: -1, R: 1}, 1},
		{"one step nw", CellKey{Q: 0, R: 0}, CellKey{Q: -1, R: 0}, 1},
		{"two steps", CellKey{Q: 0, R: 0}, CellKey{Q: 0, R: -2}, 2},
		{"diagonal", CellKey{Q: 0, R: 0}, CellKey{Q: 2, R: -2}, 2},
		// dq=-20, dr=8, ds=-12 => (20+8+12)/2 = 20.
		{"far apart", CellKey{Q: -10, R: 4}, CellKey{Q: 10, R: -4}, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AxialDistance(tt.a, tt.b); got != tt.want {
				t.Errorf("AxialDistance(%+v, %+v) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestAxialDistanceIsSymmetric(t *testing.T) {
	a := CellKey{Q: 3, R: -1}
	b := CellKey{Q: -2, R: 5}
	if AxialDistance(a, b) != AxialDistance(b, a) {
		t.Error("AxialDistance is not symmetric")
	}
}

func TestAxialDistanceIgnoresLayer(t *testing.T) {
	a := CellKey{Q: 1, R: 1, Layer: 0}
	b := CellKey{Q: 1, R: 1, Layer: 5}
	if got := AxialDistance(a, b); got != 0 {
		t.Errorf("AxialDistance across layers = %d, want 0", got)
	}
}

func TestInRadius(t *testing.T) {
	center := CellKey{Q: 0, R: 0, Layer: 0}
	tests := []struct {
		target CellKey
		radius int
		want   bool
	}{
		{CellKey{Q: 0, R: 0, Layer: 0}, 0, true},
		{CellKey{Q: 0, R: 0, Layer: 0}, 1, true},
		{CellKey{Q: 1, R: 0, Layer: 0}, 1, true},
		{CellKey{Q: 2, R: 0, Layer: 0}, 1, false},
		{CellKey{Q: 2, R: 0, Layer: 0}, 2, true},
		{CellKey{Q: 0, R: 0, Layer: 3}, 0, true},
	}
	for _, tt := range tests {
		if got := InRadius(center, tt.target, tt.radius); got != tt.want {
			t.Errorf("InRadius(center, %+v, %d) = %v, want %v", tt.target, tt.radius, got, tt.want)
		}
	}
}

func TestDirectionBetween(t *testing.T) {
	origin := CellKey{Q: 2, R: 2}
	for d := Direction(0); d < dirCount; d++ {
		got, ok := directionBetween(origin, origin.Neighbor(d))
		if !ok {
			t.Errorf("directionBetween failed for direction %s", d)
			continue
		}
		if got != d {
			t.Errorf("directionBetween returned %s, want %s", got, d)
		}
	}

	if _, ok := directionBetween(origin, CellKey{Q: 9, R: 9}); ok {
		t.Error("directionBetween reported a direction for non-adjacent cells")
	}
}

func TestHexRegion(t *testing.T) {
	tests := []struct {
		radius int
		want   int
	}{
		{0, 1},
		{1, 7},
		{2, 19},
		{3, 37},
		{24, 3*24*24 + 3*24 + 1},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("radius=%d", tt.radius), func(t *testing.T) {
			keys := HexRegion(tt.radius)
			if len(keys) != tt.want {
				t.Errorf("HexRegion(%d) returned %d cells, want %d", tt.radius, len(keys), tt.want)
			}

			// Every cell must be inside the region, and no cell may
			// appear twice.
			seen := make(map[CellKey]bool, len(keys))
			for _, k := range keys {
				if !InHexRegion(tt.radius, k) {
					t.Errorf("HexRegion(%d) returned %+v, which is outside the region", tt.radius, k)
				}
				if seen[k] {
					t.Errorf("HexRegion(%d) returned %+v twice", tt.radius, k)
				}
				seen[k] = true
			}

			// The region must be centred: the origin is included and
			// every cell is within the radius.
			if !seen[CellKey{}] {
				t.Errorf("HexRegion(%d) does not include the origin", tt.radius)
			}
		})
	}
}

func TestHexRegionIsSymmetric(t *testing.T) {
	const radius = 5
	keys := HexRegion(radius)
	seen := make(map[CellKey]bool, len(keys))
	for _, k := range keys {
		seen[k] = true
	}

	// Count how many neighbours each cell has outside the region.
	//
	// A hexagon of radius r has six corners, each with three neighbours
	// outside, and 6(r-1) other edge cells, each with two. The interior
	// cells have none.
	outsideLinks := 0
	cornerCells := 0
	edgeCells := 0

	for _, k := range keys {
		out := 0
		for d := Direction(0); d < dirCount; d++ {
			if !seen[k.Neighbor(d)] {
				out++
			}
		}
		outsideLinks += out

		switch out {
		case 0:
		case 2:
			edgeCells++
		case 3:
			cornerCells++
		default:
			t.Errorf("cell %+v has %d neighbours outside the region, want 0, 2 or 3", k, out)
		}

		// If a cell's neighbour is inside the region, the cell must be
		// inside it too.
		for d := Direction(0); d < dirCount; d++ {
			if seen[k.Neighbor(d)] && !InHexRegion(radius, k) {
				t.Errorf("cell %+v is outside the region but its neighbour %+v is inside", k, k.Neighbor(d))
			}
		}
	}

	if want := 6; cornerCells != want {
		t.Errorf("HexRegion(%d) has %d corner cells, want %d", radius, cornerCells, want)
	}
	if want := 6 * (radius - 1); edgeCells != want {
		t.Errorf("HexRegion(%d) has %d edge cells, want %d", radius, edgeCells, want)
	}
	if want := 6*3 + 6*(radius-1)*2; outsideLinks != want {
		t.Errorf("HexRegion(%d) has %d neighbour links leaving the region, want %d",
			radius, outsideLinks, want)
	}
}

func TestInHexRegion(t *testing.T) {
	const radius = 3
	tests := []struct {
		key  CellKey
		want bool
	}{
		{CellKey{}, true},
		{CellKey{Q: 3, R: 0}, true},
		{CellKey{Q: 4, R: 0}, false},
		{CellKey{Q: 0, R: 3}, true},
		{CellKey{Q: 0, R: 4}, false},
		{CellKey{Q: -3, R: 0}, true},
		{CellKey{Q: -4, R: 0}, false},
		// The layer does not affect the footprint.
		{CellKey{Q: 0, R: 0, Layer: 3}, true},
	}

	for _, tt := range tests {
		if got := InHexRegion(radius, tt.key); got != tt.want {
			t.Errorf("InHexRegion(%d, %+v) = %v, want %v", radius, tt.key, got, tt.want)
		}
	}
}
