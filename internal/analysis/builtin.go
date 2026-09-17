package analysis

import (
	_ "embed"
	"fmt"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// builtinSentinelErrorYAML is the embedded copy of preset/sentinel-error.yaml
// shipped inside the binary so the analyzer is self-contained. The
// TestBuiltinMatchesReference test keeps it byte-identical to the
// repository-root reference file.
//
//go:embed builtin/sentinel-error.yaml
var builtinSentinelErrorYAML []byte

// builtinStdResultYAML is the embedded copy of preset/std/result.yaml,
// providing the OkErr / Either rule templates available to any
// function that imports them via `vow:import`.
//
//go:embed builtin/std/result.yaml
var builtinStdResultYAML []byte

// builtinClosableYAML is the embedded copy of preset/closable.yaml,
// supplying the built_in_closables list (standard-library and
// third-party types whose source the author cannot annotate
// directly) for the Closable lifecycle pass.
//
//go:embed builtin/closable.yaml
var builtinClosableYAML []byte

// builtinPresets returns the presets compiled into the analyzer binary.
// Callers must not mutate the returned slice.
func builtinPresets() []*dsl.Preset {
	sentinel, err := dsl.Parse(builtinSentinelErrorYAML)
	if err != nil {
		// The YAML is embedded at build time and covered by a test, so
		// reaching this branch indicates a programmer error.
		panic(fmt.Errorf("vow: embedded sentinel-error preset is invalid: %v", err))
	}
	closable, err := dsl.Parse(builtinClosableYAML)
	if err != nil {
		panic(fmt.Errorf("vow: embedded closable preset is invalid: %v", err))
	}
	return []*dsl.Preset{sentinel, closable}
}

// builtinStdPresets returns the rule-only presets that can be brought
// into scope via `vow:import <alias> "<path>"`. Keyed by the path the
// user writes — e.g. `preset/std/result` maps to the parsed std/result
// preset.
func builtinStdPresets() map[string]*dsl.Preset {
	p, err := dsl.Parse(builtinStdResultYAML)
	if err != nil {
		panic(fmt.Errorf("vow: embedded std/result preset is invalid: %v", err))
	}
	return map[string]*dsl.Preset{
		"preset/std/result": p,
	}
}
