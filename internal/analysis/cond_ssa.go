package analysis

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// ssaNarrowedNilCheck refines the literal-only nil check with the
// SSA forward walk and a block-context dominator walk. The two
// paths cover the shapes the literal-only evaluator cannot reach:
//
//   - The forward walk consults resolveArgNilness, which chains
//     parameter nilness from the enclosing signature decl, one-hop
//     local alias resolution, the field-nil registry, and the
//     slice/map element decls in a single helper. An operand bound
//     to one of those shapes commits on the same nilness lattice the
//     nil-safety pass establishes for `vow:nil`.
//   - The block walk dominator-narrows an identifier operand against
//     an enclosing `if ident != nil` / `if ident == nil` guard. The
//     parameter requirement narrowing already drives the same idom
//     pattern; the logical-arrow refinement extends the recognition
//     to the identifier shapes a `vow:cond` operand names.
//
// The decision returns (held, decided): decided=false leaves the
// rule unevaluated at this site so the caller drops it silently.
// Nillable and Unknown both report undecided because the operand
// could still go either way — the under-approximation keeps false
// positives out of the diagnostic.
func ssaNarrowedNilCheck(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function, expr ast.Expr, op dsl.CompareOp, block *ssa.BasicBlock) (bool, bool) {
	if ssaFn == nil {
		return false, false
	}
	if op != dsl.OpEq && op != dsl.OpNeq {
		return false, false
	}
	if held, decided := forwardWalkNilness(pass, state, fn, ssaFn, expr, op); decided {
		return held, true
	}
	return blockGuardNilness(pass, ssaFn, expr, op, block)
}

// forwardWalkNilness consults resolveArgNilness for expr and maps
// the NonNil / KnownNil leaves to the operator-specific boolean
// outcome. Nillable and Unknown report undecided so the caller can
// fall through to the block-guard walk before giving up.
func forwardWalkNilness(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function, expr ast.Expr, op dsl.CompareOp) (bool, bool) {
	flow := resolveArgNilness(pass, state, fn, ssaFn, expr)
	switch flow {
	case nilnessNonNil:
		return op == dsl.OpNeq, true
	case nilnessKnownNil:
		return op == dsl.OpEq, true
	}
	return false, false
}

// blockGuardNilness narrows a parameter identifier operand through
// an enclosing if-guard at block. The helper resolves the
// identifier to the SSA parameter that represents it, then walks
// the dominator chain to find a guard whose condition compares the
// parameter against the nil constant. The operator the rule names
// selects which boolean the helper returns. Operands that are not
// identifiers, or identifiers that do not resolve to a parameter,
// fall through undecided — local-identifier narrowing is left to
// the forward walk's one-hop alias chain because tracking a local
// across a guard requires reads of versioned SSA registers that
// sit outside this helper's scope.
func blockGuardNilness(pass *analysis.Pass, ssaFn *ssa.Function, expr ast.Expr, op dsl.CompareOp, block *ssa.BasicBlock) (bool, bool) {
	if block == nil {
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
	return narrowedByDominatingGuard(value, block, op)
}

// narrowedByDominatingGuard walks the idom chain of block and asks
// each terminating *ssa.If whether its condition decides nilness
// for value on the branch that dominates block. The helper stops
// at the function entry. Cycles are impossible because the idom
// relation is a tree.
func narrowedByDominatingGuard(value ssa.Value, block *ssa.BasicBlock, op dsl.CompareOp) (bool, bool) {
	for cur := block; cur != nil; cur = cur.Idom() {
		idom := cur.Idom()
		if idom == nil {
			return false, false
		}
		ifInstr, ok := terminatingIf(idom)
		if !ok {
			continue
		}
		side, ok := dominantSide(ifInstr, block)
		if !ok {
			continue
		}
		if held, decided := ifGuardDecidesNilOp(ifInstr.Cond, value, side, op); decided {
			return held, true
		}
	}
	return false, false
}

// ifGuardDecidesNilOp reports whether the terminating If condition
// proves the nil-check operand on the branch the dominator walk
// selected. The condition must compare value against the nil
// constant; EQL on the then side proves the value is nil, NEQ
// proves it is non-nil, and the else side flips both senses. The
// rule's operator (`==` or `!=`) selects which boolean the helper
// returns.
func ifGuardDecidesNilOp(cond ssa.Value, value ssa.Value, then bool, op dsl.CompareOp) (bool, bool) {
	bop, ok := cond.(*ssa.BinOp)
	if !ok {
		return false, false
	}
	if !compareValueAgainstNilConst(bop, value) {
		return false, false
	}
	var sideIsNil bool
	switch {
	case bop.Op == token.EQL && then:
		sideIsNil = true
	case bop.Op == token.NEQ && then:
		sideIsNil = false
	case bop.Op == token.EQL && !then:
		sideIsNil = false
	case bop.Op == token.NEQ && !then:
		sideIsNil = true
	default:
		return false, false
	}
	switch op {
	case dsl.OpEq:
		return sideIsNil, true
	case dsl.OpNeq:
		return !sideIsNil, true
	}
	return false, false
}

// compareValueAgainstNilConst reports whether the BinOp compares
// value against the nil constant on either operand position. The
// SSA builder leaves the operand order unspecified, so the helper
// accepts both `value OP nil` and `nil OP value`.
func compareValueAgainstNilConst(bop *ssa.BinOp, value ssa.Value) bool {
	switch {
	case bop.X == value && isNilSSAConst(bop.Y):
		return true
	case bop.Y == value && isNilSSAConst(bop.X):
		return true
	}
	return false
}

// isNilSSAConst reports whether v is the untyped-nil SSA constant.
// The nil-fact narrowing only cares about the bare nil literal; a
// typed-nil conversion shows up through the same *ssa.Const shape
// with IsNil() reporting true.
func isNilSSAConst(v ssa.Value) bool {
	c, ok := v.(*ssa.Const)
	if !ok {
		return false
	}
	return c.IsNil()
}
