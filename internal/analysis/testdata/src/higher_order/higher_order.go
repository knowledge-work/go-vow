// Package higher_order is the analysistest fixture for the
// [subject] scope qualifier that every marker family — vow:nil,
// vow:cond, vow:use, vow:emit — accepts on the function-doc
// surface. The qualifier retargets the marker at a signature-scope
// identifier (a receiver, a regular parameter, or a named return)
// so the same declaration grammar describes a higher-order
// contract carried by a callback parameter or by a return value.
//
// Each marker family gets two fixtures: a silent baseline whose
// subject resolves cleanly against the enclosing signature and a
// diagnose case whose subject does not name any slot in the
// signature. The diagnose case anchors its expected diagnostic on
// the function declaration line through the analysistest
// regex-pinning convention so the harness matches the resolver's
// failure output.
//
// Subject-scoped declarations defer their executable semantics to
// its own scope; the silent baselines therefore exercise the
// resolution path only, not any contract enforcement on the
// enclosing function's body.
package higher_order

import "errors"

// ErrFoo is the subject the vow:use and vow:emit fixtures name.
//
// vow:define @Sentinel
var ErrFoo = errors.New("foo")

// CloseFn is the callback shape the higher-order fixtures hand
// off. Each marker's subject (cb / handler / b) points at a
// parameter of this type so the resolver locates a single slot.
type CloseFn func() error

// ArgFn carries a single pointer parameter so the vow:nil[cb]
// fixtures can pin the contract on a position the callback
// actually exposes.
type ArgFn func(p *int) error

// PairFn returns two positions so the return-axis arity fixtures
// can pin where the empty-expression unification stops.
type PairFn func(p *int) (*int, error)

// HandlerFn is the callback shape the vow:use fixture asserts a
// caller-side discharge against. The vow:use[handler] scope
// qualifier names this parameter so the resolver clears the
// marker.
type HandlerFn func(error) error

// ----- vow:nil[subject] -----

// nilSilentBaseline declares the cb callback's argument as non-nil
// via the [cb] scope qualifier. The cb identifier appears in the
// signature, the cb's signature is function-typed, and the
// contract's parameter count matches the callback's arity so the
// validator stays silent on every check it runs.
//
// vow:nil[cb] () ?
func nilSilentBaseline(cb ArgFn) error { return cb(nil) }

// nilDiagnoseUnresolvable points the nil declaration at an
// identifier the signature does not declare. The resolver reports
// a vow[nil-safety] diagnostic at the function declaration so the
// author sees the typo instead of silently losing the contract.
//
// vow:nil[nope] (!)
func nilDiagnoseUnresolvable(cb ArgFn) error { return cb(nil) } // want `vow\[nil-safety\]: vow:nil\[nope\] subject does not name a receiver, parameter, or named return of the enclosing function`

// nilDiagnoseNotCallback anchors the scoped contract at a regular
// pointer parameter rather than a function-typed slot. The
// callback-type gate catches the mismatch at the function
// declaration.
//
// vow:nil[count] (!)
func nilDiagnoseNotCallback(count *int) error { _ = count; return nil } // want `vow\[nil-safety\]: vow:nil\[count\] subject is not a callback; the contract only carries through a function-typed slot`

// nilDiagnoseArityOverflow declares one more parameter position
// than the callback's signature exposes. The arity gate surfaces
// the count mismatch at the function declaration so a wider
// contract than the callback admits does not silently bind to a
// non-existent slot.
//
// vow:nil[cb] (!, !) ?
func nilDiagnoseArityOverflow(cb ArgFn) error { return cb(nil) } // want `vow\[nil-safety\]: vow:nil\[cb\] declares 2 parameter positions but the callback signature has 1`

// nilDiagnoseReturnOverflow declares one more return position
// than the callback's signature exposes. The arity gate fires on
// the return-axis branch so an over-wide return contract surfaces
// the same way an over-wide parameter contract does.
//
// vow:nil[cb] () !, !
func nilDiagnoseReturnOverflow(cb ArgFn) error { return cb(nil) } // want `vow\[nil-safety\]: vow:nil\[cb\] declares 2 return positions but the callback signature has 1`

