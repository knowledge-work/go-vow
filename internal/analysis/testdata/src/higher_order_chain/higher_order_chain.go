// Package higher_order_chain is the analysistest fixture for the
// chained `[X][Y]...` scope qualifier that retargets a marker
// through nested higher-order callbacks. Each chain segment
// resolves against the previous segment's callback signature, so
// the same declaration grammar describes a contract carried by a
// callback that itself exposes a callback parameter.
//
// The fixtures cover the four marker families that accept the
// scope qualifier — vow:cond, vow:use, vow:emit, vow:nil — with
// silent baselines whose chain resolves cleanly through nested
// callbacks paired with diagnose cases whose chain stops at a
// segment the signature lacks, at an intermediate slot that is
// not callback-typed, or at a body reference the innermost
// callback signature does not host.
//
// Chain-aware declarations defer their executable semantics to its
// own scope; the silent baselines therefore exercise the
// resolution path only, not any contract enforcement on the
// enclosing function's body.
package higher_order_chain

import "errors"

// ErrFoo is the subject the body references name when they fire
// against the innermost callback signature.
//
// vow:define @Sentinel
var ErrFoo = errors.New("foo")

// InnerFn is the innermost callback shape; the chain's deepest
// segment resolves against this signature, so body references
// match against `z` (parameter) and `$1` (the first return slot).
type InnerFn func(z error) error

// OuterFn carries an InnerFn parameter named `inner`, so the
// chain `[outer][inner]` steps from OuterFn into InnerFn through
// the named slot.
type OuterFn func(inner InnerFn) error

// DeeperFn nests another level so the chain
// `[deeper][outer][inner]` covers three resolution depths.
type DeeperFn func(outer OuterFn) error

// FactoryFn returns an InnerFn as its first return, so a chain
// with a `[$1]` mid-segment reaches the unnamed return slot
// positionally.
type FactoryFn func() (InnerFn, error)

// ----- vow:cond[X][Y] chain — silent baselines -----

// condChainSilent declares a logical-arrow rule scoped to the
// inner callback two levels deep. The body's bare identifier `z`
// resolves to inner's parameter and `$1` resolves to inner's
// first return slot, so the validator clears the rule without a
// diagnostic.
//
// vow:cond[outer][inner] z == nil <=> $1 == nil
func condChainSilent(outer OuterFn) error { _ = outer; return nil }

// condChainPositionalMidSilent uses a `[$1]` mid-segment to reach
// FactoryFn's unnamed first return slot positionally, then names
// `z` and `$1` inside the body against InnerFn's signature.
//
// vow:cond[factory][$1] z == nil <=> $1 == nil
func condChainPositionalMidSilent(factory FactoryFn) error { _ = factory; return nil }

// condDeepChainSilent reaches three nesting levels. Each segment
// resolves against the previous callback signature so the chain
// `[deeper][outer][inner]` lands at InnerFn for body validation.
//
// vow:cond[deeper][outer][inner] z == nil <=> $1 == nil
func condDeepChainSilent(deeper DeeperFn) error { _ = deeper; return nil }

// ----- vow:cond[X][Y] chain — diagnose cases -----

// condChainHeadUnresolved names a head segment the enclosing
// function's signature lacks. The walker stops at chain depth 0
// and reports the chain-aware unresolved diagnostic.
//
// vow:cond[xxx][inner] z == nil <=> $1 == nil
func condChainHeadUnresolved(outer OuterFn) error { _ = outer; return nil } // want `vow\[sentinel-error\]: vow:cond\[xxx\]\[inner\] subject "xxx" at chain depth 0 does not name a receiver, parameter, or named return of the enclosing function`

// condChainTailUnresolved names a tail segment the inner callback
// signature lacks. The walker resolves outer, steps into its
// callback signature, then fails at chain depth 1 because the
// segment does not name any slot of OuterFn.
//
// vow:cond[outer][zzz] z == nil <=> $1 == nil
func condChainTailUnresolved(outer OuterFn) error { _ = outer; return nil } // want `vow\[sentinel-error\]: vow:cond\[outer\]\[zzz\] subject "zzz" at chain depth 1 does not name a receiver, parameter, or named return of the callback signature`

