package dsl

import (
	"fmt"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/seq"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// samePredicate asserts predicate identity under PredicateEquals,
// naming both values in the label so a failure reads like the
// got/want an equality assertion would print.
func samePredicate(t *testing.T, label string, got, want Predicate) {
	t.Helper()
	assert.Equal(t, fmt.Sprintf("%s = %v, want %v", label, got, want), PredicateEquals(got, want), true)
}

// TestParseConditionAnnotation_basic covers the primary
// parameter-requirement → return-requirement shape. Both halves
// use the same precedence-based grammar that ParsePresetBody
// drives, so this gate also confirms the shared parser entry
// survives the token-stream split on `->`.
func TestParseConditionAnnotation_basic(t *testing.T) {
	cond, err := ParseConditionAnnotation("nonnil x -> nonnil result, nonnil err")
	assert.MustNoError(t, "ParseConditionAnnotation", err)
	assert.MustNotNil(t, "Prereq", cond.Prereq)
	assert.MustLen(t, "Prereq.Positions", cond.Prereq.Positions, 1)
	prereqTerm := cond.Prereq.Positions[0].Single
	assert.MustNotNil(t, "Prereq.Positions[0].Single", prereqTerm)
	assert.Equal(t, "prereq term Type", prereqTerm.Type, "x")
	samePredicate(t, "prereq term Pred", prereqTerm.Pred, Not{Inner: Nil{}})

	assert.MustNotNil(t, "Conseq", cond.Conseq)
	assert.MustLen(t, "Conseq.Positions", cond.Conseq.Positions, 2)
	consTerm0 := cond.Conseq.Positions[0].Single
	assert.MustNotNil(t, "Conseq.Positions[0].Single", consTerm0)
	assert.Equal(t, "conseq[0] Type", consTerm0.Type, "result")
	samePredicate(t, "conseq[0] Pred", consTerm0.Pred, Not{Inner: Nil{}})
}

// TestParseConditionAnnotation_multiArgPrereq covers a comma-
// separated parameter requirement spanning several arguments. The
// order of positions in the AST must match the source order so
// the later analyzer can bind each position to its argument by
// signature index.
func TestParseConditionAnnotation_multiArgPrereq(t *testing.T) {
	cond, err := ParseConditionAnnotation("nonnil x, nonzero y -> result")
	assert.MustNoError(t, "ParseConditionAnnotation", err)
	assert.MustNotNil(t, "Prereq", cond.Prereq)
	assert.MustLen(t, "Prereq.Positions", cond.Prereq.Positions, 2)
	for i, want := range []string{"x", "y"} {
		single := cond.Prereq.Positions[i].Single
		assert.MustNotNil(t, fmt.Sprintf("Prereq[%d].Single", i), single)
		assert.Equal(t, fmt.Sprintf("Prereq[%d].Single.Type", i), single.Type, want)
	}
}

// TestParseConditionAnnotation_bareNilPrereq pins the surface
// promotion of a bare `nil` token in a parameter-requirement
// position. The shared expression parser treats `nil` without a
// trailing type as the nil literal because the return-requirement
// half admits `nil` as an authorised return value; a
// parameter-requirement slot has no use for the literal
// interpretation, so the post-parse step rewrites it into the Nil
// predicate. This keeps the surface uniform with `nonnil`, `zero`,
// and `nonzero`, which already parse as predicates without a
// trailing type.
func TestParseConditionAnnotation_bareNilPrereq(t *testing.T) {
	cond, err := ParseConditionAnnotation("nil -> ErrFoo | nil")
	assert.MustNoError(t, "ParseConditionAnnotation", err)
	assert.MustNotNil(t, "Prereq", cond.Prereq)
	assert.MustLen(t, "Prereq.Positions", cond.Prereq.Positions, 1)
	got := cond.Prereq.Positions[0].Single
	assert.MustNotNil(t, "Prereq.Positions[0].Single", got)
	samePredicate(t, "prereq[0] Pred", got.Pred, Nil{})
	assert.Equal(t, "prereq[0].Type; no documentation hint was authored", got.Type, "")
}

// TestParseConditionAnnotation_bareNilPrereqMultiPosition gates
// the promotion against a multi-position parameter requirement.
// Only the `nil` token rewrites; the `nonnil` and `zero` positions
// already land on the predicate surface through the shared
// parser and must pass through the promotion step unchanged.
func TestParseConditionAnnotation_bareNilPrereqMultiPosition(t *testing.T) {
	cond, err := ParseConditionAnnotation("nil, nonnil, zero -> result")
	assert.MustNoError(t, "ParseConditionAnnotation", err)
	assert.MustNotNil(t, "Prereq", cond.Prereq)
	assert.MustLen(t, "Prereq.Positions", cond.Prereq.Positions, 3)
	for i, want := range []Predicate{Nil{}, Not{Inner: Nil{}}, Zero{}} {
		got := cond.Prereq.Positions[i].Single
		assert.MustNotNil(t, fmt.Sprintf("prereq[%d].Single", i), got)
		samePredicate(t, fmt.Sprintf("prereq[%d] Pred", i), got.Pred, want)
	}
}

// TestParsePresetBody_bareNilStillLiteral guards the
// scoping of the promotion: the return-requirement half of a
// condition (and every standalone `vow:cond` payload) must
// keep its `nil` literal semantics. The fixture pins the legacy
// interpretation so a tightening that accidentally extended the
// rewrite to the return-requirement half would surface as a test
// failure.
func TestParsePresetBody_bareNilStillLiteral(t *testing.T) {
	anno, err := ParsePresetBody("ErrFoo | nil")
	assert.MustNoError(t, "ParsePresetBody", err)
	assert.MustLen(t, "Positions", anno.Positions, 1)
	assert.MustNotNil(t, "Positions[0].Sum", anno.Positions[0].Sum)
	members := anno.Positions[0].Sum.Members
	assert.MustLen(t, "Sum.Members", members, 2)
	assert.MustNotNil(t, "members[1].Literal", members[1].Literal)
	assert.Equal(t, "members[1].Literal.Kind", members[1].Literal.Kind, LitNil)
}

// TestParseConditionAnnotation_returnRequirementTupleSum
// confirms that the return-requirement half retains every shape
// ParsePresetBody already accepts — including the
// pipe-of-tuples form that lifts to the TupleSum AST. Authors
// should be able to write any existing `vow:cond` payload as
// the right side of a condition without reshaping it.
func TestParseConditionAnnotation_returnRequirementTupleSum(t *testing.T) {
	cond, err := ParseConditionAnnotation("nonnil x -> (nonzero Result, nil) | (nil, nonzero error)")
	assert.MustNoError(t, "ParseConditionAnnotation", err)
	assert.MustNotNil(t, "Conseq", cond.Conseq)
	assert.MustNotNil(t, "Conseq.TupleSum", cond.Conseq.TupleSum)
	assert.Len(t, "TupleSum.Members", cond.Conseq.TupleSum.Members, 2)
}

// TestParseConditionAnnotation_errors pins the structured
// diagnostics the condition surface owns. Each case names the
// invariant the error signals so a future change cannot silently
// loosen the rule.
func TestParseConditionAnnotation_errors(t *testing.T) {
	type errorCase struct {
		in      string
		wantMsg string
	}

	tabletest.Run(t, map[string]errorCase{
		"missing arrow":               {"x, y", "missing arrow separator"},
		"empty parameter requirement": {"-> result", "parameter requirement is empty"},
		"tuple-sum prereq rejected":   {"(a, b) | (c, d) -> result", "tuple-sum form is not supported in a parameter requirement"},
		"multiple arrows":             {"a -> b -> c", "multiple top-level `->`"},
		"empty return requirement":    {"x ->", "return requirement"},
	}, func(t *testing.T, c errorCase) {
		_, err := ParseConditionAnnotation(c.in)
		assert.ErrorContains(t, fmt.Sprintf("ParseConditionAnnotation(%q)", c.in), err, c.wantMsg)
	})
}

// TestConditionString covers the round-trip rendering rules: a
// trivial-Prereq Condition prints as just its return requirement
// (so a `vow:cond * -> X` payload renders as `X` after the
// wildcard-prereq normalisation), and a fully-populated Condition
// prints `<prereq> -> <conseq>` with each half using its own
// canonical surface form.
func TestConditionString(t *testing.T) {
	type renderCase struct {
		in   string
		want string
	}

	tabletest.Run(t, map[string]renderCase{
		"trivial prereq via wildcard": {"* -> error", "error"},
		"non-nil x to single result":  {"nonnil x -> nonnil result", "nonnil x -> nonnil result"},
		"sum return requirement":      {"nonnil x -> (ErrFoo | nil)", "nonnil x -> (ErrFoo | nil)"},
	}, func(t *testing.T, c renderCase) {
		cond, err := ParseConditionAnnotation(c.in)
		assert.MustNoError(t, fmt.Sprintf("ParseConditionAnnotation(%q)", c.in), err)
		assert.Equal(t, "Condition.String()", cond.String(), c.want)
	})
}

// TestTokenize_arrow confirms the lexer emits a dedicated tArrow
// token for `->` while leaving `-1`-style negative integer
// literals as a single tIdent run (the existing precedence-parser
// expects the `-1` shape to reach parseLiteral atomically).
func TestTokenize_arrow(t *testing.T) {
	type tokenCase struct {
		in   string
		want []tokKind
	}

	tabletest.Run(t, map[string]tokenCase{
		"->":     {"->", []tokKind{tArrow, tEOF}},
		"x -> y": {"x -> y", []tokKind{tIdent, tArrow, tIdent, tEOF}},
		"-1":     {"-1", []tokKind{tIdent, tEOF}},
		"x, -1":  {"x, -1", []tokKind{tIdent, tComma, tIdent, tEOF}},
	}, func(t *testing.T, c tokenCase) {
		toks, err := tokenize(c.in, lexReturn)
		assert.MustNoError(t, fmt.Sprintf("tokenize(%q)", c.in), err)
		got := seq.ChainOf(toks...).Map(func(tok token) tokKind { return tok.Kind }).ToSlice()
		assert.DeepEqual(t, fmt.Sprintf("tokenize(%q) kinds", c.in), got, c.want)
	})
}
