package analysis

import (
	"go/ast"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// validateSubjectScopedLogicalConds walks every function-doc
// vow:cond[subject] marker carrying a logical-arrow rule and
// validates that each Expression's reference resolves within the
// callback subject's signature scope. The marker delegates the
// rule's evaluation to the callback rather than to the enclosing
// function, so references must name a parameter, the receiver,
// or a named return of the callback's own signature. A reference
// that does not resolve surfaces a diagnostic at the function
// declaration so the author sees the typo before the contract
// silently disappears.
//
// Non-callback subjects (regular value parameters whose type is
// not a function signature) skip the check because the rule body
// is interpreted under a different layer — flow analysis on a
// value parameter, or a tag-based discriminator on a struct
// field — that a layer wires through.
func validateSubjectScopedLogicalConds(pass *analysis.Pass, state *passState) {
	for _, file := range pass.Files {
		scope := state.ruleScope(pass)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			conds, ok := state.annotation(fn, scope)
			if !ok {
				continue
			}
			for _, cond := range conds {
				if cond == nil || len(cond.Subject) == 0 {
					continue
				}
				validateSubjectScopedCond(pass, fn, cond)
			}
		}
	}
}

// validateSubjectScopedCond drives the per-condition check. The
// helper walks the subject chain through nested callback
// signatures, narrows to the innermost callback signature, and
// then validates every logical-arrow rule's operands against that
// signature scope. Chain failures (unresolved segment or
// intermediate non-callback) are reported elsewhere — by the
// vow:cond discovery walk that runs ahead of this validator — so
// this helper silently returns when the chain does not resolve.
// A successfully resolved chain whose innermost slot is not
// callback-typed also returns silently because a layer-specific
// handler covers value parameters and tag discriminators.
func validateSubjectScopedCond(pass *analysis.Pass, fn *ast.FuncDecl, cond *dsl.Condition) {
	sig, _, status := resolveSignatureSubjectChain(pass, fn, cond.Subject)
	if status != chainResolved || sig == nil {
		return
	}
	for _, r := range cond.Rules {
		if r == nil || r.Kind != dsl.RuleArrow || r.Arrow == nil {
			continue
		}
		if r.Arrow.Direction == dsl.DirStructural {
			continue
		}
		validateLogicalOperandInCallbackSig(pass, fn, cond.Subject, sig, r.Arrow.Left)
		validateLogicalOperandInCallbackSig(pass, fn, cond.Subject, sig, r.Arrow.Right)
	}
}

// validateLogicalOperandInCallbackSig reports when an operand's
// reference does not name any slot in the innermost callback's
// signature. Bare identifier references match against the
// receiver, parameters, or named returns of the callback; `$N`
// positional references match against the return list by index.
// Postfix-chained references are not inspected at this layer and
// stay silent because the callback's field structure is decided
// by the type checker rather than by the rule's surface scope.
//
// The chain argument carries the full `[X][Y]...` qualifier so
// the diagnostic quotes the qualifier the author wrote, letting
// the reader trace which nesting level the offending operand
// belongs to.
func validateLogicalOperandInCallbackSig(pass *analysis.Pass, fn *ast.FuncDecl, chain dsl.SubjectChain, sig *types.Signature, expr *dsl.Expression) {
	if expr == nil {
		return
	}
	if len(expr.Ref.Path) > 0 {
		return
	}
	switch expr.Ref.Kind {
	case dsl.RefIdent:
		if !callbackSigHasIdentRef(sig, expr.Ref.Name) {
			vowReportf(pass, fn.Pos(),
				"vow[sentinel-error]: vow:cond%s references %q, which is not a parameter, receiver, or named return of the callback signature",
				formatSubjectChain(chain), expr.Ref.Name)
		}
	case dsl.RefDollar:
		n, err := strconv.Atoi(expr.Ref.Name)
		if err != nil || n < 1 {
			return
		}
		results := sig.Results()
		if results == nil || n > results.Len() {
			vowReportf(pass, fn.Pos(),
				"vow[sentinel-error]: vow:cond%s references $%d but the callback signature has fewer return positions",
				formatSubjectChain(chain), n)
		}
	}
}

// validateSubjectScopedEmits walks vow:emit markers carrying a
// `[subject]` scope — both the function-doc form attached to a
// declaration and the statement-scope form anchored to a line
// inside a body — and validates that the resolved subject is
// function-typed. The marker propagates emission credit through
// the callback when the enclosing function calls it; a non-
// callback subject cannot carry that flow, so the marker
// surfaces a diagnostic at the marker's anchor (the function
// declaration for the function-doc form, the comment for the
// statement-scope form).
func validateSubjectScopedEmits(pass *analysis.Pass, state *passState) {
	_ = state
	for _, file := range pass.Files {
		validateFuncDocEmitSubjects(pass, file)
		validateStatementScopeEmitSubjects(pass, file)
	}
}

