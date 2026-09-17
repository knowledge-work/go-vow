package analysis

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// validateSubjectScopedUseHints walks every `vow:use[subject]`
// marker — both the function-doc form attached to a function's
// declaration and the line-scope form that anchors to a statement
// inside a body — and reports when the resolved subject is not
// function-typed. A vow:use scoped assertion describes a
// caller-side discharge that propagates through a callback's
// invocation, so a non-function slot cannot host the contract;
// the diagnostic anchors at the marker's declaration line so the
// author sees the mismatch alongside the offending source.
func validateSubjectScopedUseHints(pass *analysis.Pass, state *passState) {
	_ = state
	for _, file := range pass.Files {
		validateUseHintFuncDocSubjects(pass, file)
		validateUseHintLineScopeSubjects(pass, file)
	}
}

// validateUseHintFuncDocSubjects walks function-doc vow:use
// markers and surfaces a diagnostic on the function declaration
// when a [subject] qualifier resolves to a non-callback signature
// slot. Markers with no subject, with an unresolvable subject, or
// with no subjects in the payload are handled upstream so this
// walker stays focused on the callback-type check.
func validateUseHintFuncDocSubjects(pass *analysis.Pass, file *ast.File) {
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
			scope, subjects, present := parseUseHintLine(text)
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
				reportChainStepNotCallback(pass, fn.Pos(), useHintMarker, scope, failedAt)
				continue
			}
			if sig != nil {
				continue
			}
			reported[key] = struct{}{}
			reportSubjectChainNotCallback(pass, fn.Pos(), useHintMarker, scope,
				"discharge only propagates through function-typed slots")
		}
	}
}

// validateUseHintLineScopeSubjects walks line-scope vow:use
// markers — comments that sit inside a function body rather than
// in the function's doc — and surfaces a diagnostic when the
// scope qualifier resolves to a non-callback slot of the
// enclosing function's signature. The marker's anchor follows the
// existing line-scope resolution path; this walker layers the
// callback-type gate over it.
func validateUseHintLineScopeSubjects(pass *analysis.Pass, file *ast.File) {
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
			scope, subjects, present := parseUseHintLine(text)
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
				reportChainStepNotCallback(pass, c.Pos(), useHintMarker, scope, failedAt)
				continue
			}
			if sig != nil {
				continue
			}
			reportSubjectChainNotCallback(pass, c.Pos(), useHintMarker, scope,
				"discharge only propagates through function-typed slots")
		}
	}
}
