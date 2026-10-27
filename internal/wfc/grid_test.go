package wfc

import (
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
