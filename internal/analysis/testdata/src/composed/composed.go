// Package composed exercises the @rule composition surface. The
// package-doc `vow:import` line below brings the std/result preset's
// rule namespace into scope under the alias `result`, so functions
// in this package can write `@result.OkErr[...]` and
// `@result.Either[...]` as the atom-rule of a `vow:cond`
// (the surface for "this function's signature matches the referenced
// rule's expansion").
//
// vow:import result "preset/std/result"
package composed

import "errors"

// vow:define @Sentinel
var OkMarker = errors.New("ok")

// vow:define @Sentinel
var ErrNotReady = errors.New("not ready")

// okErrOK returns either (OkMarker, nil) or (nil, ErrNotReady); both
// alternatives match a tuple of the expanded OkErr rule. No
// diagnostic.
//
// vow:cond @result.OkErr[OkMarker, ErrNotReady]
func okErrOK(ready bool) (error, error) {
	if ready {
		return OkMarker, nil
	}
	return nil, ErrNotReady
}

// okErrLeak returns (nil, errors.New("other")). The second position
// is a CallExpr the analyzer cannot resolve syntactically, so the
// return matches neither declared tuple and the tuple-sum checker
// fires. The must-consume rule stays silent because errors.New(...)
// is not a subject reference.
//
// vow:cond @result.OkErr[OkMarker, ErrNotReady]
func okErrLeak() (error, error) {
	return nil, errors.New("other") // want `vow\[sentinel-error\]: return values do not match any tuple of the declared sum`
}

// eitherOK exercises the Either rule. Both branches match one of the
// declared alternatives in the expanded position-wise sum.
//
// vow:cond @result.Either[OkMarker, ErrNotReady]
func eitherOK(ok bool) error {
	if ok {
		return OkMarker
	}
	return ErrNotReady
}

// ErrUnexpected is a third subject that the Either rule below does
// not list, used to exercise the negative case for a position-wise
// sum after rule expansion.
//
// vow:define @Sentinel
var ErrUnexpected = errors.New("unexpected")

// eitherLeak returns ErrUnexpected, which is not listed in the
// expanded Either sum. Two diagnostics fire on the same identifier:
//
//   - the vow:cond checker reports the sum-membership violation.
//   - the must-consume rule reports a leak because the annotation
//     does not authorize ErrUnexpected.
//
// vow:cond @result.Either[OkMarker, ErrNotReady]
func eitherLeak() error {
	return ErrUnexpected // want `vow\[sentinel-error\]: return position 0: returning ErrUnexpected but the position only accepts nonzero OkMarker \| nonzero ErrNotReady` `vow\[sentinel-error\]: sentinel error ErrUnexpected leaked: needs observation or explicit propagation`
}

// closureChain stores a function literal in a package-level var.
// Function literals cannot carry their own doc, and the analyzer no
// longer reaches outward to the enclosing var's doc for chain
// authorisation — the FuncLit's return is therefore a leak. Authors
// who need the same effect must wrap the literal in a named
// FuncDecl carrying an explicit vow:cond signature.
var closureChain = func() error {
	return OkMarker // want `vow\[sentinel-error\]: sentinel error OkMarker leaked: needs observation or explicit propagation`
}
