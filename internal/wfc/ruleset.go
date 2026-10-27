package wfc

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// TileID identifies a tile type within a Ruleset. It is the index of the tile
// in the ruleset's ordered tile list, so it is stable for a given ruleset.
type TileID int

// Axis identifies which kind of adjacency a compatibility rule governs.
type Axis string

const (
	// AxisHex is adjacency between two cells in the same layer, across a
	// shared hex edge.
	AxisHex Axis = "hex"
	// AxisUp is adjacency between a cell and the cell directly above it.
	AxisUp Axis = "up"
)

// Connector values with special meaning.
const (
	// ConnectorAny is a wildcard: it matches any other connector. It is
	// used for the edges of tiles that are indifferent to their
	// neighbours, such as an empty plot or a floor.
	ConnectorAny = "any"

	// ConnectorNothing matches nothing at all. It is used for a tile that
	// cannot support anything above it, such as a roof.
	ConnectorNothing = "nothing"
)

// maxTiles is the maximum number of tile types a ruleset may define. The
// solver stores candidate sets as bitmasks, so this is bounded by the width
// of the mask type.
const maxTiles = 64

// TileDef is the definition of a single tile type.
//
// Each tile declares a connector for every hex edge, one for the cell above
// it ("up") and one for the cell below it ("down"). Two tiles may touch when
// their facing connectors form a *compatible pair*: the pair must appear in
// the ruleset's connector table, or one of them must be ConnectorAny.
//
// This is a single unified mechanism. There is no separate list of
// "allowed tile pairs" — the connector values carry that meaning, so a
// ruleset author controls adjacency purely by choosing connector names.
type TileDef struct {
	// ID is the stable string identifier for the tile, e.g. "wall".
	ID string `json:"id"`
	// Weight controls how likely the solver is to pick this tile. Higher
	// weight means more likely. Must be > 0.
	Weight int `json:"weight"`
	// Layer restricts the tile to a specific layer index. Use "*" for any
	// layer. Ground-level tiles use 0.
	Layer string `json:"layer"`
	// Edges holds the hex connector for each of the six directions. The
	// JSON object keys are direction names: "n", "ne", "se", "s", "sw", "nw".
	Edges map[string]string `json:"edges"`
	// Up is the connector this tile presents to the cell above it: what
	// may rest on this tile.
	Up string `json:"up"`
	// Down is the connector this tile requires from the cell below it:
	// what may support this tile.
	Down string `json:"down"`
	// Colorable reports whether the player may choose a color for this tile.
	Colorable bool `json:"colorable"`
	// Solid reports whether the tile occludes the cell above it, which the
	// renderer uses to decide whether to draw an upper storey.
	Solid bool `json:"solid"`
}

// ConnectorRule declares that two connector values may meet on a given axis.
//
// Rules are directional: "a" is the connector on the lower/original side and
// "b" the connector on the facing side. For hex rules the pair is also valid
// reversed, because adjacency is symmetric. For up rules it is not: "a" is
// the lower tile's connector and "b" the upper tile's connector.
type ConnectorRule struct {
	A    string `json:"a"`
	B    string `json:"b"`
	Axis Axis   `json:"axis"`
}

// RulesetSchema is the on-disk representation of a ruleset.
type RulesetSchema struct {
	Tiles []TileDef `json:"tiles"`
	// Connectors lists the connector pairs that may meet. It is the single
	// source of truth for adjacency.
	Connectors []ConnectorRule `json:"connectors"`
}

// Ruleset is a compiled, immutable set of tile rules.
//
// The compatibility data is flattened at load time into lookup tables so the
// solver's hot loop performs O(1) lookups rather than scanning rule lists.
type Ruleset struct {
	// Tiles is the ordered list of tile definitions. A TileID is an index
	// into this slice.
	Tiles []TileDef

	// byID maps a tile's string ID to its TileID.
	byID map[string]TileID

	// hexConnectors maps a connector pair to whether the two connectors may
	// meet across a hex edge. It is symmetric.
	hexConnectors map[connectorPair]bool

	// upConnectors maps a connector pair to whether the lower connector may
	// support the upper one. It is directional.
	upConnectors map[connectorPair]bool

	// layerTiles maps a layer key to the tiles valid on that layer. The
	// special key anyLayerKey holds tiles valid on every layer.
	layerTiles map[string][]TileID

	// schema is the ruleset as it was declared, kept so that the
	// connector table can be handed to the frontend unchanged.
	schema RulesetSchema
}

