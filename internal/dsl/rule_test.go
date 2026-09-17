package dsl

import (
	"path/filepath"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

func TestRuleDefExpandBody(t *testing.T) {
	// wantErr is the substring the failure must carry; an empty wantErr
	// means the expansion must succeed and produce want.
	type expandCase struct {
		params  []string
		body    string
		args    []string
		want    string
		wantErr string
	}

	tabletest.Run(t, map[string]expandCase{
		"substitutes every param": {
			params: []string{"T", "E"},
			body:   "(T!, nil) | (nil, E!)",
			args:   []string{"Result", "error"},
			want:   "(Result!, nil) | (nil, error!)",
		},
		"rejects a wrong-arity argument list": {
			params:  []string{"T"},
			body:    "T",
			args:    []string{"a", "b"},
			wantErr: "expects 1 type arguments",
		},
		"leaves a non-param identifier intact": {
			params: []string{"T"},
			body:   "errors.Is(_, T)",
			args:   []string{"ErrFoo"},
			want:   "errors.Is(_, ErrFoo)",
		},
		"does not rescan an arg that names a param": {
			params: []string{"T", "E"},
			body:   "T",
			args:   []string{"E", "Other"},
			want:   "E",
		},
		"leaves an identifier that merely starts with a param": {
			params: []string{"T"},
			body:   "Tname",
			args:   []string{"X"},
			want:   "Tname",
		},
		"matches a param case-sensitively": {
			params: []string{"T"},
			body:   "t T",
			args:   []string{"X"},
			want:   "t X",
		},
		"inserts an argument carrying brackets verbatim": {
			params: []string{"T"},
			body:   "(T, nil)",
			args:   []string{"map[string]int"},
			want:   "(map[string]int, nil)",
		},
		"substitutes params adjacent to punctuation": {
			params: []string{"L", "R"},
			body:   "(L!|R!)",
			args:   []string{"Foo", "Bar"},
			want:   "(Foo!|Bar!)",
		},
	}, func(t *testing.T, c expandCase) {
		got, err := RuleDef{Params: c.params, Body: c.body}.ExpandBody(c.args)
		if c.wantErr != "" {
			assert.MustError(t, "ExpandBody", err)
			assert.ErrorContains(t, "ExpandBody", err, c.wantErr)
			return
		}
		assert.MustNoError(t, "ExpandBody", err)
		assert.Equal(t, "ExpandBody", got, c.want)
	})
}

func TestPresetValidate(t *testing.T) {
	// wantAccepted says whether Load must succeed; when it must fail,
	// wantErr is the substring the failure must carry, and an empty
	// wantErr leaves the message unchecked.
	type presetCase struct {
		yaml         string
		wantAccepted bool
		wantErr      string
	}

	tabletest.Run(t, map[string]presetCase{
		"accepts a preset carrying only rules": {
			yaml: `
name: rule-only
rules:
  Either:
    params: [L, R]
    body: "(L! | R!)"
`,
			wantAccepted: true,
		},
		"rejects a rule with duplicate params": {
			yaml: `
name: bad
rules:
  Dup:
    params: [T, T]
    body: "T"
`,
			wantErr: "duplicate param",
		},
		"rejects a rule with an empty body": {
			yaml: `
name: bad
rules:
  Bad:
    params: [T]
    body: ""
`,
		},
	}, func(t *testing.T, c presetCase) {
		path := filepath.Join(t.TempDir(), "p.yaml")
		assert.MustNoError(t, "writeFile", writeFile(t, path, c.yaml))

		_, err := Load(path)
		if c.wantAccepted {
			assert.NoError(t, "Load", err)
			return
		}
		assert.MustError(t, "Load", err)
		if c.wantErr != "" {
			assert.ErrorContains(t, "Load", err, c.wantErr)
		}
	})
}
