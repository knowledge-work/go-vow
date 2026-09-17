package analysis

import (
	"go/ast"
	"go/constant"
	"go/types"
	"math"

	"golang.org/x/tools/go/analysis"
)

// zeroState classifies how confident the analyzer is about a return
// expression being the zero value of its type.
type zeroState int

const (
	// zeroUnknown — the expression's value cannot be resolved at
	// analysis time (e.g. a local variable, a function call). The
	// caller should not diagnose; deeper data-flow analysis may
	// widen this in the future.
	zeroUnknown zeroState = iota
	// zeroNo — the expression resolves to a non-zero constant.
	zeroNo
	// zeroYes — the expression is provably the zero value.
	zeroYes
)

// classifyZero reports the zero state of expr using three layers of
// evidence:
//
//  1. `pass.TypesInfo.Types[expr].Value` for constant folding of
//     primitives, named-primitive const refs, and similar constant
//     expressions.
//  2. The bare `nil` identifier — Go's zero value for interfaces
//     (notably `error`), pointers, slices, maps, channels, and
//     function values. The Go parser exposes this as `*ast.Ident`
//     with name "nil"; it is never a constant in `types.Info`.
//  3. Composite literals — both the empty form `T{}` and partial
//     forms `T{Field: zero}` whose every element evaluates to zero
//     are treated as zero. The walk pulls KeyValueExpr values out of
//     the way and recurses via classifyZero, so nested composites
//     are inspected to arbitrary depth.
func classifyZero(pass *analysis.Pass, expr ast.Expr) zeroState {
	if tv, ok := pass.TypesInfo.Types[expr]; ok && tv.Value != nil {
		if isConstantZero(tv.Value) {
			return zeroYes
		}
		return zeroNo
	}
	if ident, ok := expr.(*ast.Ident); ok && ident.Name == "nil" {
		return zeroYes
	}
	if lit, ok := expr.(*ast.CompositeLit); ok {
		return classifyCompositeLitZero(pass, lit)
	}
	return zeroUnknown
}

// classifyCompositeLitZero reports whether a composite literal
// represents the zero value of its type.
//
// Only struct and array composites can be "zero" — for slices and
// maps the zero value is nil, and a literal like `[]int{}` or
// `map[string]int{}` produces a *non-nil empty* value that is
// distinct from the type's zero. Restricting the rule keeps
// authors of slice-typed sentinels from being told that their
// deliberate "empty but allocated" return is zero.
//
// Within struct/array composites: `T{}` is trivially zero; the
// partial form `T{Field: v}` is zero only when every listed element
// evaluates to zero. An element whose zero state is anything other
// than zeroYes (including zeroUnknown for variable references)
// short-circuits the composite to non-zero.
func classifyCompositeLitZero(pass *analysis.Pass, lit *ast.CompositeLit) zeroState {
	t := pass.TypesInfo.TypeOf(lit)
	if t == nil {
		return zeroUnknown
	}
	switch t.Underlying().(type) {
	case *types.Struct, *types.Array:
		// fall through
	default:
		return zeroNo
	}
	if len(lit.Elts) == 0 {
		return zeroYes
	}
	for _, elt := range lit.Elts {
		value := elt
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			value = kv.Value
		}
		if classifyZero(pass, value) != zeroYes {
			return zeroNo
		}
	}
	return zeroYes
}

// isConstantZero reports whether v represents the zero value of its
// underlying kind.
//
// The policy per kind:
//
//   - Int / Uint: only the literal 0 is zero. Values outside int64
//     range are treated as non-zero (the constant package's exact
//     flag becomes false).
//   - String / Bool: the empty string and false are zero.
//   - Float: +0.0 and -0.0 are zero. NaN and ±Inf are non-zero by
//     policy — there is no plausible obligation that treats a NaN as
//     "the zero float", and the resulting diagnostic would only
//     confuse the author.
//   - Complex: 0+0i is zero. NaN or ±Inf in either component is
//     non-zero, matching the float policy component-wise.
//
// Unsupported kinds (e.g. constant.Unknown) fall through to non-zero
// so callers consult other evidence rather than tripping a false
// positive.
func isConstantZero(v constant.Value) bool {
	switch v.Kind() {
	case constant.Int:
		if i, exact := constant.Int64Val(v); exact {
			return i == 0
		}
		return false
	case constant.String:
		return constant.StringVal(v) == ""
	case constant.Bool:
		return !constant.BoolVal(v)
	case constant.Float:
		return isFloatZero(v)
	case constant.Complex:
		return isFloatZero(constant.Real(v)) && isFloatZero(constant.Imag(v))
	}
	return false
}

// isFloatZero reports whether a constant.Float (or one component of a
// complex) represents the zero of its kind. NaN and ±Inf are excluded
// explicitly because the strict `f == 0` comparison happens to do the
// right thing, but the explicit guards document the policy and
// guard against future Go releases changing constant.Float64Val.
func isFloatZero(v constant.Value) bool {
	f, _ := constant.Float64Val(v)
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return false
	}
	return f == 0
}
