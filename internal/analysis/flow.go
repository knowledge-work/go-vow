package analysis

import (
	"go/ast"
	"go/token"
	"slices"

	"golang.org/x/tools/go/analysis"
)

// resolveLocalAlias walks the body of fn and reports the right-hand
// side expression bound to the given identifier name by a `:=` define
// statement. The result is non-nil only when the function body
// contains exactly one short-form definition for the name — multiple
// definitions, reassignments, or branch-merged forms produce nil so
// the caller treats the alias as unresolved (conservative).
//
// This is the intra-procedural data-flow primitive backing the
// AST-based subject resolver. SSA would be more precise (it would
// handle branch merges and phi nodes), but the AST walk is
// sufficient for the canonical `x := ErrFoo; return x` shape and
// keeps this resolver free of the buildssa dependency. The SSA pass
// covers the remaining shapes separately.
func resolveLocalAlias(fn *ast.FuncDecl, ident *ast.Ident) ast.Expr {
	if fn == nil || fn.Body == nil || ident == nil {
		return nil
	}
	var found ast.Expr
	matches := 0
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || as.Tok != token.DEFINE {
			return true
		}
		for i, lhs := range as.Lhs {
			lhsIdent, ok := lhs.(*ast.Ident)
			if !ok || lhsIdent.Name != ident.Name {
				continue
			}
			if i >= len(as.Rhs) {
				continue
			}
			matches++
			found = as.Rhs[i]
		}
		return true
	})
	if matches != 1 {
		return nil
	}
	return found
}

// isAliasBindingRHS reports whether the stack ends with an identifier
// that sits as a right-hand side of a short-form (`:=`) assignment.
// Such identifiers are bookkeeping for the local alias resolver — the
// classifier should not treat them as independent uses of the subject.
func isAliasBindingRHS(stack []ast.Node) bool {
	if len(stack) < 2 {
		return false
	}
	parent := stack[len(stack)-2]
	as, ok := parent.(*ast.AssignStmt)
	if !ok || as.Tok != token.DEFINE {
		return false
	}
	ident := stack[len(stack)-1]
	return slices.ContainsFunc(as.Rhs, func(rhs ast.Expr) bool {
		return rhs == ident
	})
}

// enclosingFuncDecl extracts the *ast.FuncDecl that owns the given
// inspector stack, or nil when the stack root is a different kind of
// declaration (e.g. a package-level var initializer running inside an
// init body).
func enclosingFuncDecl(stack []ast.Node) *ast.FuncDecl {
	for _, n := range stack {
		if fn, ok := n.(*ast.FuncDecl); ok {
			return fn
		}
	}
	return nil
}

// resolveSubject decides whether ident refers (directly or via a
// single-define alias) to one of the package's tracked subjects. It
// returns the subject's display name and true on success.
//
// The stack is consulted only to find the enclosing FuncDecl. Callers
// that already have the FuncDecl on hand (e.g. the SSA leak detector
// in flow_ssa.go) should invoke resolveSubjectInFunc directly to skip
// the walk-out step.
func resolveSubject(pass *analysis.Pass, stack []ast.Node, ident *ast.Ident, subjects subjectSet) (string, bool) {
	fn := enclosingFuncDecl(stack)
	if fn == nil {
		return resolveSubjectInFunc(pass, nil, ident, subjects)
	}
	return resolveSubjectInFunc(pass, fn, ident, subjects)
}

// resolveSubjectInFunc is the stack-independent core shared by the
// AST per-Ident walk (via resolveSubject) and the SSA-driven leak
// detector. Direct references win immediately; otherwise the function
// follows one short-form alias hop within fn and reports the alias's
// RHS subject. fn may be nil — the alias hop is then skipped and only
// the direct check applies. Only one hop is followed so
// `y := x; x := ErrFoo; return y` stays unresolved at the syntactic
// level (the SSA pass picks up the rest).
func resolveSubjectInFunc(pass *analysis.Pass, fn *ast.FuncDecl, ident *ast.Ident, subjects subjectSet) (string, bool) {
	if subjects.contains(pass.TypesInfo.ObjectOf(ident)) {
		return ident.Name, true
	}
	if fn == nil {
		return "", false
	}
	rhs := resolveLocalAlias(fn, ident)
	if rhs == nil {
		return "", false
	}
	rhsIdent, ok := rhs.(*ast.Ident)
	if !ok {
		return "", false
	}
	if !subjects.contains(pass.TypesInfo.ObjectOf(rhsIdent)) {
		return "", false
	}
	return rhsIdent.Name, true
}