// nilDiagnoseArityUnder declares one parameter position while
// BinaryFn's signature exposes two. The subject-scoped arity gate
// surfaces the count mismatch on the parameter axis; the empty-
// parens exception applies only to 0/1-param callbacks. BinaryFn
// is defined further down in this file for the vow:emit fixtures
// and reused here.
//
// vow:nil[bcb] (!) ?
func nilDiagnoseArityUnder(bcb BinaryFn) error { return bcb(nil, nil) } // want `vow\[nil-safety\]: vow:nil\[bcb\] declares 1 parameter positions but the callback signature has 2`

// nilDiagnoseReturnUnified declares no return positions while
// ArgFn returns one. The empty-expression unification reaches the
// subject-scoped surface too, so the arity gate stays silent.
//
// vow:nil[cb] (!)
func nilDiagnoseReturnUnified(cb ArgFn) error { return cb(nil) }

// nilDiagnoseReturnBoundary pins where it stops: PairFn returns
// two positions.
//
// vow:nil[pcb] (!)
func nilDiagnoseReturnBoundary(pcb PairFn) error { // want `vow\[nil-safety\]: vow:nil\[pcb\] declares 0 return positions but the callback signature has 2`
	_, err := pcb(nil)
	return err
}

// ----- vow:use[subject] -----

// useSilentBaseline asserts that the handler callback discharges
// ErrFoo from the caller side. The [handler] qualifier names a
// parameter the resolver locates, so no subject-resolution
// diagnostic fires; the function-level discharge tracker leaves
// the scoped assertion to a layer.
//
// vow:use[handler] ErrFoo
func useSilentBaseline(handler HandlerFn) error { _ = handler; return nil }

// useDiagnoseUnresolvable points the discharge assertion at a
// subject the signature does not declare.
//
// vow:use[absent] ErrFoo
func useDiagnoseUnresolvable(handler HandlerFn) error { _ = handler; return nil } // want `vow\[sentinel-error\]: vow:use\[absent\] subject does not name a receiver, parameter, or named return of the enclosing function`

// useDiagnoseNotCallback anchors the scoped discharge at a
// regular pointer parameter rather than a function-typed slot.
// Discharge only propagates through a callback, so a non-function
// subject surfaces the diagnostic at the function declaration.
//
// vow:use[count] ErrFoo
func useDiagnoseNotCallback(count *int) error { _ = count; return nil } // want `vow\[sentinel-error\]: vow:use\[count\] subject is not a callback; discharge only propagates through function-typed slots`

// useLineScopeSilentBaseline anchors a line-scope discharge
// marker inside the body. The [handler] qualifier names a
// function-typed parameter so the callback-type gate stays
// silent; the existing line-scope resolver clears the scope and
// the new gate adds no diagnostic.
func useLineScopeSilentBaseline(handler HandlerFn) error {
	_ = handler
	// vow:use[handler] ErrFoo
	return nil
}

// useLineScopeNotCallback anchors a line-scope discharge marker
// at a non-function subject. The callback-type gate fires at the
// comment so the author sees the mismatch alongside the offending
// line.
func useLineScopeNotCallback(count *int) error {
	_ = count
	// vow:use[count] ErrFoo // want `vow\[sentinel-error\]: vow:use\[count\] subject is not a callback; discharge only propagates through function-typed slots`
	return nil
}

// ----- vow:use[$N] positional return subject -----

// usePositionalSilentBaseline names a positional slot in the
// signature: `$1` reaches the first return value, which is an
// unnamed callback. The resolver locates the slot by index so
// no subject-resolution diagnostic fires even though the slot
// carries no declared name.
//
// vow:use[$1] ErrFoo
func usePositionalSilentBaseline() (HandlerFn, error) { return nil, nil }

// usePositionalOutOfRange names a positional slot beyond the
// return list. The resolver scans the return slots and returns
// no match, so the standard subject-unresolved diagnostic
// surfaces at the function declaration.
//
// vow:use[$5] ErrFoo
func usePositionalOutOfRange() (HandlerFn, error) { return nil, nil } // want `vow\[sentinel-error\]: vow:use\[\$5\] subject does not name a receiver, parameter, or named return of the enclosing function`

// usePositionalNotCallback names a positional return slot whose
// declared type is a non-function value. The callback-type gate
// fires at the function declaration so the author sees the
// mismatch with a precise reason.
//
// vow:use[$1] ErrFoo
func usePositionalNotCallback() (*int, error) { return nil, nil } // want `vow\[sentinel-error\]: vow:use\[\$1\] subject is not a callback; discharge only propagates through function-typed slots`

