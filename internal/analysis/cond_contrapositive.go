package analysis

import (
	"github.com/knowledge-work/go-vow/internal/dsl"
)

// logicalRuleGroup pairs an author-written rule with the optional
// derivations the analyzer produces from it: a contrapositive that
// mirrors the implication, and chain closures that combine the
// rule with the rest of the annotation's implications. The
// evaluator inspects the original first; only when the original
// could not commit does it fall back to the contrapositive, and
// only when neither commits does it consult the chain closures so
// the three never report the same offending claim twice.
//
// Forward-implication rules (`=>`) carry a contrapositive and
// chain closures; biconditional rules (`<=>`) carry neither
// because biconditional is self-dual on the contrapositive axis
// and the chain layer only connects forward implications.
type logicalRuleGroup struct {
	Original       *dsl.ArrowRule
	Contrapositive *dsl.ArrowRule
	Transitives    []transitiveEntry
}

// groupRulesWithContrapositives augments a rule list with the
// per-rule derived contrapositive and the chain closures the
// transitive expansion produces. The result preserves the input
// order so a multi-rule annotation reports diagnostics in source
// order, with each group resolving to one diagnostic at most per
// site through the original-first walk.
//
// The chain pool combines every author-written rule with the
// contrapositive of every implication so a chain that crosses a
// negated operand finds its next link. Each contrapositive is
// derived once and shared between the pool entry and the group's
// seed, so the chain walker's cycle break recognises a contrapositive
// re-entered through the same pointer rather than treating two
// derivations of the same negation as distinct rules.
func groupRulesWithContrapositives(rules []*dsl.ArrowRule) []logicalRuleGroup {
	contraByRule := map[*dsl.ArrowRule]*dsl.ArrowRule{}
	for _, r := range rules {
		if r == nil || r.Direction != dsl.DirImply {
			continue
		}
		if c, ok := contrapositiveOf(r); ok {
			contraByRule[r] = c
		}
	}
	pool := make([]chainPoolEntry, 0, len(rules)+len(contraByRule))
	for _, r := range rules {
		if r == nil {
			continue
		}
		pool = append(pool, chainPoolEntry{Rule: r})
		if c, ok := contraByRule[r]; ok {
			pool = append(pool, chainPoolEntry{Rule: c, Origin: r})
		}
	}
	out := make([]logicalRuleGroup, 0, len(rules))
	for _, r := range rules {
		if r == nil || r.Left == nil || r.Right == nil {
			continue
		}
		group := logicalRuleGroup{Original: r}
		seeds := []chainPoolEntry{{Rule: r}}
		if c, ok := contraByRule[r]; ok {
			group.Contrapositive = c
			seeds = append(seeds, chainPoolEntry{Rule: c, Origin: r})
		}
		group.Transitives = expandTransitives(seeds, pool)
		out = append(out, group)
	}
	return out
}

// contrapositiveOf builds the contrapositive of a forward
// implication: `A => B` becomes `!B => !A`. The two operands are
// negated through negateExpression, which flips the comparison
// operator while preserving the reference and literal payload, then
// swapped to put the negated former-right on the new left. When
// either operand resists negation (a bare-reference shape the
// parser does not produce, or any other unsupported operand
// kind), the helper reports ok=false so the caller drops the
// derivation rather than emitting a half-formed rule.
func contrapositiveOf(rule *dsl.ArrowRule) (*dsl.ArrowRule, bool) {
	if rule == nil || rule.Direction != dsl.DirImply || rule.Left == nil || rule.Right == nil {
		return nil, false
	}
	negLeft, ok := negateExpression(rule.Left)
	if !ok {
		return nil, false
	}
	negRight, ok := negateExpression(rule.Right)
	if !ok {
		return nil, false
	}
	return &dsl.ArrowRule{
		Direction: dsl.DirImply,
		Left:      negRight,
		Right:     negLeft,
	}, true
}

// negateExpression returns the boolean negation of expr. For a
// nil-check, the operator flips between `==` and `!=`. For a
// comparison, the operator flips through the standard pairs:
// `==`↔`!=`, `<`↔`>=`, `<=`↔`>`. The reference and literal payload
// are preserved so the negated operand binds to the same signature
// slot and reads the same right-hand side. Bare-reference shapes
// the parser reserves for future grammar growth report ok=false
// because the grammar attaches no comparison to them.
func negateExpression(expr *dsl.Expression) (*dsl.Expression, bool) {
	if expr == nil {
		return nil, false
	}
	switch expr.Kind {
	case dsl.ExprNilCheck:
		op, ok := flipNilCheckOp(expr.Op)
		if !ok {
			return nil, false
		}
		return &dsl.Expression{
			Kind: dsl.ExprNilCheck,
			Ref:  expr.Ref,
			Op:   op,
		}, true
	case dsl.ExprComparison:
		// A multi-member sum on the right-hand side does not carry a
		// contrapositive in the single-expression shape: the
		// distribution of `!=` over a sum is a conjunction, and the
		// distribution of `==` over a sum is a disjunction, neither
		// of which fits one Expression. The contrapositive layer
		// stays silent for such rules so the per-site evaluator
		// remains the single source for sum-RHS commitments.
		if len(expr.Members) != 1 {
			return nil, false
		}
		op, ok := flipComparisonOp(expr.Op)
		if !ok {
			return nil, false
		}
		return &dsl.Expression{
			Kind:    dsl.ExprComparison,
			Ref:     expr.Ref,
			Op:      op,
			Members: expr.Members,
		}, true
	}
	return nil, false
}

// flipNilCheckOp swaps `==` and `!=` for a nil-check operand. The
// grammar restricts nil-check to these two operators, so any other
// input reports ok=false to signal an unexpected parser state.
func flipNilCheckOp(op dsl.CompareOp) (dsl.CompareOp, bool) {
	switch op {
	case dsl.OpEq:
		return dsl.OpNeq, true
	case dsl.OpNeq:
		return dsl.OpEq, true
	}
	return 0, false
}

// flipComparisonOp swaps every supported comparison operator with
// its boolean complement: equality with inequality, strict-less
// with greater-or-equal, and less-or-equal with strict-greater.
// The grammar exposes exactly these six shapes, so any other input
// reports ok=false.
func flipComparisonOp(op dsl.CompareOp) (dsl.CompareOp, bool) {
	switch op {
	case dsl.OpEq:
		return dsl.OpNeq, true
	case dsl.OpNeq:
		return dsl.OpEq, true
	case dsl.OpLt:
		return dsl.OpGte, true
	case dsl.OpLte:
		return dsl.OpGt, true
	case dsl.OpGt:
		return dsl.OpLte, true
	case dsl.OpGte:
		return dsl.OpLt, true
	}
	return 0, false
}

// describeLogicalRule renders a rule for inclusion in a diagnostic
// message. A nil Origin renders the rule directly through its
// surface form; a non-nil Origin renders the original rule with a
// `contrapositive of` prefix so a reader recognises the derivation
// without parsing the negated operands.
func describeLogicalRule(rule *dsl.ArrowRule, origin *dsl.ArrowRule) string {
	if origin == nil {
		return "vow:cond " + rule.String()
	}
	return "contrapositive of vow:cond " + origin.String()
}
