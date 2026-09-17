// Package nilDeclBasic is the analysistest fixture for the
// signature-mirror `vow:nil` marker at its first surface: top-
// level nullness tokens (`!` and `?`) without nest grammar. The
// caller-side argument check reads the parsed signature and
// reports diagnostics for statically-nil arguments at positions
// the declaration pins as non-nil; nillable positions stay silent
// at the call site.
package nilDeclBasic

// Request is the argument shape used by the demo callees.
type Request struct{}

// allNonNil pins every regular parameter as non-nil through a
// signature-mirror declaration with `!` at each slot. The receiver
// is omitted from the payload because the function is a free
// function; the empty receiver position is the only legal way to
// spell "no receiver" in the grammar.
//
// vow:nil (!,!)
func allNonNil(p *int, q *Request) {} // want allNonNil:"nilDecl\\(!,!\\)"

// firstNonNil pins only the first parameter as non-nil. The
// second parameter is left at platform (empty slot), so vow stays
// silent on it at the call site.
//
// vow:nil (!,)
func firstNonNil(p *int, q *Request) {} // want firstNonNil:"nilDecl\\(!,\\)"

// nillableOK marks the first parameter as nillable. The caller-
// side check stays silent on nillable positions because the
// contract explicitly admits a nil argument.
//
// vow:nil (?)
func nillableOK(p *int) {} // want nillableOK:"nilDecl\\(\\?\\)"

// okCalls pins the silent paths: every argument is an explicit
// address-of (non-nil by construction) or sits in a position the
// callee leaves unconstrained.
func okCalls() {
	a := 1
	b := Request{}
	allNonNil(&a, &b)
	firstNonNil(&a, nil)
	nillableOK(nil)
}

// nilLiteralArgs exercises the literal-nil proof path against the
// new signature-mirror declaration. Each call supplies a literal
// nil at a position the callee pins as `!`, so the analyzer must
// surface a diagnostic on the argument token.
func nilLiteralArgs() {
	allNonNil(nil, nil)   // want `vow\[nil-safety\]: argument 1 \(p\) is nil` `vow\[nil-safety\]: argument 2 \(q\) is nil`
	firstNonNil(nil, nil) // want `vow\[nil-safety\]: argument 1 \(p\) is nil`
}

// typedNilArgs pins the typed-nil-conversion proof path on the
// same signature-mirror surface. The contract violation is
// identical regardless of whether the right-hand side is the
// bare literal or a typed conversion.
func typedNilArgs() {
	allNonNil((*int)(nil), (*Request)(nil)) // want `vow\[nil-safety\]: argument 1 \(p\) is nil` `vow\[nil-safety\]: argument 2 \(q\) is nil`
}

// arityOver declares one more parameter slot than the Go
// signature exposes. The analyzer surfaces a diagnostic at the
// function declaration so the author notices the typo.
//
// vow:nil (!,!)
func arityOver(p *int) {} // want arityOver:"nilDecl\\(!,!\\)" `vow\[nil-safety\]: vow:nil declares 2 parameter positions but the signature has 1`

// arityUnder declares fewer parameter slots than the Go
// signature exposes. Marker tokens are optional per slot, but
// the slot count itself must mirror the signature so the trailing
// positions cannot be silently dropped.
//
// vow:nil (!)
func arityUnder(p, q *int) {} // want arityUnder:"nilDecl\\(!\\)" `vow\[nil-safety\]: vow:nil declares 1 parameter positions but the signature has 2`

// arityReturnUnder declares fewer return slots than the Go
// signature exposes.
var arityReturnUnderCell int

// vow:nil () !
func arityReturnUnder() (*int, *int) { return &arityReturnUnderCell, &arityReturnUnderCell } // want arityReturnUnder:"nilDecl\\(\\) !" `vow\[nil-safety\]: vow:nil declares 1 return positions but the signature has 2`

// arityUnifiedZeroForOneParam pins the empty-parens exception:
// `()` is the unified canonical form for both a 0-param and a
// 1-param signature. The grammar has no surface form for a lone
// empty (platform) slot, so a marker-less contract on a 1-param
// signature has no other way to spell itself; the parameter-axis
// arity gate stays silent while the return contract still applies.
//
// vow:nil () !
func arityUnifiedZeroForOneParam(p *int) *int { return p } // want arityUnifiedZeroForOneParam:"nilDecl\\(\\) !"

// arityUnifiedZeroForOneReturn pins the same unification on the
// return axis. This is what lets a parameter contract sit on a
// function whose single return is a non-pointer, a position no
// return token could describe truthfully.
//
// vow:nil (!)
func arityUnifiedZeroForOneReturn(p *int) string { _ = p; return "" } // want arityUnifiedZeroForOneReturn:"nilDecl\\(!\\)"

// arityZeroDeclForTwoReturns pins where the unification stops: two
// returns still need the separator that carves the slots out.
//
// vow:nil (!)
func arityZeroDeclForTwoReturns(p *int) (*int, *int) { // want arityZeroDeclForTwoReturns:"nilDecl\\(!\\)" `vow\[nil-safety\]: vow:nil declares 0 return positions but the signature has 2`
	return p, p
}

// arityUnifiedReturnCallers pins that the parameter contract still
// binds on both shapes: the contract becomes writable, not inert.
func arityUnifiedReturnCallers() {
	_ = arityUnifiedZeroForOneReturn(nil)  // want `vow\[nil-safety\]: argument 1 \(p\) is nil`
	_, _ = arityZeroDeclForTwoReturns(nil) // want `vow\[nil-safety\]: argument 1 \(p\) is nil`
}

// duplicateMarker carries two vow:nil lines. The first parsed
// signature wins; the second emits a duplicate-marker
// diagnostic so the author resolves the redundancy.
//
// vow:nil (!)
// vow:nil (?)
func duplicateMarker(p *int) {} // want duplicateMarker:"nilDecl\\(!\\)" `vow\[nil-safety\]: function carries more than one vow:nil marker`

// malformedDecl carries an unrecognised token in a position
// slot. The parser rejects the payload and the diagnostic
// anchors at the function declaration.
//
// vow:nil (xyz)
func malformedDecl(p *int) {} // want `vow\[nil-safety\]: vow:nil: vow:nil .*: param 0: expected .*got "xyz"`