// ----- vow:emit[subject] -----

// emitSilentBaseline propagates an ErrFoo emission to the cb
// callback. The [cb] qualifier resolves cleanly so the marker
// records without surfacing a subject-resolution diagnostic;
// callee self-validation skips the scoped case because the
// emission contract belongs to the callback, not the enclosing
// function's return statements.
//
// vow:emit[cb] ErrFoo
func emitSilentBaseline(cb CloseFn) error { _ = cb; return nil }

// emitDiagnoseUnresolvable claims an emission for an identifier
// the signature does not declare.
//
// vow:emit[gone] ErrFoo
func emitDiagnoseUnresolvable(cb CloseFn) error { _ = cb; return nil } // want `vow\[sentinel-error\]: vow:emit\[gone\] subject does not name a receiver, parameter, or named return of the enclosing function`

// ----- vow:cond[subject] callback reference resolution -----

// BinaryFn is a callback shape with named parameters so the
// logical-arrow rules below reference left and right by name and
// the validator resolves them against the callback's own
// signature scope rather than the enclosing function.
type BinaryFn func(left, right *int) error

// condCallbackRefSilent declares a logical-arrow rule scoped to
// the fn callback. Both operands reference BinaryFn's parameters
// directly (left and right), so the callback-signature resolver
// finds them and the validator stays silent.
//
// vow:cond[fn] left != nil => right != nil
func condCallbackRefSilent(fn BinaryFn) error { _ = fn; return nil }

// condCallbackRefDiagnose declares a logical-arrow rule scoped
// to the fn callback but references an identifier the callback
// signature does not declare. The validator surfaces a
// diagnostic at the function declaration so the unresolvable
// reference does not silently lose the contract.
//
// vow:cond[fn] missing != nil => right != nil
func condCallbackRefDiagnose(fn BinaryFn) error { _ = fn; return nil } // want `vow\[sentinel-error\]: vow:cond\[fn\] references "missing", which is not a parameter, receiver, or named return of the callback signature`

// condCallbackDollarSilent exercises the `$N` positional return
// reference on the callback side. BinaryFn returns one value, so
// $1 resolves cleanly.
//
// vow:cond[fn] left != nil => $1 != nil
func condCallbackDollarSilent(fn BinaryFn) error { _ = fn; return nil }

// condCallbackDollarDiagnose references a positional return slot
// that exceeds BinaryFn's single-return arity. The validator
// surfaces a diagnostic because the callback signature has no
// $2 slot to bind against.
//
// vow:cond[fn] left != nil => $2 != nil
func condCallbackDollarDiagnose(fn BinaryFn) error { _ = fn; return nil } // want `vow\[sentinel-error\]: vow:cond\[fn\] references \$2 but the callback signature has fewer return positions`

// ----- vow:emit[subject] callback type check -----

// emitNonCallbackDiagnose anchors the marker to a regular pointer
// parameter rather than a function-typed slot. Emission only
// propagates through a callback, so a non-function subject
// surfaces a diagnostic at the function declaration.
//
// vow:emit[count] ErrFoo
func emitNonCallbackDiagnose(count *int) error { _ = count; return nil } // want `vow\[sentinel-error\]: vow:emit\[count\] subject is not a callback; emission only propagates through function-typed slots`

// emitStatementScopeSilentBaseline anchors a statement-scope
// emission marker inside the body. The [cb] qualifier names a
// function-typed parameter so the callback-type gate stays
// silent; the existing destination inferrer skips the scoped
// shape because the contract belongs to the callback.
func emitStatementScopeSilentBaseline(cb CloseFn) error {
	_ = cb
	// vow:emit[cb] ErrFoo
	return nil
}

// emitStatementScopeNotCallback anchors a statement-scope
// emission marker at a non-function subject. The callback-type
// gate fires at the comment so the author sees the mismatch
// alongside the offending line.
func emitStatementScopeNotCallback(count *int) error {
	_ = count
	// vow:emit[count] ErrFoo // want `vow\[sentinel-error\]: vow:emit\[count\] subject is not a callback; emission only propagates through function-typed slots`
	return nil
}
