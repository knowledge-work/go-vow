package dsl

import (
	"path/filepath"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

func TestLoadSentinelErrorPreset(t *testing.T) {
	p, err := Load(filepath.Join("..", "..", "preset", "sentinel-error.yaml"))
	assert.MustNoError(t, "Load", err)

	assert.Equal(t, "Name", p.Name, "sentinel-error")
	assert.MustLen(t, "Subjects", p.Subjects, 1)
	assert.Equal(t, "Subjects[0].AnnotationMarker()", p.Subjects[0].AnnotationMarker(), "vow:define @Sentinel")
	assert.MustLen(t, "Obligations", p.Obligations, 1)
	assert.DeepEqual(t, "MustConsumeConsumers()", p.Obligations[0].MustConsumeConsumers(), []string{"errors.Is", "errors.As"})
}

func TestLoadRejectsEmptyName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.yaml")
	assert.MustNoError(t, "write fixture",
		writeFile(t, path, "name: \"\"\nsubjects: [{match: x}]\nobligations: [{type: y}]\n"))
	_, err := Load(path)
	assert.MustError(t, "Load of a preset with an empty name", err)
}

// TestPassthroughEmitFunctions pins the YAML-to-struct contract
// the obligation helper drives. The happy path picks up a
// fully-populated entry; the noise branches show that a partial
// or wrongly-typed entry drops silently so a single malformed
// entry does not erase the rest of the list.
func TestPassthroughEmitFunctions(t *testing.T) {
	o := Obligation{
		Type: "passthrough-emit",
		Detail: map[string]any{
			"functions": []any{
				map[string]any{
					"pattern":      "pkg.Wrap",
					"arg-position": 0,
					"return-slot":  1,
				},
				map[string]any{
					"pattern":      "",
					"arg-position": 0,
					"return-slot":  1,
				},
				map[string]any{
					"pattern":      "pkg.NegSlot",
					"arg-position": 0,
					"return-slot":  0,
				},
				map[string]any{
					"pattern":      "pkg.NegArg",
					"arg-position": -1,
					"return-slot":  1,
				},
				"not-a-map",
				map[string]any{
					"pattern":      "pkg.Errorf",
					"arg-position": int64(1),
					"return-slot":  float64(2),
				},
			},
		},
	}
	assert.DeepEqual(t, "PassthroughEmitFunctions()", o.PassthroughEmitFunctions(), []PassthroughEmitFunction{
		{Pattern: "pkg.Wrap", ArgPosition: 0, ReturnSlot: 1},
		{Pattern: "pkg.Errorf", ArgPosition: 1, ReturnSlot: 2},
	})
}

// TestPassthroughEmitFunctions_wrongType pins the silent nil
// path the helper takes when the obligation type is something
// other than passthrough-emit, or when the Detail map carries no
// functions key, or carries a key whose value is not a list.
func TestPassthroughEmitFunctions_wrongType(t *testing.T) {
	tabletest.Run(t, map[string]Obligation{
		"other type":            {Type: "must-consume", Detail: map[string]any{"functions": []any{}}},
		"missing functions key": {Type: "passthrough-emit", Detail: map[string]any{"other": 1}},
		"functions not a list":  {Type: "passthrough-emit", Detail: map[string]any{"functions": "scalar"}},
	}, func(t *testing.T, o Obligation) {
		assert.Nil(t, "PassthroughEmitFunctions()", o.PassthroughEmitFunctions())
	})
}
