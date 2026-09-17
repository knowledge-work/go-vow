package dsl

import (
	"fmt"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// TestParseConditionAnnotation_atomRule pins the atom-rule
// shape: a payload that is a lone rule-ref produces a single
// RuleAtom Rule whose Atom field carries the parsed reference. The
// Prereq / Conseq fields stay nil because no expansion has
// happened at the parser level — the dsl package now treats
// rule-refs as a first-class AST node, not as text the resolver
// must splice in.
func TestParseConditionAnnotation_atomRule(t *testing.T) {
	cond, err := ParseConditionAnnotation("@MyRule[ErrFoo]")
	assert.MustNoError(t, "ParseConditionAnnotation", err)
	assert.MustLen(t, "Rules", cond.Rules, 1)
	r := cond.Rules[0]
	assert.MustEqual(t, "Rule kind", r.Kind, RuleAtom)
	assert.MustNotNil(t, "Atom", r.Atom)
	assert.Equal(t, "Atom.Alias", r.Atom.Alias, "")
	assert.Equal(t, "Atom.RuleName", r.Atom.RuleName, "MyRule")
	assert.DeepEqual(t, "Atom.TypeArgs", r.Atom.TypeArgs, []string{"ErrFoo"})
	// The back-compat fields only describe a single value-conseq rule.
	assert.Nil(t, "Prereq for an atom-rule", cond.Prereq)
	assert.Nil(t, "Conseq for an atom-rule", cond.Conseq)
}

// TestParseConditionAnnotation_multiRule covers the `;` separator:
// every rule appears as an independent entry in Rules and the
// Prereq / Conseq fields stay nil because they can only
// describe a single-rule condition.
func TestParseConditionAnnotation_multiRule(t *testing.T) {
	cond, err := ParseConditionAnnotation("@MyRule[ErrFoo] ; @OtherRule[ErrBar]")
	assert.MustNoError(t, "ParseConditionAnnotation", err)
	assert.MustLen(t, "Rules", cond.Rules, 2)
	for i, want := range []string{"MyRule", "OtherRule"} {
		at := fmt.Sprintf("Rules[%d]", i)
		assert.MustEqual(t, at+".Kind", cond.Rules[i].Kind, RuleAtom)
		assert.MustNotNil(t, at+".Atom", cond.Rules[i].Atom)
		assert.Equal(t, at+".Atom.RuleName", cond.Rules[i].Atom.RuleName, want)
	}
	assert.Nil(t, "Prereq for a multi-rule condition", cond.Prereq)
	assert.Nil(t, "Conseq for a multi-rule condition", cond.Conseq)
}

// TestParseConditionAnnotation_arrowRuleValueConseq pins backward
// compatibility for the `prereq -> value-conseq` shape.
// The arrow-rule lands on Rules[0] AND the Prereq / Conseq
// fields are populated so back-compat consumers keep working.
func TestParseConditionAnnotation_arrowRuleValueConseq(t *testing.T) {
	cond, err := ParseConditionAnnotation("nonnil x -> result")
	assert.MustNoError(t, "ParseConditionAnnotation", err)
	assert.MustLen(t, "Rules", cond.Rules, 1)
	assert.MustEqual(t, "Rules[0].Kind", cond.Rules[0].Kind, RuleArrow)
	assert.MustNotNil(t, "legacy Prereq", cond.Prereq)
	assert.Len(t, "legacy Prereq.Positions", cond.Prereq.Positions, 1)
	assert.MustNotNil(t, "Conseq", cond.Conseq)
	assert.Len(t, "Conseq.Positions", cond.Conseq.Positions, 1)
	arrow := cond.Rules[0].Arrow
	assert.MustNotNil(t, "Arrow", arrow)
	assert.MustNotNil(t, "Arrow.Conseq", arrow.Conseq)
	assert.Equal(t, "Arrow.Conseq.Kind", arrow.Conseq.Kind, ConseqValue)
}

// TestParseConditionAnnotation_conseqRuleRef covers the higher-
// order arrow-rule conseq: `prereq -> @rule`. The conseq Kind is
// ConseqRule and Rule carries the parsed reference; Conseq
// stays nil because the legacy surface had no way to express "the
// function returns another function".
func TestParseConditionAnnotation_conseqRuleRef(t *testing.T) {
	cond, err := ParseConditionAnnotation("* -> @MyRule[ErrFoo]")
	assert.MustNoError(t, "ParseConditionAnnotation", err)
	assert.MustLen(t, "Rules", cond.Rules, 1)
	assert.MustEqual(t, "Rules[0].Kind", cond.Rules[0].Kind, RuleArrow)
	arrow := cond.Rules[0].Arrow
	assert.MustNotNil(t, "Arrow", arrow)
	assert.Nil(t, "Arrow.Prereq for a trivial wildcard", arrow.Prereq)
	assert.Equal(t, "Arrow.Conseq.Kind", arrow.Conseq.Kind, ConseqRule)
	assert.MustNotNil(t, "Arrow.Conseq.Rule", arrow.Conseq.Rule)
	assert.Equal(t, "Arrow.Conseq.Rule.RuleName", arrow.Conseq.Rule.RuleName, "MyRule")
	assert.Nil(t, "Conseq for a higher-order return", cond.Conseq)
}

// TestParseConditionAnnotation_conseqNested covers the bracketed-
// condition conseq for multi-rule higher-order returns. Detection
// hinges on the presence of a top-level `;` inside the parens —
// otherwise `(A | B)?` and tuple shapes would be misclassified.
func TestParseConditionAnnotation_conseqNested(t *testing.T) {
	cond, err := ParseConditionAnnotation("* -> (@MyRule[ErrFoo] ; @OtherRule[ErrBar])")
	assert.MustNoError(t, "ParseConditionAnnotation", err)
	arrow := cond.Rules[0].Arrow
	assert.MustEqual(t, "Arrow.Conseq.Kind", arrow.Conseq.Kind, ConseqNested)
	assert.MustNotNil(t, "Arrow.Conseq.Nested", arrow.Conseq.Nested)
	assert.Len(t, "Nested.Rules", arrow.Conseq.Nested.Rules, 2)
}

// TestTokenSpan_dropsEOF pins tokenSpan's EOF rejection, which no
// reachable input exercises: tryNestedCondition, its only caller, passes
// a strict interior of stripTrailingEOF's output. Without a direct
// fixture the rejection can be deleted with the suite still green.
func TestTokenSpan_dropsEOF(t *testing.T) {
	toks := []token{
		{Kind: tIdent, Val: "A"},
		{Kind: tEOF, Val: ""},
		{Kind: tIdent, Val: "B"},
	}
	assert.Equal(t, "tokenSpan drops the EOF token", tokenSpan(toks), "A B")
}

// TestParseConditionAnnotation_prereqRuleRefRejected pins the
// grammar invariant that rule references cannot appear in prereq
// positions. The error wraps ErrInvalidGrammar so callers can
// discriminate the failure mode programmatically.
func TestParseConditionAnnotation_prereqRuleRefRejected(t *testing.T) {
	_, err := ParseConditionAnnotation("@MyRule[ErrFoo] -> result")
	assert.MustError(t, "ParseConditionAnnotation with an @rule in the prereq", err)
	assert.ErrorIs(t, "ParseConditionAnnotation", err, ErrInvalidGrammar)
	assert.ErrorContains(t, "ParseConditionAnnotation", err, "@rule references are not allowed")
}

// TestParseConditionAnnotation_groupedSumNotMistakenAsNested
// guards against a regression where `(A | B)?` (a grouped sum
// with a qualifier) would be parsed as a nested condition. The
// detection rule requires a top-level `;` inside the parens, so
// the grouped-sum shape stays on the ConseqValue path.
func TestParseConditionAnnotation_groupedSumNotMistakenAsNested(t *testing.T) {
	cond, err := ParseConditionAnnotation("* -> (ErrFoo | nil)")
	assert.MustNoError(t, "ParseConditionAnnotation", err)
	assert.Equal(t, "Arrow.Conseq.Kind", cond.Rules[0].Arrow.Conseq.Kind, ConseqValue)
}

// TestAsSingleRuleRef covers the recognition helper. A bare
// rule-ref returns the parsed reference; mixed payloads return
// ok=false so the dispatcher falls through to the standard path.
func TestAsSingleRuleRef(t *testing.T) {
	type refCase struct {
		in        string
		wantOk    bool
		wantRule  string
		wantAlias string
	}

	tabletest.Run(t, map[string]refCase{
		"bare rule with args":        {"@MyRule[ErrFoo]", true, "MyRule", ""},
		"local rule with empty args": {"@OkErr[]", true, "OkErr", ""},
		"surrounding whitespace":     {"  @MyRule[ErrFoo]  ", true, "MyRule", ""},
		"plain value rejected":       {"error", false, "", ""},
		"arrow rejected":             {"@MyRule[ErrFoo] -> result", false, "", ""},
		"two rules joined rejected":  {"@a.B[X] @c.D[Y]", false, "", ""},
	}, func(t *testing.T, c refCase) {
		ref, ok := AsSingleRuleRef(c.in)
		assert.MustEqual(t, fmt.Sprintf("AsSingleRuleRef(%q) ok", c.in), ok, c.wantOk)
		if !c.wantOk {
			return
		}
		assert.Equal(t, "RuleName", ref.RuleName, c.wantRule)
		assert.Equal(t, "Alias", ref.Alias, c.wantAlias)
	})
}

// TestWrapAtomRuleAsCondition pins the atom-rule lift. The
// expanded body is stored on the Conseq so back-compat
// consumers keep applying the function-level shape check, and the
// atom-rule is recorded on Rules so the downstream discharge,
// transducer, and emit passes can dispatch on Effect kind.
func TestWrapAtomRuleAsCondition(t *testing.T) {
	ref := &RuleRef{Alias: "result", RuleName: "OkErr", TypeArgs: []string{"T", "E"}}
	body, err := ParsePresetBody("(nonzero T, nil) | (nil, nonzero E)")
	assert.MustNoError(t, "ParsePresetBody", err)
	cond := WrapAtomRuleAsCondition(ref, body)
	assert.MustNotNil(t, "WrapAtomRuleAsCondition", cond)
	assert.MustLen(t, "Rules", cond.Rules, 1)
	assert.Equal(t, "Rules[0].Kind", cond.Rules[0].Kind, RuleAtom)
	// Pointer identity: the lift must record the supplied ref and
	// body, not copies of them.
	assert.Equal(t, "Rules[0].Atom is the supplied ref", cond.Rules[0].Atom, ref)
	assert.Equal(t, "Conseq is the expanded body", cond.Conseq, body)
}
