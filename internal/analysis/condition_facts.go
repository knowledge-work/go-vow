package analysis

import (
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
	"github.com/knowledge-work/go-vow/internal/seq"
)

// conditionLogicalFact is the analysis.Fact exported for every
// function whose vow:cond payload carries a logical-arrow rule
// (`=>` or `<=>`). The fact carries the parsed operand pair so
// importing packages drive the caller-side literal evaluator
// without the callee's AST in scope. Same-package callers
// consult the in-memory rule cache instead; the two paths
// converge on the same dsl.ArrowRule shape after materialisation.
//
// Only logical-arrow rules go through the fact mechanism.
// Structural arrow-rules (`->`) drive same-package return
// checking through a dedicated path that the analyzer applies
// without needing a fact, and atom-rules are resolved through
// the preset registry rather than per-function facts.
type conditionLogicalFact struct {
	Rules []conditionLogicalFactRule
}

// conditionLogicalFactRule is the serialisable payload of one
// logical-arrow rule. The struct copies the Direction
// discriminator and the two operand pointers so a gob round-trip
// reconstructs the same logical content the in-memory rule
// carries; the parser-owned bookkeeping
// (SugarCarriedFromPrevious) is dropped because it has no effect
// on the rule's semantics, only on how the source rendered.
type conditionLogicalFactRule struct {
	Direction dsl.Direction
	Left      *dsl.Expression
	Right     *dsl.Expression
}

// AFact admits conditionLogicalFact to the Go analysis Facts
// mechanism. The empty body matches the convention every
// analysis.Fact implementation uses.
func (*conditionLogicalFact) AFact() {}

// String renders the fact in cache dumps and diagnostic reports.
// Each rule renders through the same `lhs <arrow> rhs` shape the
// source surface uses, joined by `;` so a multi-rule fact still
// fits on one line.
func (f *conditionLogicalFact) String() string {
	if f == nil || len(f.Rules) == 0 {
		return "vow:cond()"
	}
	parts := seq.ChainOf(f.Rules...).Map(renderFactRule).ToSlice()
	return "vow:cond(" + strings.Join(parts, "; ") + ")"
}

// renderFactRule prints one fact entry in `lhs <arrow> rhs`
// shape. The rendering mirrors ArrowRule.String for the logical
// arrow direction so a reader who recognises the source surface
// recognises the fact dump.
func renderFactRule(r conditionLogicalFactRule) string {
	left := "<nil>"
	if r.Left != nil {
		left = r.Left.String()
	}
	right := "<nil>"
	if r.Right != nil {
		right = r.Right.String()
	}
	return left + " " + r.Direction.String() + " " + right
}

// exportConditionLogicalFacts publishes a conditionLogicalFact
// for every exported function whose vow:cond payload carries at
// least one logical-arrow rule. The export goes through the
// cached rules-by-object set so a multi-line annotation lands as
// a single fact carrying every rule in source order. Functions
// whose annotations exclusively carry structural arrow-rules or
// atom-rules contribute no fact because the importing analyzer
// has no caller-side use for those shapes.
//
// Unexported declarations are skipped because they are not
// reachable across the package boundary, so a fact on them would
// never be imported. The same-package caller-side evaluator
// consults the in-memory rules cache instead, which covers every
// declaration regardless of its export status.
func exportConditionLogicalFacts(pass *analysis.Pass, state *passState) {
	for obj, rules := range state.logicalRulesSet(pass) {
		if obj == nil || len(rules) == 0 {
			continue
		}
		if !obj.Exported() {
			continue
		}
		entries := make([]conditionLogicalFactRule, len(rules))
		for i, r := range rules {
			entries[i] = conditionLogicalFactRule{
				Direction: r.Direction,
				Left:      r.Left,
				Right:     r.Right,
			}
		}
		pass.ExportObjectFact(obj, &conditionLogicalFact{Rules: entries})
	}
}

// materialiseFactRules converts the serialised fact entries back
// into dsl.ArrowRule pointers the caller-side evaluator already
// understands. The reconstruction restores the operand pointers
// directly because gob already deep-copies them during decode,
// so the result is safe to retain past the fact's lifetime.
func materialiseFactRules(entries []conditionLogicalFactRule) []*dsl.ArrowRule {
	if len(entries) == 0 {
		return nil
	}
	rules := make([]*dsl.ArrowRule, len(entries))
	for i, e := range entries {
		rules[i] = &dsl.ArrowRule{
			Direction: e.Direction,
			Left:      e.Left,
			Right:     e.Right,
		}
	}
	return rules
}
