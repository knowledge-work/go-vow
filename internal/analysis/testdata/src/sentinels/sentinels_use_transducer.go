package sentinels

import "errors"

// A `vow:use X` declaration on a function whose signature
// returns a single bool promotes the function to transducer
// status. A call site that evaluates the predicate inside a
// conditional context discharges the must-consume obligation
// only for the declared subjects, leaving other tracked
// subjects to fall through to the leak path.
//
// This file pins the following shapes in the sentinel-error
// preset context:
//
//   - positive: declared subject in argument position, call inside
//     `if` cond
//   - positive: declared subject in argument position, call inside
//     `switch` cond after init
//   - positive: one of multiple declared subjects observed
//   - negative: un-declared subject leaks even when the transducer
//     call is in a conditional
//   - alias-return: defined or aliased boolean return type stays
//     eligible

// matchesNotFound transduces the ErrNotFound sentinel to bool.
// Other tracked subjects are not discharged by calls to this
// predicate even when the call sits in a conditional context. The
// body wraps the errors.Is call in an if so the ErrNotFound subject
// reference inside the predicate body is itself observed by the
// preset's errors.Is recognition; without the wrapper, the in-body
// reference would leak.
//
// vow:use ErrNotFound
func matchesNotFound(err error) bool {
	if errors.Is(err, ErrNotFound) {
		return true
	}
	return false
}

// matchesNotFoundOrFlow declares two subjects through a
// comma-separated marker payload. Either ErrNotFound or
// sentinelForFlow passed at a call site inside conditional
// context is discharged.
//
// vow:use ErrNotFound, sentinelForFlow
func matchesNotFoundOrFlow(err error) bool {
	if errors.Is(err, ErrNotFound) {
		return true
	}
	if errors.Is(err, sentinelForFlow) {
		return true
	}
	return false
}

// observedInIfCond exercises the canonical observe path. The
// matchesNotFound call appears as the condition of an `if`, and
// ErrNotFound (the declared subject) is passed as an argument. The
// reference is discharged.
func observedInIfCond(err error) error {
	if matchesNotFound(ErrNotFound) {
		return nil
	}
	return err
}

// observedInSwitchInit exercises the same path through a switch's
// init clause. The boolean glue inside isInConditional
// transparently traverses the assignment.
func observedInSwitchInit(err error) error {
	switch ok := matchesNotFound(ErrNotFound); ok {
	case true:
		return nil
	}
	return err
}

// observedThroughMultiSubject exercises the two-subject shape.
// sentinelForFlow is one of the declared subjects, so the call
// inside an `if` discharges the reference.
func observedThroughMultiSubject(err error) error {
	if matchesNotFoundOrFlow(sentinelForFlow) {
		return nil
	}
	return err
}

// leakedAtUnDeclaredSubject pins the subject-specification gate.
// matchesNotFound declares only ErrNotFound; passing ErrFoo at the
// call site means the transducer is not observing this reference,
// and the leak path fires even though the call sits inside an `if`
// condition.
func leakedAtUnDeclaredSubject(err error) error {
	if matchesNotFound(ErrFoo) { // want `vow\[sentinel-error\]: sentinel error ErrFoo leaked: needs observation or explicit propagation`
		return nil
	}
	return err
}

// matchedAlias is a transparent alias for bool. A predicate that
// returns this alias is still a boolean predicate as far as Go's
// type system is concerned, and it is usable directly in an `if`
// condition. The eligibility check resolves the return type through
// types.Info, so the alias does not block registration.
type matchedAlias = bool

// vow:use ErrNotFound
func matchesAlias(err error) matchedAlias {
	if errors.Is(err, ErrNotFound) {
		return true
	}
	return false
}

// observedThroughAlias would leak at ErrNotFound if the eligibility
// check rejected matchesAlias purely because its declared return
// type is spelled as an alias identifier rather than the bare token
// `bool`. The types.Info-driven check accepts the alias and the
// observe path fires for the declared subject.
func observedThroughAlias(err error) error {
	if matchesAlias(ErrNotFound) {
		return nil
	}
	return err
}
