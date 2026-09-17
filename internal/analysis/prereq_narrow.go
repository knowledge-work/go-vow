package analysis

import (
	"go/ast"

	"golang.org/x/tools/go/ssa"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// prereqHoldsAtReturn reports whether every position of a
// `vow:cond` Prerequisite is provably narrowed at the SSA block
// containing ret. A nil Prerequisite is the trivial wildcard `*` and
// always holds — callers use this helper uniformly so the trivial
// path needs no separate branch.
//
// The decision is conservatively under-approximate: only narrowing
// patterns the helper recognises return true, anything else returns
// false so return-requirement enforcement skips the return. The
// motivation for under-approximation is false-positive avoidance —
// authors who see a diagnostic at a return inside an unanalysed
// control shape would lose trust in the linter and slow migration;
// authors who miss a leak that the linter could have caught with
// richer narrowing still get the other detection passes
// (must-consume, SSA flow) as a safety net.
//
// Supported narrowing patterns:
//
//   - `nonnil x` proved by `if x != nil { ... }` on the then side, or
//     by `if x == nil { ... }` on the else side.
//   - `nil x`  proved by the mirror pattern.
//   - `nonzero x` proved by `if x != 0` / `x != ""` / `x != false`
//     guard branches.
//   - `zero x`  proved by the mirror pattern.
//
// Other parameter requirement shapes — sentinel-equality
// (`x = ErrFoo`), type switches, transitive equality — are not
// recognised and are not recognised at this layer alongside
// their grammar support.
func prereqHoldsAtReturn(fn *ast.FuncDecl, ssaFn *ssa.Function, ret *ssa.Return, prereq *dsl.Prerequisite) bool {
	if prereq == nil {
		return true
	}
	if ssaFn == nil || ret == nil {
		return false
	}
	block := ret.Block()
	recvCount := receiverParamCount(fn)
	for i, pos := range prereq.Positions {
		if !positionHoldsAtBlock(ssaFn, block, recvCount+i, pos) {
			return false
		}
	}
	return true
}

// receiverParamCount returns the number of SSA Parameters consumed
// by the method receiver, so a prereq position index can be added
// to it to land on the correct regular argument. Free functions
// return 0; methods return the receiver field's arity (typically
// 1; the comma-grouped form `func (a, b T) M()` is not legal Go,
// so the typical case is the only case here, but the linear count
// keeps the code shape uniform with signatureArity).
func receiverParamCount(fn *ast.FuncDecl) int {
	if fn == nil || fn.Recv == nil {
		return 0
	}
	n := 0
	for _, field := range fn.Recv.List {
		if len(field.Names) == 0 {
			n++
			continue
		}
		n += len(field.Names)
	}
	return n
}

// positionHoldsAtBlock dispatches a single prereq position to the
// predicate-specific narrowing matcher. The Term.Type field carries
// documentation only (the position name); param binding is by
// signature index so a function-level rename does not break the
// annotation.
func positionHoldsAtBlock(ssaFn *ssa.Function, block *ssa.BasicBlock, paramIdx int, pos dsl.PositionSpec) bool {
	// Prereq positions are validated upstream to never carry a Sum;
	// only the Single shape lands here.
	term := pos.Single
	if term == nil {
		return false
	}
	if term.Pred == nil {
		// Bare type or placeholder with no predicate is a no-op
		// constraint — the position holds trivially.
		return true
	}
	if paramIdx < 0 || paramIdx >= len(ssaFn.Params) {
		return false
	}
	param := ssaFn.Params[paramIdx]
	return predicateHoldsForParam(param, block, term.Pred)
}

// predicateHoldsForParam decides whether the named predicate holds
// for param at block. Each supported predicate maps to one of the
// two atomic facts (param-is-nil / param-is-zero) and one of the
// two senses (positive / negated); branchProvesFact handles the
// SSA-side recognition uniformly.
func predicateHoldsForParam(param *ssa.Parameter, block *ssa.BasicBlock, pred dsl.Predicate) bool {
	switch p := pred.(type) {
	case dsl.Nil:
		return blockProvesFact(block, valueSubject{value: param}, factNil, true)
	case dsl.Zero:
		return blockProvesFact(block, valueSubject{value: param}, factZero, true)
	case dsl.Not:
		switch p.Inner.(type) {
		case dsl.Nil:
			return blockProvesFact(block, valueSubject{value: param}, factNil, false)
		case dsl.Zero:
			return blockProvesFact(block, valueSubject{value: param}, factZero, false)
		}
	}
	return false
}