// validateFuncDocEmitSubjects walks fn.Doc vow:emit markers and
// reports when the chain qualifier resolves to a non-callback
// signature slot. The walker steps through the chain via
// resolveSignatureSubjectChain; an intermediate non-callback slot
// surfaces a chain-step diagnostic, and a final non-callback slot
// surfaces the existing "not a callback" diagnostic. The
// diagnostic anchors at fn.Pos() so the author sees the mismatch
// alongside the contract declaration.
func validateFuncDocEmitSubjects(pass *analysis.Pass, file *ast.File) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc == nil {
			continue
		}
		reported := map[string]struct{}{}
		for _, c := range fn.Doc.List {
			text, isLine := lineCommentBody(c)
			if !isLine {
				continue
			}
			scope, subjects, present := parseEmitMarkerLine(text)
			if !present || len(scope) == 0 || len(subjects) == 0 {
				continue
			}
			key := formatSubjectChain(scope)
			if _, dup := reported[key]; dup {
				continue
			}
			sig, failedAt, status := resolveSignatureSubjectChain(pass, fn, scope)
			if status == chainResolveFailed {
				continue
			}
			if status == chainStepFailed {
				reported[key] = struct{}{}
				reportChainStepNotCallback(pass, fn.Pos(), emitMarker, scope, failedAt)
				continue
			}
			if sig != nil {
				continue
			}
			reported[key] = struct{}{}
			reportSubjectChainNotCallback(pass, fn.Pos(), emitMarker, scope,
				"emission only propagates through function-typed slots")
		}
	}
}

// validateStatementScopeEmitSubjects walks vow:emit markers that
// sit on a body statement (rather than in a function's doc) and
// reports when the chain qualifier resolves to a non-callback
// signature slot. The walker reads the same parseEmitMarkerLine
// helper the function-doc form uses so the surface grammar stays
// consistent.
func validateStatementScopeEmitSubjects(pass *analysis.Pass, file *ast.File) {
	docComments := collectFuncDocComments(file)
	for _, group := range file.Comments {
		for _, c := range group.List {
			if _, isFuncDoc := docComments[c]; isFuncDoc {
				continue
			}
			text, isLine := lineCommentBody(c)
			if !isLine {
				continue
			}
			scope, subjects, present := parseEmitMarkerLine(text)
			if !present || len(scope) == 0 || len(subjects) == 0 {
				continue
			}
			enclosing := enclosingFuncDeclAt(file, c.Pos())
			if enclosing == nil {
				continue
			}
			sig, failedAt, status := resolveSignatureSubjectChain(pass, enclosing, scope)
			if status == chainResolveFailed {
				continue
			}
			if status == chainStepFailed {
				reportChainStepNotCallback(pass, c.Pos(), emitMarker, scope, failedAt)
				continue
			}
			if sig != nil {
				continue
			}
			reportSubjectChainNotCallback(pass, c.Pos(), emitMarker, scope,
				"emission only propagates through function-typed slots")
		}
	}
}

// callbackSignature returns the function signature carried by
// the subject ref's declared type, when the type reduces to a
// function shape. Non-function types (pointers, structs,
// channels) report ok=false so the higher-order validator stays
// focused on callbacks and leaves other shapes to the
// layer-specific handlers. When the ref names an identifier
// (named parameter or named return) the lookup goes through
// pass.TypesInfo.Defs; when the ref is positional and unnamed
// (the `[$N]` qualifier reaches an unnamed return slot) the
// lookup walks the field's declared type expression instead.
func callbackSignature(pass *analysis.Pass, ref signatureSubjectRef) (*types.Signature, bool) {
	if ref.Ident != nil {
		obj := pass.TypesInfo.Defs[ref.Ident]
		if obj == nil {
			return nil, false
		}
		t := obj.Type()
		if t == nil {
			return nil, false
		}
		sig, ok := t.Underlying().(*types.Signature)
		if !ok {
			return nil, false
		}
		return sig, true
	}
	if ref.Field == nil || ref.Field.Type == nil {
		return nil, false
	}
	t := pass.TypesInfo.TypeOf(ref.Field.Type)
	if t == nil {
		return nil, false
	}
	sig, ok := t.Underlying().(*types.Signature)
	if !ok {
		return nil, false
	}
	return sig, true
}

// callbackSigHasIdentRef reports whether name matches the
// receiver, a parameter, or a named return slot of sig. Unnamed
// positions never match because the surface form of a vow:cond
// reference requires a declared identifier.
func callbackSigHasIdentRef(sig *types.Signature, name string) bool {
	if name == "" {
		return false
	}
	if recv := sig.Recv(); recv != nil && recv.Name() == name {
		return true
	}
	if params := sig.Params(); params != nil {
		for i := 0; i < params.Len(); i++ {
			if params.At(i).Name() == name {
				return true
			}
		}
	}
	if results := sig.Results(); results != nil {
		for i := 0; i < results.Len(); i++ {
			if results.At(i).Name() == name {
				return true
			}
		}
	}
	return false
}
