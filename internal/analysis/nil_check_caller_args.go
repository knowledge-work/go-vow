package analysis

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// validateCallerArgNilSafety scans every call site in pass.Files
// and reports when an argument is statically known to be nil at a
// position the callee's vow:nil signature pins as non-nil (`!`).
// The argument-nil-ability judgement is intentionally conservative
// on the false-positive side: only arguments the helper can
// statically prove to be nil (the bare nil literal or an explicit
// typed-nil conversion such as `(*T)(nil)`) are reported. Values
// whose nil-ability cannot be decided without flow analysis fall
// through unreported and are picked up later by the callee self-
// validation pass and the SSA forward walk. The conservative
// stance keeps vow-declared callees producing hard lint errors
// when a proof exists; the receiver-side default-warn path and
// the richer flow-based callee tracking are handled by their own
// dedicated passes.
func validateCallerArgNilSafety(pass *analysis.Pass, state *passState) {
	srcFuncs := ssaSourceFunctions(pass)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, isFn := decl.(*ast.FuncDecl)
			if !isFn {
				inspectCallsForArgNilSafety(pass, state, decl, nil)
				continue
			}
			if fn.Body == nil {
				continue
			}
			inspectCallsForArgNilSafety(pass, state, fn.Body, findSSAFunction(srcFuncs, fn))
		}
	}
}

// inspectCallsForArgNilSafety walks every call site under node and
// checks it against the callee's contract. ssaFn is the SSA form of the
// function whose body node belongs to, or nil for a call outside any
// body — a package-level initialiser — where the guard-based discharge
// has no control-flow graph to consult and stays out of the way.
func inspectCallsForArgNilSafety(pass *analysis.Pass, state *passState, node ast.Node, ssaFn *ssa.Function) {
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		checkCallNilSafety(pass, state, ssaFn, call)
		return true
	})
}

// ssaSourceFunctions returns the SSA functions built for this pass's
// source files, or nil when the SSA result is absent. A nil slice
// leaves every lookup unresolved, which degrades the guard-based
// discharge to "not proved" rather than failing the pass.
func ssaSourceFunctions(pass *analysis.Pass) []*ssa.Function {
	result, ok := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	if !ok || result == nil {
		return nil
	}
	return result.SrcFuncs
}

// checkCallNilSafety resolves call's callee, builds the list of
// argument positions the callee's vow:nil signature pins as
// non-nil, and emits one diagnostic per statically-nil argument
// that sits in one of those positions. A call whose callee
// carries no vow:nil signature (or whose signature leaves every
// argument position unconstrained) returns without inspecting
// the arguments — the same callee surface drives both the
// lookup and the emission so the two stay aligned.
func checkCallNilSafety(pass *analysis.Pass, state *passState, ssaFn *ssa.Function, call *ast.CallExpr) {
	callee := calleeObject(pass, call)
	if callee == nil {
		return
	}
	required := nonNilRequirementsForCallee(pass, state, callee)
	if len(required) == 0 {
		return
	}
	for _, req := range required {
		if req.argIndex < 0 || req.argIndex >= len(call.Args) {
			continue
		}
		arg := call.Args[req.argIndex]
		if argumentIsStaticallyNil(pass, arg) {
			vowReportNilSafetyf(
				pass,
				arg.Pos(),
				"vow[nil-safety]: argument %d (%s) is nil; %s",
				req.argIndex+1,
				req.paramName,
				req.reason,
			)
			continue
		}
		if fieldName, ok := argumentIsDeclaredNillableField(pass, state, arg); ok {
			if fieldArgNarrowed(pass, ssaFn, call, arg) {
				continue
			}
			vowReportNilSafetyf(
				pass,
				arg.Pos(),
				"vow[nil-safety]: argument %d (%s) is potentially nil (field %s %s); %s",
				req.argIndex+1,
				req.paramName,
				fieldName,
				reasonFieldNilDecl,
				req.reason,
			)
		}
	}
}