// connectorPair is an ordered pair of connector values.
type connectorPair struct {
	lower string
	upper string
}

// anyLayerKey is the Ruleset.layerTiles key for tiles valid on all layers.
const anyLayerKey = "*"

// DefaultLayer is the layer index used for ground-level tiles.
const DefaultLayer = 0

// LoadRuleset reads and compiles a ruleset from a JSON file on disk.
func LoadRuleset(path string) (*Ruleset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ruleset %q: %w", path, err)
	}
	return CompileRulesetJSON(data)
}

// CompileRulesetJSON parses and compiles a ruleset from raw JSON.
func CompileRulesetJSON(data []byte) (*Ruleset, error) {
	var schema RulesetSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("parse ruleset: %w", err)
	}
	return CompileRuleset(schema)
}

// CompileRuleset validates a ruleset schema and compiles it into the lookup
// tables used by the solver.
func CompileRuleset(schema RulesetSchema) (*Ruleset, error) {
	if len(schema.Tiles) == 0 {
		return nil, fmt.Errorf("ruleset contains no tiles")
	}
	if len(schema.Tiles) > maxTiles {
		return nil, fmt.Errorf("ruleset has %d tiles, maximum supported is %d", len(schema.Tiles), maxTiles)
	}

	r := &Ruleset{
		Tiles:         schema.Tiles,
		byID:          make(map[string]TileID, len(schema.Tiles)),
		hexConnectors: make(map[connectorPair]bool),
		upConnectors:  make(map[connectorPair]bool),
		layerTiles:    make(map[string][]TileID),
		schema:        schema,
	}

	for i, def := range schema.Tiles {
		if def.ID == "" {
			return nil, fmt.Errorf("tile %d has an empty id", i)
		}
		if _, dup := r.byID[def.ID]; dup {
			return nil, fmt.Errorf("duplicate tile id %q", def.ID)
		}
		if def.Weight <= 0 {
			return nil, fmt.Errorf("tile %q has non-positive weight %d", def.ID, def.Weight)
		}
		r.byID[def.ID] = TileID(i)

		// Every tile must declare all six edges; an omitted edge would
		// make the tile unable to connect to anything.
		for d := Direction(0); d < dirCount; d++ {
			name := edgeKey(d)
			if _, ok := def.Edges[name]; !ok {
				return nil, fmt.Errorf("tile %q is missing edge %q", def.ID, name)
			}
		}
	}

	// Compile the connector compatibility table.
	for i, rule := range schema.Connectors {
		if rule.Axis != AxisHex && rule.Axis != AxisUp {
			return nil, fmt.Errorf("connector rule %d has unknown axis %q", i, rule.Axis)
		}
		if rule.A == "" || rule.B == "" {
			return nil, fmt.Errorf("connector rule %d has an empty connector", i)
		}
		if rule.Axis == AxisHex {
			// Hex adjacency is symmetric.
			r.hexConnectors[connectorPair{rule.A, rule.B}] = true
			r.hexConnectors[connectorPair{rule.B, rule.A}] = true
		} else {
			// Vertical adjacency is directional.
			r.upConnectors[connectorPair{rule.A, rule.B}] = true
		}
	}

	// Group tiles by the layer they are allowed on.
	for i, def := range schema.Tiles {
		layer := def.Layer
		if layer == "" {
			layer = anyLayerKey
		}
		r.layerTiles[layer] = append(r.layerTiles[layer], TileID(i))
	}

	return r, nil
}

// edgeKey returns the JSON object key used for a direction's edge connector.
func edgeKey(d Direction) string {
	switch d {
	case DirN:
		return "n"
	case DirNE:
		return "ne"
	case DirSE:
		return "se"
	case DirS:
		return "s"
	case DirSW:
		return "sw"
	case DirNW:
		return "nw"
	}
	return ""
}

