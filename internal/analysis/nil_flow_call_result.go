package analysis

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// callResultNilness reads the nullness a callee declares at its single
// return slot. An undeclared callee stays Unknown: absence of a
// declaration is not a claim that the result may be nil.
//
// Multi-result callees are skipped because the surrounding checks do not
// map a spread to its slots, so a nullness here would name the wrong one.
//
// Never returns KnownNil: forwardWalkNilness acts on KnownNil and NonNil
// only, so stopping at Nillable is what leaves the vow:cond axis unmoved.
func callResultNilness(pass *analysis.Pass, state *passState, call *ast.CallExpr) nilness {
	callee := calleeObject(pass, call)
	if callee == nil {
		return nilnessUnknown
	}
	if !calleeHasSingleResult(callee) {
		return nilnessUnknown
	}
	sig := nilDeclForFunc(pass, state, callee)
	if sig == nil || len(sig.Returns) != 1 {
		return nilnessUnknown
	}
	decl := sig.Returns[0]
	if decl == nil || decl.Nullness != dsl.NullnessNillable {
		return nilnessUnknown
	}
	return nilnessNillable
}

// calleeHasSingleResult reports whether callee returns exactly one value.
func calleeHasSingleResult(callee types.Object) bool {
	fn, ok := callee.(*types.Func)
	if !ok {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return false
	}
	return sig.Results().Len() == 1
}
