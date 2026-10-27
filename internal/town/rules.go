package town

import (
	_ "embed"
	"fmt"

	"github.com/incurl/arkpeak/internal/wfc"
)

//go:embed rules/town_rules.json
var rulesJSON []byte

// LoadEmbeddedRules compiles the tile ruleset that is embedded in the
// binary.
//
// Embedding the ruleset means the shipped application always carries the
// rules it was built with, and the solver never depends on a file being
// present at runtime.
func LoadEmbeddedRules() (*wfc.Ruleset, error) {
	rules, err := wfc.CompileRulesetJSON(rulesJSON)
	if err != nil {
		return nil, fmt.Errorf("compile embedded ruleset: %w", err)
	}
	return rules, nil
}