// Tile returns the definition of the given tile.
func (r *Ruleset) Tile(id TileID) TileDef {
	return r.Tiles[id]
}

// Lookup resolves a tile's string ID to its TileID.
func (r *Ruleset) Lookup(id string) (TileID, bool) {
	t, ok := r.byID[id]
	return t, ok
}

// MustLookup resolves a tile's string ID to its TileID, panicking if the
// tile does not exist. It is intended for tests and package-level constants.
func (r *Ruleset) MustLookup(id string) TileID {
	t, ok := r.byID[id]
	if !ok {
		panic("wfc: unknown tile id " + id)
	}
	return t
}

// TilesForLayer returns the tiles that may be placed on the given layer.
func (r *Ruleset) TilesForLayer(layer int) []TileID {
	if t, ok := r.layerTiles[fmt.Sprintf("%d", layer)]; ok {
		return t
	}
	return r.layerTiles[anyLayerKey]
}

// EdgeConnector returns the connector value a tile declares on one of its hex
// edges.
func (r *Ruleset) EdgeConnector(id TileID, d Direction) string {
	return r.Tiles[id].Edges[edgeKey(d)]
}

// UpConnector returns the connector a tile presents to the cell above it.
func (r *Ruleset) UpConnector(id TileID) string {
	return r.Tiles[id].Up
}

// DownConnector returns the connector a tile requires from the cell below it.
func (r *Ruleset) DownConnector(id TileID) string {
	return r.Tiles[id].Down
}

// HexConnectorsMatch reports whether two connectors may meet across a hex
// edge.
//
// A connector of ConnectorAny matches any other connector, which lets a tile
// sit next to anything. Otherwise the pair must be listed in the ruleset's
// connector table.
func (r *Ruleset) HexConnectorsMatch(a, b string) bool {
	if a == ConnectorAny || b == ConnectorAny {
		return true
	}
	if a == ConnectorNothing || b == ConnectorNothing {
		return false
	}
	return r.hexConnectors[connectorPair{a, b}]
}

// UpConnectorsMatch reports whether the lower tile's "up" connector may
// support the upper tile's "down" connector.
//
// The same wildcard rules apply: ConnectorAny matches anything, and
// ConnectorNothing matches nothing.
func (r *Ruleset) UpConnectorsMatch(lowerUp, upperDown string) bool {
	if lowerUp == ConnectorAny || upperDown == ConnectorAny {
		return true
	}
	if lowerUp == ConnectorNothing || upperDown == ConnectorNothing {
		return false
	}
	return r.upConnectors[connectorPair{lowerUp, upperDown}]
}

// TileIDs returns the string IDs of all tiles, in TileID order. It is used
// for error messages and for validating player input.
func (r *Ruleset) TileIDs() []string {
	out := make([]string, len(r.Tiles))
	for i, def := range r.Tiles {
		out[i] = def.ID
	}
	return out
}

// ConnectorRules returns the connector compatibility table as it was
// declared.
//
// The frontend needs it to decide which cells connect, and it must come
// from the ruleset rather than be restated there, so the rules stay the
// single source of truth.
func (r *Ruleset) ConnectorRules() []ConnectorRule {
	out := make([]ConnectorRule, 0, len(r.schema.Connectors))
	out = append(out, r.schema.Connectors...)
	return out
}

// ValidateTileID returns an error if the given tile ID is not in the ruleset.
func (r *Ruleset) ValidateTileID(id string) error {
	if _, ok := r.byID[id]; !ok {
		return fmt.Errorf("unknown tile %q (valid tiles: %s)", id, strings.Join(r.SortedTileIDs(), ", "))
	}
	return nil
}

// SortedTileIDs returns the tile IDs sorted alphabetically. Useful for
// deterministic error messages.
func (r *Ruleset) SortedTileIDs() []string {
	out := r.TileIDs()
	sort.Strings(out)
	return out
}
