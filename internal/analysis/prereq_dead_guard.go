package analysis

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// validatePrereqDeadGuards walks every vow:cond-annotated
// function and reports a body-leading `if x == nil` guard whose
// parameter the function's own `nonnil` prereq already proves
// non-nil at entry. The prereq pre-establishes non-nil before
// any body statement runs, so the guard's then-branch is
// unreachable and almost always signals that the author meant
// the looser `nil | nonnil x` shape. The recogniser stays
// vow:cond-driven so the same check covers sentinel-
// narrowing prereqs (where `nonnil err` is the common form for
// pinning an error-narrowing branch) alongside any future
// prereq-driven non-nil declaration that lands without going
// through the vow:nil signature-mirror surface.
func validatePrereqDeadGuards(pass *analysis.Pass, state *passState) {
	for _, file := range pass.Files {
		scope := state.ruleScope(pass)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			conds, ok := state.annotation(fn, scope)
			if !ok {
				continue
			}
			reportNonNilDeadGuards(pass, fn, conds)
		}
	}
}

// reportNonNilDeadGuards emits a diagnostic for every body-leading
// `if x == nil` guard whose parameter any of the function's
// `vow:cond` rules declares as `nonnil`. Authors that hit this
// diagnostic typically meant the looser `nil | nonnil x` prereq.
func reportNonNilDeadGuards(pass *analysis.Pass, fn *ast.FuncDecl, conds []*dsl.Condition) {
	nonNilParams := nonNilPrereqParamNames(fn, conds)
	if len(nonNilParams) == 0 {
		return
	}
	for _, stmt := range leadingIfStatements(fn.Body) {
		paramName, ok := ifCondIsNilCheckOnParam(stmt.Cond, nonNilParams)
		if !ok {
			continue
		}
		if !branchShortCircuits(stmt.Body) {
			continue
		}
		vowReportNilSafetyf(
			pass,
			stmt.Pos(),
			"vow[nil-safety]: guard on %s is dead; callee declared nonnil %s in vow:cond (the value is already non-nil at entry)",
			paramName,
			paramName,
		)
	}
}

// nonNilPrereqParamNames inspects parsed conditions for arrow-rule
// prereq positions whose predicate is `nonnil` and pairs each
// position with the named parameter at that index. Positions whose
// signature lacks a name (unnamed parameter) contribute no entry
// because a guard expression cannot reference an unnamed position.
// The signature-side parameter list is sourced through the same
// helper the caller-side argument check uses so the two surfaces
// expand grouped declarations identically.
func nonNilPrereqParamNames(fn *ast.FuncDecl, conds []*dsl.Condition) map[string]struct{} {
	names := signatureParamNames(fn)
	out := map[string]struct{}{}
	for _, cond := range conds {
		if cond == nil || cond.Prereq == nil {
			continue
		}
		if len(cond.Subject) > 0 {
			// Subject-scoped conditions describe a higher-order
			// contract anchored on the named callback, not on the
			// enclosing function's parameters, so the prereq's
			// positional index would mis-resolve against the
			// enclosing signature and credit unrelated guards as
			// dead.
			continue
		}
		for i, pos := range cond.Prereq.Positions {
			if pos.Single == nil || pos.Single.Pred == nil {
				continue
			}
			if !prereqPredicateIsNonNil(pos.Single.Pred) {
				continue
			}
			if i >= len(names) || names[i] == "" {
				continue
			}
			out[names[i]] = struct{}{}
		}
	}
	return out
}

// prereqPredicateIsNonNil reports whether pred carries the literal
// `nonnil` shape. Only `Not{Inner: Nil{}}` qualifies; richer
// predicate compositions (`Not{ParenExpr}`, `Not{Sum}`, sentinel-
// equality, `nonzero`, ...) are out of scope so the dead-guard
// recogniser stays anchored to the single surface form authors
// type for "this position is non-nil at entry".
func prereqPredicateIsNonNil(pred dsl.Predicate) bool {
	not, ok := pred.(dsl.Not)
	if !ok {
		return false
	}
	_, ok = not.Inner.(dsl.Nil)
	return ok
}
