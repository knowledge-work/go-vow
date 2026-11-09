package analysis

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// validateLogicalCondReferences reports a logical-arrow operand of an
// unscoped vow:cond that names no parameter, receiver, or named
// return of the function, or a `$N` past its last return. Such an
// operand binds to nothing, so the rule never decides and stays
// silent at every return and call site. Subject-scoped conditions
// are checked against their callback by validateSubjectScopedCond.
func validateLogicalCondReferences(pass *analysis.Pass, state *passState) {
	for _, file := range pass.Files {
		scope := state.ruleScope(pass)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			obj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			if !ok {
				continue
			}
			conds, ok := state.annotation(fn, scope)
			if !ok {
				continue
			}
			for _, rule := range collectLogicalRules(conds) {
				validateLogicalOperandInSig(pass, fn, nil, obj.Signature(), rule.Left)
				validateLogicalOperandInSig(pass, fn, nil, obj.Signature(), rule.Right)
			}
		}
	}
}
