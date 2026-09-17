package sentinels

import "errors"

// This file pins the analyzer's behaviour on generic (Go 1.18+)
// FuncDecls so a future go/ssa release that changes the Syntax
// pointer association for generic origins versus their
// instantiations cannot silently break the SSA leak detector. Each
// diagnostic below is expected from a specific code path; the pin
// is the diagnostic itself, not the implementation route.

// vow:define @Sentinel
var ErrGenericFlow = errors.New("generic-flow")

// genericMultiHopLeak exercises findSSAFunction against a generic
// origin. The body matches the canonical SSA-detected multi-hop
// chain (`x := sentinel; y := x; return y`) so any SSA leak
// detection failure inside the generic surfaces at the return —
// Syntax pointer mismatches between FuncDecl and *ssa.Function
// would silently swallow this diagnostic.
func genericMultiHopLeak[T any](_ T) error {
	x := ErrGenericFlow
	y := x
	return y // want `vow\[sentinel-error\]: sentinel error ErrGenericFlow leaked: needs observation or explicit propagation`
}

// instantiateGenericLeak forces the compiler to monomorphise
// genericMultiHopLeak at least once. The instantiation's SSA
// Function shares its Syntax pointer with the generic origin under
// the buildssa default mode; this call site keeps a regression
// gate on that assumption — if generics ever start producing
// independent Syntax pointers per instantiation, the leak inside
// the generic body should still be reported exactly once (at the
// origin's return), not duplicated per instantiation.
//
// The return expression here is a CallExpr, not an Ident, so the
// SSA pass does not analyse this site; only the per-Ident walk
// might fire on the argument, which is a non-subject literal.
func instantiateGenericLeak() error {
	return genericMultiHopLeak[int](0)
}

// genericChainAuthorized pairs the regression gate with the chain
// authorisation integration in the SSA leak detector. The generic
// origin lists ErrGenericFlow in its vow:cond signature so the
// SSA leak inside the body must stay silent, demonstrating that
// doc-comment-based authorisation reaches the generic origin's
// FuncDecl. A regression where the chain check resolves to the
// instantiation's FuncDecl (or to nothing at all) would cause this
// case to leak again.
//
// vow:cond * -> ErrGenericFlow | nil
func genericChainAuthorized[T any](_ T) error {
	x := ErrGenericFlow
	y := x
	return y
}

// instantiateGenericAuthorized is the symmetric instantiation site
// for genericChainAuthorized. It exists so the generic origin is
// not dead code at the package level and the SSA program reliably
// monomorphises it.
func instantiateGenericAuthorized() error {
	return genericChainAuthorized[int](0)
}

// genericMethodBox is the receiver for the generic-method fixtures. It
// carries a type parameter of its own so that the method below declares a
// second one, which is the shape a method could not have before Go 1.27.
type genericMethodBox[T any] struct{}

// genericMethodMultiHopLeak repeats genericMultiHopLeak on a method that
// declares its own type parameters. findSSAFunction pairs an
// *ast.FuncDecl with its *ssa.Function by Syntax pointer, and a method
// with type parameters reaches that pairing by a different route than a
// generic function does; a mismatch on that route would drop the SSA pass
// for the body without any diagnostic saying so.
//
// The multi-hop chain is the only shape here because everything below the
// pairing — phi resolution, alias walking — is shared with non-generic
// bodies, which branchMergeLeak and multiHopLeak already pin. A
// generic-specific branch added below the pairing would need its own
// shape here.
func (genericMethodBox[T]) genericMethodMultiHopLeak[U any](_ U) error {
	x := ErrGenericFlow
	y := x
	return y // want `vow\[sentinel-error\]: sentinel error ErrGenericFlow leaked: needs observation or explicit propagation`
}

// instantiateGenericMethodLeak monomorphises both the receiver and the
// method so the origin is not dead code at the package level.
func instantiateGenericMethodLeak() error {
	return genericMethodBox[int]{}.genericMethodMultiHopLeak(0)
}