// condChainIntermediateNotCallback chains through a non-callback
// intermediate slot. The walker resolves `count` against the
// enclosing signature but cannot step inward because `count` is
// `*int` rather than a function-typed slot.
//
// vow:cond[count][inner] z == nil <=> $1 == nil
func condChainIntermediateNotCallback(count *int) error { _ = count; return nil } // want `vow\[sentinel-error\]: vow:cond\[count\]\[inner\] subject "count" at chain depth 0 is not a callback; chain only steps through function-typed slots`

// condChainBodyRefUnresolved names a body reference the innermost
// callback signature lacks. The chain resolves cleanly through
// outer and inner, but `missing` does not match InnerFn's
// receiver, parameters, or named returns.
//
// vow:cond[outer][inner] missing == nil <=> $1 == nil
func condChainBodyRefUnresolved(outer OuterFn) error { _ = outer; return nil } // want `vow\[sentinel-error\]: vow:cond\[outer\]\[inner\] references "missing", which is not a parameter, receiver, or named return of the callback signature`

// condChainBodyDollarOutOfRange points the body's `$N` reference
// beyond the innermost callback's return list. InnerFn has one
// return slot, so `$5` exceeds the available positions.
//
// vow:cond[outer][inner] z == nil <=> $5 == nil
func condChainBodyDollarOutOfRange(outer OuterFn) error { _ = outer; return nil } // want `vow\[sentinel-error\]: vow:cond\[outer\]\[inner\] references \$5 but the callback signature has fewer return positions`

// ----- vow:use[X][Y] chain -----

// useChainSilent declares a caller-side discharge for ErrFoo on
// the inner callback that outer exposes. The chain resolves
// through OuterFn into InnerFn, both callback-typed, so the
// callback-type gate stays silent.
//
// vow:use[outer][inner] ErrFoo
func useChainSilent(outer OuterFn) error { _ = outer; return nil }

// useChainHeadUnresolved names a head segment the enclosing
// signature lacks. The walker stops at depth 0 and surfaces the
// chain-unresolved diagnostic.
//
// vow:use[xxx][inner] ErrFoo
func useChainHeadUnresolved(outer OuterFn) error { _ = outer; return nil } // want `vow\[sentinel-error\]: vow:use\[xxx\]\[inner\] subject "xxx" at chain depth 0 does not name a receiver, parameter, or named return of the enclosing function`

// useChainTailUnresolved names a tail segment OuterFn's signature
// lacks. The walker resolves outer but fails at depth 1 because
// the segment does not match any slot of OuterFn.
//
// vow:use[outer][zzz] ErrFoo
func useChainTailUnresolved(outer OuterFn) error { _ = outer; return nil } // want `vow\[sentinel-error\]: vow:use\[outer\]\[zzz\] subject "zzz" at chain depth 1 does not name a receiver, parameter, or named return of the callback signature`

// useChainIntermediateNotCallback chains through a non-callback
// intermediate slot. The walker resolves `count` but cannot step
// inward because `count` is `*int` rather than function-typed.
//
// vow:use[count][inner] ErrFoo
func useChainIntermediateNotCallback(count *int) error { _ = count; return nil } // want `vow\[sentinel-error\]: vow:use\[count\]\[inner\] subject "count" at chain depth 0 is not a callback; chain only steps through function-typed slots`

// ----- vow:emit[X][Y] chain -----

// emitChainSilent declares that the inner callback emits Close.
// The chain resolves through OuterFn into InnerFn, both
// callback-typed.
//
// vow:emit[outer][inner] Close
func emitChainSilent(outer OuterFn) error { _ = outer; return nil }

// emitChainHeadUnresolved names a head segment the enclosing
// signature lacks.
//
// vow:emit[xxx][inner] Close
func emitChainHeadUnresolved(outer OuterFn) error { _ = outer; return nil } // want `vow\[sentinel-error\]: vow:emit\[xxx\]\[inner\] subject "xxx" at chain depth 0 does not name a receiver, parameter, or named return of the enclosing function`

// emitChainTailUnresolved names a tail segment OuterFn's
// signature lacks. The walker resolves outer but fails at depth 1
// because the segment does not match any slot of OuterFn.
//
// vow:emit[outer][zzz] Close
func emitChainTailUnresolved(outer OuterFn) error { _ = outer; return nil } // want `vow\[sentinel-error\]: vow:emit\[outer\]\[zzz\] subject "zzz" at chain depth 1 does not name a receiver, parameter, or named return of the callback signature`

