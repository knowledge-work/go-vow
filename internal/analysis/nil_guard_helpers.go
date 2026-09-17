package analysis

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// indexFuncDecls returns a map from each same-package function's
// type-checker Object to its *ast.FuncDecl. Building the index once
// at the top of a pass keeps every per-call lookup O(1) — the
// alternative (scanning pass.Files inside the CallExpr walk) was
// quadratic in (decls × call-sites). Used by the receiver-side
// caller check, which still needs the AST-level annotation lookup
// to dispatch the silent branch on a vow:cond-only callee.
func indexFuncDecls(pass *analysis.Pass) map[types.Object]*ast.FuncDecl {
	out := map[types.Object]*ast.FuncDecl{}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			obj := pass.TypesInfo.Defs[fn.Name]
			if obj == nil {
				continue
			}
			out[obj] = fn
		}
	}
	return out
}

// leadingIfStatements returns the if statements that appear at the
// top of body before any non-if statement. Only the contiguous
// leading run is considered: a guard buried after a non-guard
// statement does not qualify as a body-leading shape and is
// left to the SSA-based flow pass. Shared by every body-leading
// guard recogniser the analyzer runs — the vow:nil parameter
// and receiver dead-guard passes, and the vow:cond prereq
// dead-guard pass — so an author who reads any of those
// diagnostics recognises the same syntactic anchor.
func leadingIfStatements(body *ast.BlockStmt) []*ast.IfStmt {
	if body == nil {
		return nil
	}
	var out []*ast.IfStmt
	for _, stmt := range body.List {
		ifs, ok := stmt.(*ast.IfStmt)
		if !ok {
			break
		}
		out = append(out, ifs)
	}
	return out
}

// ifCondIsNilCheckOnParam reports the parameter name when cond is
// `x == nil` (or `nil == x`) for some x in candidates. Other
// shapes (negated, typed-nil conversion, compound expressions)
// return false so the simpler form stays the only recognised one
// and the false-positive risk on dead-guard reporting stays low.
func ifCondIsNilCheckOnParam(cond ast.Expr, candidates map[string]struct{}) (string, bool) {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok || bin.Op.String() != "==" {
		return "", false
	}
	if name, ok := nameIfBareIdent(bin.X); ok {
		if _, isCandidate := candidates[name]; isCandidate && isBareNilIdent(bin.Y) {
			return name, true
		}
	}
	if name, ok := nameIfBareIdent(bin.Y); ok {
		if _, isCandidate := candidates[name]; isCandidate && isBareNilIdent(bin.X) {
			return name, true
		}
	}
	return "", false
}

// nameIfBareIdent returns the identifier name when expr is a bare
// identifier (paren-wrapping is stripped). Used by the guard
// recogniser to extract the parameter name from a comparison's
// operand.
func nameIfBareIdent(expr ast.Expr) (string, bool) {
	ident, ok := ast.Unparen(expr).(*ast.Ident)
	if !ok {
		return "", false
	}
	return ident.Name, true
}

// branchShortCircuits reports whether body is a short-circuit
// guard body — a block whose last statement transfers control
// before reaching the next statement. A bare return statement
// or a top-level call to the builtin panic qualifies; richer
// recovery shapes (logging then continuing, conditional reassign)
// do not because they would change the guard's semantics from
// "the value never flows past here" to "the value flows past
// here under some path".
func branchShortCircuits(body *ast.BlockStmt) bool {
	if body == nil || len(body.List) == 0 {
		return false
	}
	last := body.List[len(body.List)-1]
	switch v := last.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.ExprStmt:
		call, ok := v.X.(*ast.CallExpr)
		if !ok {
			return false
		}
		ident, ok := call.Fun.(*ast.Ident)
		return ok && ident.Name == "panic"
	}
	return false
}
