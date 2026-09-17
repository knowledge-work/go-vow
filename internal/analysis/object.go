package analysis

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// resolveExprObject returns the types.Object that expr refers to,
// after transparently unwrapping any author-written parens. The
// resolution rule is intentionally narrow:
//
//   - *ast.Ident         → pass.TypesInfo.ObjectOf(ident)
//   - *ast.SelectorExpr  → pass.TypesInfo.ObjectOf(sel.Sel)
//   - anything else      → nil
//
// Callers that wanted method values, function literals, calls
// through interface methods, or other AST shapes never received an
// Object from the two former helpers (callFuncObject and
// returnSubjectObject), and the analyzer downstream of both knew to
// treat a nil Object as "unresolved, skip this site". Keeping the
// same narrowness here preserves that contract while consolidating
// the two helpers into one place: the observer machinery feeds in
// `call.Fun` (parens around a function call target are legal Go,
// e.g. `(foo)()`), and the return-position matchers feed in a
// return expression where `(ErrFoo)` is idiomatic. Both want the
// same shape-aware name-to-object resolution, so a future widening
// (method values, instantiated generic functions, etc.) lands in a
// single site.
func resolveExprObject(pass *analysis.Pass, expr ast.Expr) types.Object {
	for {
		p, ok := expr.(*ast.ParenExpr)
		if !ok {
			break
		}
		expr = p.X
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return pass.TypesInfo.ObjectOf(e)
	case *ast.SelectorExpr:
		return pass.TypesInfo.ObjectOf(e.Sel)
	}
	return nil
}
