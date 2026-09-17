package analysis

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// reasonRecvNonNilDecl is the human-readable rationale a nil-safety
// diagnostic uses when the callee's vow:nil signature pins the
// receiver as non-nil (`!.()`). Centralised so the same-package and
// cross-package paths produce byte-identical reasons; a future
// change to the wording lands in one place.
const reasonRecvNonNilDecl = "callee declared receiver as non-nil via vow:nil"

// validateCallerReceiverNilSafety scans every method-call site in
// pass.Files and reports when the receiver expression is
// statically known to be nil. Two branches reach a diagnostic:
//
//   - The callee carries a vow:nil signature whose receiver decl
//     pins the receiver as non-nil (`!.()`) — proven-nil receiver
//     is a hard violation, and the diagnostic quotes the contract
//     so the author can confirm the constraint by reading the
//     callee's vow:nil marker.
//   - The callee has no vow declaration — proven-nil receiver
//     surfaces a default-warn diagnostic because nil-receiver
//     method calls are Go's most common runtime panic source.
//
// A callee whose vow:nil recv decl pins the receiver as nillable
// (`?.()`) stays silent at the call site because the contract
// explicitly admits a nil receiver; same with a callee whose
// signature decl omits the receiver layer (the author has not
// claimed a receiver-nil constraint). Both same-package and
// cross-package callees route through nilDeclForFunc, so the
// fact-imported signature carries the constraint without the
// callee's AST in scope. The nil-ability judgement reuses
// argumentIsStaticallyNil from the argument-side check so the
// literal nil and typed-nil conversion shapes are recognised
// uniformly across argument and receiver surfaces.
func validateCallerReceiverNilSafety(pass *analysis.Pass, state *passState) {
	declByObj := indexFuncDecls(pass)
	for _, file := range pass.Files {
		scope := state.ruleScope(pass)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			checkReceiverNilSafety(pass, state, call, scope, declByObj)
			return true
		})
	}
}

// checkReceiverNilSafety inspects a single method-call site and
// emits at most one diagnostic when the receiver is statically nil
// and the callee's declaration mode reaches a reported branch.
// The vow-declared-nillable-receiver branch and the
// no-receiver-decl-on-vow-signature branch both stay silent
// because the author has not claimed a non-nil-receiver
// constraint.
func checkReceiverNilSafety(pass *analysis.Pass, state *passState, call *ast.CallExpr, scope dsl.RuleScope, declByObj map[types.Object]*ast.FuncDecl) {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return
	}
	receiver := sel.X
	if !argumentIsStaticallyNil(pass, receiver) {
		return
	}
	fn := methodObjectFromSelector(pass, sel)
	if fn == nil {
		return
	}
	if !receiverTypeIsNillable(fn) {
		return
	}
	switch receiverDeclMode(pass, state, fn, scope, declByObj) {
	case receiverModeStrict:
		vowReportNilSafetyf(
			pass,
			receiver.Pos(),
			"vow[nil-safety]: receiver of %s is nil; %s",
			fn.Name(),
			reasonRecvNonNilDecl,
		)
	case receiverModeUnannotated:
		vowReportNilSafetyf(
			pass,
			receiver.Pos(),
			"vow[nil-safety]: receiver of %s is nil; method call panics on a nil receiver",
			fn.Name(),
		)
	case receiverModeSilent:
		// vow:nil signature explicitly admits a nil receiver (`?.()`),
		// or the signature decl omits the receiver layer altogether
		// (no `<decl>.` prefix). Either way the author has not
		// claimed a non-nil-receiver constraint, so the call site
		// surfaces no diagnostic. Changing the receiver decl to
		// `!.()` flips the same call into the strict branch
		// without touching the caller.
	}
}

// receiverMode discriminates the three branches the receiver-nil
// check tracks: vow:nil signature pinning the receiver as
// non-nil (strict), vow:nil signature admitting a nil receiver
// or omitting the receiver layer (silent), and unannotated
// (default warn).
type receiverMode int

const (
	receiverModeSilent receiverMode = iota
	receiverModeStrict
	receiverModeUnannotated
)

// receiverDeclMode classifies the callee's vow declaration into
// one of the three receiver-nil branches. The vow:nil signature
// lookup goes through nilDeclForFunc so the same-package map and
// the imported signatureNilFact share the same dispatch; the
// cross-package recv-decl fact (the Recv field on NilSignature)
// reaches the strict branch without the callee's AST in scope.
// A same-package callee that carries a vow:cond annotation
// but no vow:nil recv decl falls into the silent branch — the
// author has acknowledged vow's surface on this function but has
// not claimed a non-nil-receiver constraint, so the receiver-nil
// check stays out of the way. Cross-package callees that have
// no exported signature fact fall through to the default-warn
// branch; the fact mechanism carries no "vow declared but
// no recv decl" signal so a future fact extension can fold this
// case back into the silent branch without touching the caller.
func receiverDeclMode(pass *analysis.Pass, state *passState, fn *types.Func, scope dsl.RuleScope, declByObj map[types.Object]*ast.FuncDecl) receiverMode {
	if sig := nilDeclForFunc(pass, state, fn); sig != nil {
		if sig.Recv == nil {
			return receiverModeSilent
		}
		switch sig.Recv.Nullness {
		case dsl.NullnessNonNil:
			return receiverModeStrict
		case dsl.NullnessNillable:
			return receiverModeSilent
		}
	}
	if decl := declByObj[fn]; decl != nil {
		if _, ok := state.annotation(decl, scope); ok {
			return receiverModeSilent
		}
	}
	return receiverModeUnannotated
}

// methodObjectFromSelector resolves the selector's method object
// when the selection names a method. Returns nil for field
// accesses, package-qualified function references, or unresolved
// identifiers so the caller can treat the call as out of scope.
func methodObjectFromSelector(pass *analysis.Pass, sel *ast.SelectorExpr) *types.Func {
	obj := pass.TypesInfo.ObjectOf(sel.Sel)
	if obj == nil {
		return nil
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return nil
	}
	return fn
}

// receiverTypeIsNillable reports whether fn's receiver carries a
// type whose zero value is nil — pointer, interface, slice, map,
// channel, or function. Value receivers backed by a struct or a
// basic kind can never be nil at the language level, so a method
// call on such a receiver is out of scope here.
func receiverTypeIsNillable(fn *types.Func) bool {
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	return typeIsNillable(sig.Recv().Type())
}