// argumentIsDeclaredNillableField reports whether arg is a selector
// expression whose terminal field carries a nillable (`?`) field
// decl. The check anchors on the selector's identity (resolved
// through the type-checker) so a field renamed or aliased through
// embedding still flows back to the same Object key. Same-package
// fields are read from the in-memory fieldNilSet; fields whose
// declaring package lives outside the current pass are read from
// the imported fieldNilFact so the cross-package trace surfaces a
// nillable-field proof at the call site. Fields with no decl, or
// declared `!`, return ok=false so the caller-side surface stays
// silent on positions the field declaration left unconstrained or
// proved non-nil.
func argumentIsDeclaredNillableField(pass *analysis.Pass, state *passState, arg ast.Expr) (string, bool) {
	sel, ok := ast.Unparen(arg).(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	obj := pass.TypesInfo.ObjectOf(sel.Sel)
	if obj == nil {
		return "", false
	}
	decl := fieldNilForField(pass, state, obj)
	if decl == nil {
		return "", false
	}
	if decl.Nullness != dsl.NullnessNillable {
		return "", false
	}
	return sel.Sel.Name, true
}

// nonNilRequirementsForCallee resolves the non-nil-required
// positions for a callee. The same nilDeclForFunc helper resolves
// the vow:nil signature for both same-package and cross-package
// callees (the latter through the imported signatureNilFact), so
// the dispatch reduces to a single per-position translation. The
// parameter names come from the types.Signature so the
// diagnostic carries the same labels regardless of whether the
// declaration sits in this package or a foreign one.
func nonNilRequirementsForCallee(pass *analysis.Pass, state *passState, callee types.Object) []nonNilArgRequirement {
	sig := nilDeclForFunc(pass, state, callee)
	if sig == nil {
		return nil
	}
	fn, ok := callee.(*types.Func)
	if !ok {
		return nil
	}
	funcSig, ok := fn.Type().(*types.Signature)
	if !ok {
		return nil
	}
	return nilDeclParamRequirements(sig, signatureParamNamesFromTypes(funcSig))
}

// signatureParamNamesFromTypes returns the declared names of sig's
// regular parameters in argument-index order so caller-side checks
// driven by a fact (which has no *ast.FuncDecl in scope) supply
// the same name shape signatureParamNames would have produced from
// the AST. Unnamed parameters carry the empty string, matching the
// AST helper's degradation on unnamed positions so a cross-
// package diagnostic renders the same "argument N ()" form a
// same-package one would have rendered.
func signatureParamNamesFromTypes(sig *types.Signature) []string {
	if sig == nil {
		return nil
	}
	params := sig.Params()
	out := make([]string, params.Len())
	for i := 0; i < params.Len(); i++ {
		out[i] = params.At(i).Name()
	}
	return out
}

// nonNilArgRequirement names a single argument position the
// callee declares as non-nil-required and pairs it with the
// parameter's declared name and the human-readable reason the
// caller-side diagnostic surfaces. The reason quotes the marker
// that authored the requirement (the `!` token on a
// parameter position inside a vow:nil signature) so a reader
// can inspect the callee's signature to confirm the constraint.
type nonNilArgRequirement struct {
	argIndex  int
	paramName string
	reason    string
}

// signatureParamNames returns the declared names of decl's regular
// (non-receiver) parameters in argument-index order. Grouped
// declarations (`a, b int`) expand into one entry per name. Unnamed
// positions carry the empty string so the index alignment with an
// *ast.CallExpr's Args stays intact.
func signatureParamNames(decl *ast.FuncDecl) []string {
	if decl == nil || decl.Type == nil || decl.Type.Params == nil {
		return nil
	}
	var names []string
	for _, field := range decl.Type.Params.List {
		if len(field.Names) == 0 {
			names = append(names, "")
			continue
		}
		for _, n := range field.Names {
			names = append(names, n.Name)
		}
	}
	return names
}

// paramNameAt returns names[i] when i is in range, or the empty
// string otherwise. Callers diagnose unnamed positions by rendering
// the empty name verbatim — the surrounding "argument N (...)"
// format degrades to "argument N ()" rather than panicking on an
// out-of-range index.
func paramNameAt(names []string, i int) string {
	if i < 0 || i >= len(names) {
		return ""
	}
	return names[i]
}

// argumentIsStaticallyNil reports whether arg is provably nil at
// its appearance position without consulting flow analysis. Two
// shapes qualify:
//
//   - The bare identifier `nil`, optionally wrapped in
//     parenthesises (`(((nil)))`). The untyped nil constant lands
//     in the AST as an *ast.Ident named "nil"; matching the
//     identifier name after stripping parens is sufficient and
//     pins the diagnostic to the literal the author typed.
//   - A typed-nil conversion such as `(*T)(nil)`, `[]T(nil)`, or
//     `(map[string]int)(nil)`. The outer expression is an
//     *ast.CallExpr with one argument (the bare nil literal) whose
//     callee resolves to a type rather than a function. The type-
//     checker reports this through TypeAndValue.IsType().
//
// Both shapes carry a literal proof of nil-ness in the source;
// other shapes (variables, function returns, type assertions) are
// out of scope and fall through unreported to keep the caller-side
// diagnostic free of false positives.
func argumentIsStaticallyNil(pass *analysis.Pass, arg ast.Expr) bool {
	if isBareNilIdent(arg) {
		return true
	}
	return isTypedNilConversion(pass, arg)
}

// isBareNilIdent reports whether arg is the bare identifier `nil`,
// optionally wrapped in parens. The type-checker assigns the
// predeclared `nil` to such an ident; the textual identity is
// sufficient and matches what authors typed at the call site so
// the diagnostic anchors to the literal token.
func isBareNilIdent(arg ast.Expr) bool {
	ident, ok := ast.Unparen(arg).(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "nil"
}

// isTypedNilConversion reports whether arg is a typed-nil
// conversion such as `(*T)(nil)`. The shape carries one argument
// (the bare nil literal) and a callee expression that the type-
// checker treats as a type rather than a function. Consulting
// pass.TypesInfo.Types[call.Fun].IsType() lets the helper skip
// real calls whose returned value happens to be nil — those are
// out of scope for the literal-only judgement here.
func isTypedNilConversion(pass *analysis.Pass, arg ast.Expr) bool {
	call, ok := ast.Unparen(arg).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	if !isBareNilIdent(call.Args[0]) {
		return false
	}
	tv, ok := pass.TypesInfo.Types[call.Fun]
	if !ok {
		return false
	}
	return tv.IsType()
}
