package analysis

import (
	"go/ast"
	"go/constant"
	"go/token"
	"strconv"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// intRangeBound carries the inclusive integer interval the
// dominator walk accumulated for an SSA value. An unset bound
// stands for an open end (unbounded below or unbounded above)
// so the helper distinguishes a bound the walker reached from
// one it never saw. Intersection keeps the larger of the two
// lowers and the smaller of the two uppers so the chain of
// dominating guards refines monotonically as the walker
// ascends.
type intRangeBound struct {
	lower    int64
	upper    int64
	hasLower bool
	hasUpper bool
}

// intersect returns the intersection of r with other. The
// merged bound keeps the tighter side on each axis. Inconsistent
// bounds (lower > upper after the merge) are not normalised
// because the dominator walk only reaches consistent bounds
// along any single control path.
func (r intRangeBound) intersect(other intRangeBound) intRangeBound {
	merged := r
	if other.hasLower && (!merged.hasLower || other.lower > merged.lower) {
		merged.lower = other.lower
		merged.hasLower = true
	}
	if other.hasUpper && (!merged.hasUpper || other.upper < merged.upper) {
		merged.upper = other.upper
		merged.hasUpper = true
	}
	return merged
}

// implies decides the comparison `value OP literal` from the
// interval r the walker accumulated. The helper commits when r
// bounds value strictly on the relevant side; equality reasoning
// commits when both ends pin the same constant. Operands the
// interval does not bound on the relevant side report
// decided=false so the caller keeps the rule unevaluated and
// the under-approximation keeps false positives out of the
// diagnostic.
func (r intRangeBound) implies(op dsl.CompareOp, literal int64) (bool, bool) {
	switch op {
	case dsl.OpLt:
		if r.hasUpper && r.upper < literal {
			return true, true
		}
		if r.hasLower && r.lower >= literal {
			return false, true
		}
	case dsl.OpLte:
		if r.hasUpper && r.upper <= literal {
			return true, true
		}
		if r.hasLower && r.lower > literal {
			return false, true
		}
	case dsl.OpGt:
		if r.hasLower && r.lower > literal {
			return true, true
		}
		if r.hasUpper && r.upper <= literal {
			return false, true
		}
	case dsl.OpGte:
		if r.hasLower && r.lower >= literal {
			return true, true
		}
		if r.hasUpper && r.upper < literal {
			return false, true
		}
	case dsl.OpEq:
		if r.hasLower && r.hasUpper && r.lower == r.upper && r.lower == literal {
			return true, true
		}
		if (r.hasLower && r.lower > literal) || (r.hasUpper && r.upper < literal) {
			return false, true
		}
	case dsl.OpNeq:
		if (r.hasLower && r.lower > literal) || (r.hasUpper && r.upper < literal) {
			return true, true
		}
		if r.hasLower && r.hasUpper && r.lower == r.upper && r.lower == literal {
			return false, true
		}
	}
	return false, false
}

// ssaNarrowedComparison refines an ordered or equality
// comparison against an integer literal with the SSA block-
// context dominator walk. The walker resolves the operand
// expression to its SSA parameter, reads the chain of
// dominating *ssa.If conditions whose comparison binds that
// parameter against an integer constant, accumulates the
// interval those guards prove on the dominating side, and asks
// the resulting bound whether the rule operand commits.
//
// The forward-walk path the nil-check refinement uses carries
// no useful counterpart for integer values: an SSA-versioned
// register flow analysis sits outside this helper's scope.
// The block-guard path covers the common authoring pattern (a
// guard guarding the rest of the function body) and leaves
// the rest undecided so the under-approximation keeps false
// positives out of the diagnostic.
func ssaNarrowedComparison(pass *analysis.Pass, ssaFn *ssa.Function, expr ast.Expr, op dsl.CompareOp, spelling string, block *ssa.BasicBlock) (bool, bool) {
	if ssaFn == nil || block == nil {
		return false, false
	}
	if !isOrderedOrEqualityOp(op) {
		return false, false
	}
	literal, ok := parseIntegerLiteral(spelling)
	if !ok {
		return false, false
	}
	ident, ok := ast.Unparen(expr).(*ast.Ident)
	if !ok {
		return false, false
	}
	value := ssaParameterForIdent(pass, ssaFn, ident)
	if value == nil {
		return false, false
	}
	bound := accumulateRangeFromGuards(value, block)
	return bound.implies(op, literal)
}

// isOrderedOrEqualityOp reports whether op participates in
// integer range reasoning. The nil-side operators target
// reference values rather than ordered integers; the
// ssaNarrowedNilCheck helper covers those, so this helper
// leaves them out so the two refinement paths stay disjoint.
func isOrderedOrEqualityOp(op dsl.CompareOp) bool {
	switch op {
	case dsl.OpEq, dsl.OpNeq, dsl.OpLt, dsl.OpLte, dsl.OpGt, dsl.OpGte:
		return true
	}
	return false
}

// parseIntegerLiteral decodes spelling as a signed integer
// literal. The helper mirrors the integer branch of
// classifyComparisonLiteral so the surface a rule's right-hand
// side carries reads the same here as at the AST-literal
// evaluator.
func parseIntegerLiteral(spelling string) (int64, bool) {
	n, err := strconv.ParseInt(spelling, 0, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// accumulateRangeFromGuards walks the dominator chain of block
// and intersects each dominating *ssa.If condition that
// compares value against an integer constant into the
// accumulating interval. The walk stops at the function
// entry; cycles are impossible because the dominator relation
// is a tree.
func accumulateRangeFromGuards(value ssa.Value, block *ssa.BasicBlock) intRangeBound {
	bound := intRangeBound{}
	for cur := block; cur != nil; cur = cur.Idom() {
		idom := cur.Idom()
		if idom == nil {
			return bound
		}
		ifInstr, ok := terminatingIf(idom)
		if !ok {
			continue
		}
		dominantTrue, ok := dominantSide(ifInstr, cur)
		if !ok {
			continue
		}
		guardBound, ok := guardRangeBound(ifInstr.Cond, value, dominantTrue)
		if !ok {
			continue
		}
		bound = bound.intersect(guardBound)
	}
	return bound
}

// guardRangeBound translates a terminating-If condition into
// the inclusive interval it proves for value on the
// dominating side. The condition must compare value against
// an integer constant; the comparison operator together with
// the side selects which inclusive bound the guard
// establishes. Equality on the dominating side pins both ends
// to the constant; inequality on the dominating side carves
// out a single point that no interval bound can represent, so
// the helper reports ok=false for that shape. Other shapes
// the comparison does not match (a non-constant operand, a
// comparison against a non-integer constant) also report
// ok=false so the walker drops the guard.
func guardRangeBound(cond ssa.Value, value ssa.Value, dominantTrue bool) (intRangeBound, bool) {
	bop, ok := cond.(*ssa.BinOp)
	if !ok {
		return intRangeBound{}, false
	}
	literal, leftIsValue, ok := compareValueAgainstIntConst(bop, value)
	if !ok {
		return intRangeBound{}, false
	}
	op := bop.Op
	if !leftIsValue {
		op = swapOrderedOp(op)
	}
	if !dominantTrue {
		op = negateOrderedOp(op)
	}
	switch op {
	case token.LSS:
		return intRangeBound{upper: literal - 1, hasUpper: true}, true
	case token.LEQ:
		return intRangeBound{upper: literal, hasUpper: true}, true
	case token.GTR:
		return intRangeBound{lower: literal + 1, hasLower: true}, true
	case token.GEQ:
		return intRangeBound{lower: literal, hasLower: true}, true
	case token.EQL:
		return intRangeBound{lower: literal, upper: literal, hasLower: true, hasUpper: true}, true
	}
	return intRangeBound{}, false
}

// swapOrderedOp returns the operator that yields the same
// truth when the two operands swap places. Equality and
// inequality stay put; ordered operators reverse direction
// (`<` swaps with `>`, `<=` swaps with `>=`).
func swapOrderedOp(op token.Token) token.Token {
	switch op {
	case token.LSS:
		return token.GTR
	case token.LEQ:
		return token.GEQ
	case token.GTR:
		return token.LSS
	case token.GEQ:
		return token.LEQ
	}
	return op
}

// negateOrderedOp returns the operator that yields the
// opposite truth for the same pair of operands. The helper
// flips each ordered operator with its complement so the
// else-side translation of an If guard reaches the correct
// inclusive bound.
func negateOrderedOp(op token.Token) token.Token {
	switch op {
	case token.EQL:
		return token.NEQ
	case token.NEQ:
		return token.EQL
	case token.LSS:
		return token.GEQ
	case token.LEQ:
		return token.GTR
	case token.GTR:
		return token.LEQ
	case token.GEQ:
		return token.LSS
	}
	return op
}

// compareValueAgainstIntConst reports whether bop compares
// value against an integer constant on either operand
// position. The returned literal is the int64 value of the
// constant, and leftIsValue reports whether value sat on the
// left of the comparison so the caller can normalise the
// operator orientation before applying it.
func compareValueAgainstIntConst(bop *ssa.BinOp, value ssa.Value) (int64, bool, bool) {
	if bop.X == value {
		if n, ok := intSSAConst(bop.Y); ok {
			return n, true, true
		}
	}
	if bop.Y == value {
		if n, ok := intSSAConst(bop.X); ok {
			return n, false, true
		}
	}
	return 0, false, false
}

// intSSAConst returns the int64 value of the SSA integer
// constant v. Non-constant or non-integer SSA values, and
// constants whose payload does not fit in an int64, report
// ok=false so the caller drops the guard.
func intSSAConst(v ssa.Value) (int64, bool) {
	c, ok := v.(*ssa.Const)
	if !ok {
		return 0, false
	}
	val := c.Value
	if val == nil || val.Kind() != constant.Int {
		return 0, false
	}
	n, exact := constant.Int64Val(val)
	if !exact {
		return 0, false
	}
	return n, true
}