// emitChainIntermediateNotCallback chains through a non-callback
// intermediate slot.
//
// vow:emit[count][inner] Close
func emitChainIntermediateNotCallback(count *int) error { _ = count; return nil } // want `vow\[sentinel-error\]: vow:emit\[count\]\[inner\] subject "count" at chain depth 0 is not a callback; chain only steps through function-typed slots`

// emitLineChainSilent anchors a statement-scope vow:emit marker
// at a chain. The marker resolves silently because the chain
// reaches a callback-typed innermost slot.
func emitLineChainSilent(outer OuterFn) error {
	_ = outer
	// vow:emit[outer][inner] Close
	return nil
}

// emitLineChainHeadUnresolved anchors a statement-scope chain
// whose head segment does not resolve. The caller-side
// discovery reports the chain-unresolved failure under
// `vow[closable]`.
func emitLineChainHeadUnresolved(outer OuterFn) error {
	_ = outer
	// vow:emit[xxx][inner] Close // want `vow\[closable\]: vow:emit\[xxx\]\[inner\] subject "xxx" at chain depth 0 does not name a receiver, parameter, or named return of the enclosing function`
	return nil
}

// emitLineChainIntermediateNotCallback anchors a statement-scope
// chain whose intermediate slot is not callback-typed. The
// chain-step diagnostic surfaces through the
// validateStatementScopeEmitSubjects walker under
// `vow[sentinel-error]`, so the discovery path stays focused on
// the unresolved-chain surface under `vow[closable]`.
func emitLineChainIntermediateNotCallback(count *int) error {
	_ = count
	// vow:emit[count][inner] Close // want `vow\[sentinel-error\]: vow:emit\[count\]\[inner\] subject "count" at chain depth 0 is not a callback; chain only steps through function-typed slots`
	return nil
}

// ----- vow:nil[X][Y] chain -----

// nilChainSilent declares the inner callback's parameter as non-
// nil through the chain. The chain resolves and the contract's
// arity matches InnerFn's signature (one parameter, the `z`
// slot).
//
// vow:nil[outer][inner] () ?
func nilChainSilent(outer OuterFn) error { _ = outer; return nil }

// nilChainHeadUnresolved names a head segment the enclosing
// signature lacks.
//
// vow:nil[xxx][inner] (!)
func nilChainHeadUnresolved(outer OuterFn) error { _ = outer; return nil } // want `vow\[nil-safety\]: vow:nil\[xxx\]\[inner\] subject "xxx" at chain depth 0 does not name a receiver, parameter, or named return of the enclosing function`

// nilChainTailUnresolved names a tail segment OuterFn's signature
// lacks. The walker resolves outer but fails at depth 1.
//
// vow:nil[outer][zzz] (!)
func nilChainTailUnresolved(outer OuterFn) error { _ = outer; return nil } // want `vow\[nil-safety\]: vow:nil\[outer\]\[zzz\] subject "zzz" at chain depth 1 does not name a receiver, parameter, or named return of the callback signature`

// nilChainIntermediateNotCallback chains through a non-callback
// intermediate slot.
//
// vow:nil[count][inner] (!)
func nilChainIntermediateNotCallback(count *int) error { _ = count; return nil } // want `vow\[nil-safety\]: vow:nil\[count\]\[inner\] subject "count" at chain depth 0 is not a callback; chain only steps through function-typed slots`

// nilChainArityOverflow declares two parameter positions but
// InnerFn carries one parameter. The arity gate quotes the chain
// in the `[X][Y]...` surface form.
//
// vow:nil[outer][inner] (!, !) ?
func nilChainArityOverflow(outer OuterFn) error { _ = outer; return nil } // want `vow\[nil-safety\]: vow:nil\[outer\]\[inner\] declares 2 parameter positions but the callback signature has 1`

// nilChainReturnUnified declares no return positions while InnerFn
// returns one. The chain side carries the same unification, so the
// arity gate stays silent on both axes here.
//
// vow:nil[outer][inner] (!)
func nilChainReturnUnified(outer OuterFn) error { _ = outer; return nil }
