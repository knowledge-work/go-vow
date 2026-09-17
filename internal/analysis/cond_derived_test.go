package analysis

import (
	"fmt"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/dsl"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// TestDescribeLogicalRuleContrapositive pins the surface form a
// contrapositive-driven diagnostic prepends to a rule. The
// `contrapositive of vow:cond ...` prefix is the user-visible
// signal that the analyzer derived the rule from a forward
// implication, so a regression that altered the prefix would
// surface here even when no testdata fixture exercises the path
// end-to-end.
func TestDescribeLogicalRuleContrapositive(t *testing.T) {
	x := &dsl.Expression{
		Kind: dsl.ExprNilCheck,
		Ref:  dsl.Reference{Kind: dsl.RefIdent, Name: "x"},
		Op:   dsl.OpNeq,
	}
	y := &dsl.Expression{
		Kind: dsl.ExprNilCheck,
		Ref:  dsl.Reference{Kind: dsl.RefIdent, Name: "y"},
		Op:   dsl.OpNeq,
	}
	original := &dsl.ArrowRule{Direction: dsl.DirImply, Left: x, Right: y}
	contra, ok := contrapositiveOf(original)
	assert.MustEqual(t, "contrapositiveOf ok for a simple nil-check rule", ok, true)
	assert.Equal(t, "describeLogicalRule(contra, original)", describeLogicalRule(contra, original),
		"contrapositive of vow:cond x != nil => y != nil")
	assert.Equal(t, "describeLogicalRule(original, nil)", describeLogicalRule(original, nil),
		"vow:cond x != nil => y != nil")
}

// TestRenderChainTransitiveClosure pins the surface form a
// chain-closure diagnostic uses. The chain renders source rules
// joined by ` and `; a derived contrapositive in the chain renders
// through its origin with the `contrapositive of` prefix rather
// than printing the negated synthetic rule the chain walked
// through.
func TestRenderChainTransitiveClosure(t *testing.T) {
	a := &dsl.Expression{
		Kind: dsl.ExprNilCheck,
		Ref:  dsl.Reference{Kind: dsl.RefIdent, Name: "a"},
		Op:   dsl.OpNeq,
	}
	b := &dsl.Expression{
		Kind: dsl.ExprNilCheck,
		Ref:  dsl.Reference{Kind: dsl.RefIdent, Name: "b"},
		Op:   dsl.OpNeq,
	}
	c := &dsl.Expression{
		Kind: dsl.ExprNilCheck,
		Ref:  dsl.Reference{Kind: dsl.RefIdent, Name: "c"},
		Op:   dsl.OpNeq,
	}
	r1 := &dsl.ArrowRule{Direction: dsl.DirImply, Left: a, Right: b}
	r2 := &dsl.ArrowRule{Direction: dsl.DirImply, Left: b, Right: c}
	assert.Equal(t, "renderChain(author chain)", renderChain([]chainPoolEntry{{Rule: r1}, {Rule: r2}}),
		"vow:cond a != nil => b != nil and vow:cond b != nil => c != nil")
	contra, ok := contrapositiveOf(r2)
	assert.MustEqual(t, "contrapositiveOf ok for a simple nil-check rule", ok, true)
	assert.Equal(t, "renderChain(mixed chain)",
		renderChain([]chainPoolEntry{{Rule: r1}, {Rule: contra, Origin: r2}}),
		"vow:cond a != nil => b != nil and contrapositive of vow:cond b != nil => c != nil")
}

// TestExpandTransitivesKindPropagation pins the seed-origin
// derivation marker `expandTransitives` attaches to every
// materialised entry. A chain whose seed carries a nil Origin
// (author-written rule) emits forward-direction entries; a chain
// whose seed names a non-nil Origin (derived contrapositive)
// emits contrapositive-rotated entries. The Kind field propagates
// through every walk step so a consumer reads the derivation
// without re-walking the chain.
func TestExpandTransitivesKindPropagation(t *testing.T) {
	aNonNil := &dsl.Expression{
		Kind: dsl.ExprNilCheck,
		Ref:  dsl.Reference{Kind: dsl.RefIdent, Name: "a"},
		Op:   dsl.OpNeq,
	}
	bNonNil := &dsl.Expression{
		Kind: dsl.ExprNilCheck,
		Ref:  dsl.Reference{Kind: dsl.RefIdent, Name: "b"},
		Op:   dsl.OpNeq,
	}
	cNonNil := &dsl.Expression{
		Kind: dsl.ExprNilCheck,
		Ref:  dsl.Reference{Kind: dsl.RefIdent, Name: "c"},
		Op:   dsl.OpNeq,
	}
	aNil := &dsl.Expression{
		Kind: dsl.ExprNilCheck,
		Ref:  dsl.Reference{Kind: dsl.RefIdent, Name: "a"},
		Op:   dsl.OpEq,
	}
	zNil := &dsl.Expression{
		Kind: dsl.ExprNilCheck,
		Ref:  dsl.Reference{Kind: dsl.RefIdent, Name: "z"},
		Op:   dsl.OpEq,
	}
	r1 := &dsl.ArrowRule{Direction: dsl.DirImply, Left: aNonNil, Right: bNonNil}
	r2 := &dsl.ArrowRule{Direction: dsl.DirImply, Left: bNonNil, Right: cNonNil}
	pool := []chainPoolEntry{{Rule: r1}, {Rule: r2}}
	forward := expandTransitives([]chainPoolEntry{{Rule: r1}}, pool)
	assert.MustEqual(t, "expandTransitives yields entries for the forward chain", len(forward) > 0, true)
	for i, entry := range forward {
		assert.Equal(t, fmt.Sprintf("forward[%d].Kind", i), entry.Kind, transitiveEntryForward)
	}
	contra, ok := contrapositiveOf(r1)
	assert.MustEqual(t, "contrapositiveOf ok for the seed rule", ok, true)
	// `r3` (`a == nil => z == nil`) extends the pool so the contrapositive
	// seed (`b == nil => a == nil`) can chain forward through one step and
	// emit a derived entry. Without this rule the contrapositive seed
	// produces zero matches and the loop below would never run.
	r3 := &dsl.ArrowRule{Direction: dsl.DirImply, Left: aNil, Right: zNil}
	contraPool := []chainPoolEntry{{Rule: r1}, {Rule: contra, Origin: r1}, {Rule: r2}, {Rule: r3}}
	mixed := expandTransitives([]chainPoolEntry{{Rule: contra, Origin: r1}}, contraPool)
	assert.MustEqual(t, "expandTransitives yields entries for the contrapositive chain", len(mixed) > 0, true)
	for i, entry := range mixed {
		assert.Equal(t, fmt.Sprintf("mixed[%d].Kind", i), entry.Kind, transitiveEntryContrapositive)
	}
}

// TestFlipComparisonOpPairs pins the boolean-negation table the
// contrapositive derivation relies on. Each pair must flip to its
// complement so a negated comparison preserves the rule's
// semantic shape.
func TestFlipComparisonOpPairs(t *testing.T) {
	type flipCase struct {
		in, want dsl.CompareOp
	}

	tabletest.Run(t, map[string]flipCase{
		"eq to neq": {dsl.OpEq, dsl.OpNeq},
		"neq to eq": {dsl.OpNeq, dsl.OpEq},
		"lt to gte": {dsl.OpLt, dsl.OpGte},
		"lte to gt": {dsl.OpLte, dsl.OpGt},
		"gt to lte": {dsl.OpGt, dsl.OpLte},
		"gte to lt": {dsl.OpGte, dsl.OpLt},
	}, func(t *testing.T, c flipCase) {
		got, ok := flipComparisonOp(c.in)
		assert.MustEqual(t, "flipComparisonOp ok", ok, true)
		assert.Equal(t, "flipComparisonOp", got, c.want)
	})
}

// TestTransitivesByForwardFirst pins the stable kind-aware
// ordering `transitivesByForwardFirst` produces: every entry
// whose Kind is `transitiveEntryForward` lands ahead of every
// entry whose Kind is `transitiveEntryContrapositive`, and
// within each partition the relative order of the input survives
// so a caller that already arranged entries by construction
// order keeps that arrangement underneath the partition.
func TestTransitivesByForwardFirst(t *testing.T) {
	rule := &dsl.ArrowRule{}
	entries := []transitiveEntry{
		{Rule: rule, ChainDesc: "chainA", Kind: transitiveEntryContrapositive},
		{Rule: rule, ChainDesc: "chainB", Kind: transitiveEntryForward},
		{Rule: rule, ChainDesc: "chainC", Kind: transitiveEntryContrapositive},
		{Rule: rule, ChainDesc: "chainD", Kind: transitiveEntryForward},
	}
	assert.DeepEqual(t, "the ChainDesc order", chainDescs(transitivesByForwardFirst(entries)),
		[]string{"chainB", "chainD", "chainA", "chainC"})
}

// TestTransitivesByForwardFirstAllForward pins the all-forward
// short-circuit: when every entry is a forward derivation, the
// helper returns the forward partition directly without appending
// a contrapositive tail, preserving the input order.
func TestTransitivesByForwardFirstAllForward(t *testing.T) {
	rule := &dsl.ArrowRule{}
	entries := []transitiveEntry{
		{Rule: rule, ChainDesc: "first", Kind: transitiveEntryForward},
		{Rule: rule, ChainDesc: "second", Kind: transitiveEntryForward},
	}
	assert.DeepEqual(t, "the ChainDesc order", chainDescs(transitivesByForwardFirst(entries)),
		[]string{"first", "second"})
}

// chainDescs projects the ordering the transitive helpers produce onto
// the axis the tests pin, so a failure names the order rather than
// dumping every entry.
func chainDescs(entries []transitiveEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.ChainDesc
	}
	return out
}
