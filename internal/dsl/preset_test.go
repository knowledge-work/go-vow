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

// TestParseRejectsMalformedPresets pins each shape check Parse adds on
// top of YAML decoding. want is a fragment of the message the check
// writes, so a case whose input trips an earlier check instead fails.
func TestParseRejectsMalformedPresets(t *testing.T) {
	const header = "name: p\nsubjects:\n  - match: \"annotation:vow:define @Foo\"\n"
	type malformedCase struct {
		raw  string
		want string
	}
	tabletest.Run(t, map[string]malformedCase{
		"misspelled top-level key": {
			raw:  header + "obligations:\n  - type: must-close\ndescripton: x\n",
			want: "field descripton not found",
		},
		"severity vow does not read": {
			raw:  header + "obligations:\n  - type: must-close\nseverity: error\n",
			want: `severity "error": vow reads no severity`,
		},
		"subject in another scheme": {
			raw:  "name: p\nsubjects:\n  - match: \"name:Foo*\"\nobligations:\n  - type: must-close\n",
			want: `match "name:Foo*" does not name a marker after annotation:`,
		},
		"unknown obligation type": {
			raw:  header + "obligations:\n  - type: must-consum\n",
			want: "(must-consum): not an obligation type vow reads",
		},
		"must-consume without consumers": {
			raw:  header + "obligations:\n  - type: must-consume\n",
			want: "detail.consumers must be a list, each entry a function name",
		},
		"must-consume with a misspelled detail key": {
			raw:  header + "obligations:\n  - type: must-consume\n    detail:\n      consumer: [errors.Is]\n",
			want: `detail has unknown key "consumer"`,
		},
		"must-consume with a consumer that is not a name": {
			raw:  header + "obligations:\n  - type: must-consume\n    detail:\n      consumers: [errors.Is, {}]\n",
			want: "detail.consumers[1] must be a function name without spaces",
		},
		"must-consume with a consumer holding a space": {
			raw:  header + "obligations:\n  - type: must-consume\n    detail:\n      consumers: [\" errors.Is\"]\n",
			want: "detail.consumers[0] must be a function name without spaces",
		},
		"passthrough-emit without functions": {
			raw:  header + "obligations:\n  - type: passthrough-emit\n    detail: {}\n",
			want: "detail.functions must be a list of wrap signatures",
		},
		"passthrough-emit entry that is not a map": {
			raw:  header + "obligations:\n  - type: passthrough-emit\n    detail:\n      functions: [pkg.Wrap]\n",
			want: "detail.functions[0] must be a map",
		},
		"passthrough-emit entry with a misspelled key": {
			raw:  header + "obligations:\n  - type: passthrough-emit\n    detail:\n      functions:\n        - {pattern: pkg.Wrap, arg-position: 0, return_slot: 1}\n",
			want: `detail.functions[0] has unknown key "return_slot"`,
		},
		"passthrough-emit entry without a pattern": {
			raw:  header + "obligations:\n  - type: passthrough-emit\n    detail:\n      functions:\n        - {arg-position: 0, return-slot: 1}\n",
			want: "detail.functions[0].pattern must be a qualified function name without spaces",
		},
		"passthrough-emit entry with a pattern holding a space": {
			raw:  header + "obligations:\n  - type: passthrough-emit\n    detail:\n      functions:\n        - {pattern: \"pkg.Wrap \", arg-position: 0, return-slot: 1}\n",
			want: "detail.functions[0].pattern must be a qualified function name without spaces",
		},
		"passthrough-emit entry with a fractional arg-position": {
			raw:  header + "obligations:\n  - type: passthrough-emit\n    detail:\n      functions:\n        - {pattern: pkg.Wrap, arg-position: 0.9, return-slot: 1}\n",
			want: "detail.functions[0].arg-position must be a 0-based parameter index",
		},
		"passthrough-emit entry with a negative arg-position": {
			raw:  header + "obligations:\n  - type: passthrough-emit\n    detail:\n      functions:\n        - {pattern: pkg.Wrap, arg-position: -1, return-slot: 1}\n",
			want: "detail.functions[0].arg-position must be a 0-based parameter index",
		},
		"passthrough-emit entry with return-slot 0": {
			raw:  header + "obligations:\n  - type: passthrough-emit\n    detail:\n      functions:\n        - {pattern: pkg.Wrap, arg-position: 0, return-slot: 0}\n",
			want: "detail.functions[0].return-slot must be a 1-based return position",
		},
		"must-close with a misspelled detail key": {
			raw:  header + "obligations:\n  - type: must-close\n    detail:\n      discharge: [defer]\n",
			want: `detail has unknown key "discharge"`,
		},
		"must-close with discharges that are not a list": {
			raw:  header + "obligations:\n  - type: must-close\n    detail:\n      discharges: defer\n",
			want: "detail.discharges must be a list, each entry a discharge name",
		},
	}, func(t *testing.T, c malformedCase) {
		_, err := Parse([]byte(c.raw))
		assert.ErrorContains(t, "Parse", err, c.want)
	})
}

// TestParseAcceptsWellFormedPresets is the control for
// TestParseRejectsMalformedPresets: each case is one edit away from a
// rejected one.
func TestParseAcceptsWellFormedPresets(t *testing.T) {
	const header = "name: p\nsubjects:\n  - match: \"annotation:vow:define @Foo\"\n"
	tabletest.Run(t, map[string]string{
		"must-consume with consumers":   header + "obligations:\n  - type: must-consume\n    detail:\n      consumers: [errors.Is, mylib.IsMiss]\n",
		"must-consume with no consumer": header + "obligations:\n  - type: must-consume\n    detail:\n      consumers: []\n",
		"passthrough-emit":              header + "obligations:\n  - type: passthrough-emit\n    detail:\n      functions:\n        - {pattern: pkg.Wrap, arg-position: 0, return-slot: 1}\n",
		"must-close without detail":     header + "obligations:\n  - type: must-close\n",
		"severity warning":              header + "obligations:\n  - type: must-close\nseverity: warning\n",
	}, func(t *testing.T, raw string) {
		_, err := Parse([]byte(raw))
		assert.NoError(t, "Parse", err)
	})
}

// TestShippedPresetsParse loads every preset file the repository ships.
func TestShippedPresetsParse(t *testing.T) {
	tabletest.Run(t, map[string]string{
		"sentinel-error": filepath.Join("..", "..", "preset", "sentinel-error.yaml"),
		"closable":       filepath.Join("..", "..", "preset", "closable.yaml"),
		"std/result":     filepath.Join("..", "..", "preset", "std", "result.yaml"),
		"example werror": filepath.Join("..", "..", "example", "werror.yaml"),
	}, func(t *testing.T, path string) {
		_, err := Load(path)
		assert.NoError(t, "Load", err)
	})
}

func TestSubjectConcept(t *testing.T) {
	type conceptCase struct {
		match string
		want  string
		ok    bool
	}
	tabletest.Run(t, map[string]conceptCase{
		"vow:define names the concept": {match: "annotation:vow:define @Foo", want: "Foo", ok: true},
		"space after the scheme":       {match: "annotation: vow:define @Foo", want: "Foo", ok: true},
		"another marker":               {match: "annotation:vow:other @Foo"},
		"no concept name":              {match: "annotation:vow:define @"},
		"trailing token":               {match: "annotation:vow:define @Foo bar"},
		"another scheme":               {match: "name:Foo"},
	}, func(t *testing.T, c conceptCase) {
		got, ok := Subject{Match: c.match}.Concept()
		assert.Equal(t, "Concept() name", got, c.want)
		assert.Equal(t, "Concept() ok", ok, c.ok)
	})
}
