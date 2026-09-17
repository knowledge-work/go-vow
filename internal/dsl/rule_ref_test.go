package dsl

import (
	"fmt"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

func TestParseRuleRefAt_aliased(t *testing.T) {
	s := "@result.OkErr[Result, error] tail"
	ref, n, err := parseRuleRefAt(s)
	assert.MustNoError(t, "parseRuleRefAt", err)
	assert.Equal(t, "Alias", ref.Alias, "result")
	assert.Equal(t, "RuleName", ref.RuleName, "OkErr")
	assert.DeepEqual(t, "TypeArgs", ref.TypeArgs, []string{"Result", "error"})
	assert.Equal(t, "consumed", n, len("@result.OkErr[Result, error]"))
}

func TestParseRuleRefAt_local(t *testing.T) {
	ref, _, err := parseRuleRefAt("@Either[int, string]")
	assert.MustNoError(t, "parseRuleRefAt", err)
	assert.Equal(t, "Alias", ref.Alias, "")
	assert.Equal(t, "RuleName", ref.RuleName, "Either")
}

func TestParseRuleRefAt_emptyArgs(t *testing.T) {
	// @rule[] is the canonical reference shape for zero-param rules.
	// The parser accepts the empty argument list; the arity mismatch
	// error is surfaced later by ExpandBody if the rule actually
	// expects parameters. ZeroParam below is a synthetic reference
	// shape — the parser does not consult the rule scope, so the
	// rule name need not exist in any preset to exercise this path.
	ref, _, err := parseRuleRefAt("@result.ZeroParam[]")
	assert.MustNoError(t, "parseRuleRefAt", err)
	assert.Equal(t, "Alias", ref.Alias, "result")
	assert.Equal(t, "RuleName", ref.RuleName, "ZeroParam")
	assert.Len(t, "TypeArgs", ref.TypeArgs, 0)
}

// TestParseRuleRefAt_zeroArgBare pins the bare-form admission for
// built-in zero-arg rules. A bare reference (no alias) without
// brackets parses successfully and the parser stops at the byte
// immediately after the name; trailing content stays unconsumed so
// the surrounding scanner can recover the rest of the payload.
// `NilSafe` is the test-fixture rule name; `parseRuleRefAt` does
// not consult the rule scope at this layer, so any zero-arg
// identifier would exercise the same path.
func TestParseRuleRefAt_zeroArgBare(t *testing.T) {
	ref, n, err := parseRuleRefAt("@NilSafe")
	assert.MustNoError(t, "parseRuleRefAt", err)
	assert.Equal(t, "Alias", ref.Alias, "")
	assert.Equal(t, "RuleName", ref.RuleName, "NilSafe")
	assert.Len(t, "TypeArgs", ref.TypeArgs, 0)
	assert.Equal(t, "consumed", n, len("@NilSafe"))
}

// TestParseRuleRefAt_zeroArgBareTrailing covers the boundary case
// where a bare zero-arg reference is followed by other tokens. The
// parser must stop at the trailing whitespace so the outer
// payload-level walker (the rule-list splitter, the textual
// scope.Expand) can claim the remaining content. The `NilSafe`
// rule name is a test fixture; the parser does not consult the
// rule scope at this layer.
func TestParseRuleRefAt_zeroArgBareTrailing(t *testing.T) {
	ref, n, err := parseRuleRefAt("@NilSafe ; tail")
	assert.MustNoError(t, "parseRuleRefAt", err)
	assert.Equal(t, "RuleName", ref.RuleName, "NilSafe")
	assert.Equal(t, "consumed; the parser stops before the space", n, len("@NilSafe"))
}

func TestParseRuleRefAt_nestedBrackets(t *testing.T) {
	// map[string]int contains brackets that must not throw off the
	// depth scanner.
	ref, _, err := parseRuleRefAt("@result.OkErr[map[string]int, error]")
	assert.MustNoError(t, "parseRuleRefAt", err)
	assert.DeepEqual(t, "TypeArgs", ref.TypeArgs, []string{"map[string]int", "error"})
}

func TestParseRuleRefAt_errors(t *testing.T) {
	type errorCase struct {
		in      string
		wantMsg string
	}

	tabletest.Run(t, map[string]errorCase{
		"@":               {"@", "missing rule or alias name"},
		"@result.":        {"@result.", "missing rule name"},
		"@result.OkErr":   {"@result.OkErr", "missing '[<args>]'"},
		"@result.OkErr[":  {"@result.OkErr[", "unbalanced '['"},
		"@1foo[X]":        {"@1foo[X]", "missing rule or alias name"},
		"@result.1foo[X]": {"@result.1foo[X]", "missing rule name"},
	}, func(t *testing.T, c errorCase) {
		_, _, err := parseRuleRefAt(c.in)
		assert.ErrorContains(t, fmt.Sprintf("parseRuleRefAt(%q)", c.in), err, c.wantMsg)
	})
}

func TestRuleScopeExpand(t *testing.T) {
	scope := RuleScope{
		Aliases: map[string]map[string]RuleDef{
			"result": {
				"OkErr": {
					Params: []string{"T", "E"},
					Body:   "(T!, nil) | (nil, E!)",
				},
			},
		},
	}
	got, err := scope.Expand("@result.OkErr[Result, error]")
	assert.MustNoError(t, "Expand", err)
	assert.Equal(t, "Expand", got, "(Result!, nil) | (nil, error!)")
}

func TestRuleScopeExpand_unknownAlias(t *testing.T) {
	scope := RuleScope{}
	_, err := scope.Expand("@result.OkErr[a, b]")
	assert.ErrorContains(t, "Expand with an alias that is not in scope", err, `alias "result" is not in scope`)
}
