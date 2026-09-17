package analysis

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// validateNilDeclCalleeSelf walks every function that carries a
// signature decl (the doc-comment marker introduces it; see
// nilDeclMarker) and reports body-level contradictions the
// declaration pins as ill-formed. Four shapes contribute:
//
//   - The body reassigns a parameter declared `!` to a statically-
//     nil right-hand side. The decl states "the value is non-nil
//     across this signature position"; overwriting the value with
//     nil contradicts the decl on first principles.
//   - The body reassigns the receiver declared `!` to nil for the
//     same reason as the parameter case.
//   - A return statement supplies the bare nil literal (or a
//     typed-nil conversion) at a return position the decl pins as
//     `!`. The decl promises non-nil at the call site; a literal
//     nil at the return slot is a direct contradiction.
//   - A leading `if x == nil` guard whose then-branch short-
//     circuits applies to a parameter or receiver the decl pins as
//     `!`. The decl already proves non-nil at entry, so the guard
//     is dead.
//
// The pass is intentionally AST-only. Forward dataflow that lifts
// a nillable value into a non-nil-required position is not
// covered by this pass. Keeping this surface AST-only keeps the
// false-positive risk minimal: every reported site is recognised
// by a direct syntactic pattern.
func validateNilDeclCalleeSelf(pass *analysis.Pass, state *passState) {
	decls := state.nilDeclSet(pass)
	if len(decls) == 0 {
		return
	}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			obj := pass.TypesInfo.Defs[fn.Name]
			if obj == nil {
				continue
			}
			sig, ok := decls[obj]
			if !ok {
				continue
			}
			reportNilDeclReassign(pass, fn, sig)
			reportNilDeclReturnNil(pass, fn, sig)
			reportNilDeclDeadGuards(pass, fn, sig)
		}
	}
}

// reportNilDeclReassign emits a diagnostic for every direct
// assignment in fn.Body whose left-hand side names a parameter or
// receiver the decl pins as `!`, and whose right-hand side is
// statically nil. The receiver case shares the same machinery
// because both ride into nonNilParamAndRecvNames as one name set.
func reportNilDeclReassign(pass *analysis.Pass, fn *ast.FuncDecl, sig *dsl.NilSignature) {
	nonNil := nonNilParamAndRecvNames(fn, sig)
	if len(nonNil) == 0 {
		return
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			ident, ok := lhs.(*ast.Ident)
			if !ok {
				continue
			}
			if _, isNonNil := nonNil[ident.Name]; !isNonNil {
				continue
			}
			if i >= len(assign.Rhs) {
				continue
			}
			if !argumentIsStaticallyNil(pass, assign.Rhs[i]) {
				continue
			}
			vowReportNilSafetyf(
				pass,
				assign.Pos(),
				"vow[nil-safety]: %s reassigned to nil; %s declared %s ! at this position",
				ident.Name,
				nilDeclMarker,
				ident.Name,
			)
		}
		return true
	})
}

// reportNilDeclReturnNil emits a diagnostic for every return
// statement that supplies a statically-nil expression at a return
// position the decl pins as `!`. The check operates on the literal
// shape of the return expression so a transitive flow (a nillable
// local that reaches the slot) falls through unreported and is
// not covered here; the SSA forward walk handles it.
func reportNilDeclReturnNil(pass *analysis.Pass, fn *ast.FuncDecl, sig *dsl.NilSignature) {
	if sig == nil || len(sig.Returns) == 0 {
		return
	}
	nonNilSlots := nonNilReturnSlots(sig)
	if len(nonNilSlots) == 0 {
		return
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, idx := range nonNilSlots {
			if idx >= len(ret.Results) {
				continue
			}
			if !argumentIsStaticallyNil(pass, ret.Results[idx]) {
				continue
			}
			vowReportNilSafetyf(
				pass,
				ret.Results[idx].Pos(),
				"vow[nil-safety]: return position %d is nil; %s declared ! at this return slot",
				idx+1,
				nilDeclMarker,
			)
		}
		return true
	})
}

// reportNilDeclDeadGuards emits a diagnostic for every body-leading
// `if x == nil` guard whose parameter or receiver the decl pins as
// `!`. The decl already proves non-nil at entry, so the guard's
// then-branch is unreachable and almost always signals that the
// author meant the looser `?` decl at that position instead.
func reportNilDeclDeadGuards(pass *analysis.Pass, fn *ast.FuncDecl, sig *dsl.NilSignature) {
	nonNil := nonNilParamAndRecvNames(fn, sig)
	if len(nonNil) == 0 {
		return
	}
	for _, stmt := range leadingIfStatements(fn.Body) {
		paramName, ok := ifCondIsNilCheckOnParam(stmt.Cond, nonNil)
		if !ok {
			continue
		}
		if !branchShortCircuits(stmt.Body) {
			continue
		}
		vowReportNilSafetyf(
			pass,
			stmt.Pos(),
			"vow[nil-safety]: guard on %s is dead; %s declared %s ! at this position (the value is already non-nil at entry)",
			paramName,
			nilDeclMarker,
			paramName,
		)
	}
}

// nonNilParamAndRecvNames returns the set of identifier names that
// the decl pins as `!`, covering both the regular parameter list
// and the receiver. Positions left at platform or declared `?`
// contribute no entry so the reassign and dead-guard checks stay
// silent on them. Unnamed positions contribute no entry either
// since the body cannot name them on the left-hand side or inside
// a guard condition.
func nonNilParamAndRecvNames(fn *ast.FuncDecl, sig *dsl.NilSignature) map[string]struct{} {
	if sig == nil {
		return nil
	}
	out := map[string]struct{}{}
	if sig.Recv != nil && sig.Recv.Nullness == dsl.NullnessNonNil && fn.Recv != nil {
		for _, field := range fn.Recv.List {
			for _, name := range field.Names {
				out[name.Name] = struct{}{}
			}
		}
	}
	paramNames := signatureParamNames(fn)
	for i, decl := range sig.Params {
		if decl == nil || decl.Nullness != dsl.NullnessNonNil {
			continue
		}
		if i >= len(paramNames) || paramNames[i] == "" {
			continue
		}
		out[paramNames[i]] = struct{}{}
	}
	return out
}

// nonNilReturnSlots returns the indices of return positions the
// decl pins as `!`. Positions left at platform or declared `?`
// contribute no entry so the return check stays silent on them.
func nonNilReturnSlots(sig *dsl.NilSignature) []int {
	if sig == nil {
		return nil
	}
	out := make([]int, 0, len(sig.Returns))
	for i, decl := range sig.Returns {
		if decl == nil || decl.Nullness != dsl.NullnessNonNil {
			continue
		}
		out = append(out, i)
	}
	return out
}
