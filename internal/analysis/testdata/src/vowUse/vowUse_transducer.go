package vowUse

// A `vow:use X` declaration on a function whose signature
// returns a single bool promotes the function to transducer
// status: a call site that evaluates the predicate inside a
// conditional context discharges the must-consume obligation
// for the declared subjects. The fixtures below pin the
// transducer behaviour alongside the caller-side discharge
// contract.

import "errors"

// matchesErrFoo transduces ErrFoo to bool. The function-doc
// carries vow:use ErrFoo so the caller-side discharge contract
// applies in the usual way; the bool-return signature also
// promotes the function to transducer status so a call site
// that evaluates the predicate inside a conditional counts as
// a (b) discharge for ErrFoo.
//
// vow:use ErrFoo
func matchesErrFoo(err error) bool {
	if errors.Is(err, ErrFoo) {
		return true
	}
	return false
}

// matchesErrFooOrBar declares two subjects on a single vow:use
// line. The bool return promotes the function to transducer
// status for both ErrFoo and ErrBar; a call site that evaluates
// the predicate inside a conditional discharges whichever
// subject the call's argument names.
//
// vow:use ErrFoo, ErrBar
func matchesErrFooOrBar(err error) bool {
	if errors.Is(err, ErrFoo) {
		return true
	}
	if errors.Is(err, ErrBar) {
		return true
	}
	return false
}

// observedThroughUseTransducerIf pins the conditional-context
// observe path: a call to the bool-returning vow:use predicate
// sits inside an `if` and discharges the reference.
func observedThroughUseTransducerIf(err error) error {
	if matchesErrFoo(ErrFoo) {
		return nil
	}
	return err
}

// observedThroughUseTransducerMulti pins the multi-subject
// observe path: the predicate transduces both ErrFoo and
// ErrBar, so a call carrying ErrBar inside a conditional
// discharges that subject.
func observedThroughUseTransducerMulti(err error) error {
	if matchesErrFooOrBar(ErrBar) {
		return nil
	}
	return err
}

// observedThroughBoundBool pins the boolean-glue path. The
// transducer call result is bound to a local through if-init,
// and the cond position consults the binding. The AssignStmt
// counts as boolean evaluation glue, so the ErrFoo reference
// at the call argument is still credited as a (b) discharge.
func observedThroughBoundBool(err error) error {
	if ok := matchesErrFoo(ErrFoo); ok {
		return nil
	}
	return err
}

// observedThroughBooleanComposition pins the same recognition
// for `!` composition. The transducer call participates in a
// negated conditional; the unary operator counts as boolean
// evaluation glue, so the (b) discharge fires for ErrFoo at
// the call argument.
func observedThroughBooleanComposition(err error) error {
	if !matchesErrFoo(ErrFoo) {
		return err
	}
	return nil
}

// observedThroughSwitchInit routes the transducer call through
// a switch's init clause. The boolean glue inside
// isInConditional traverses the assignment so the tag-position
// bool still anchors the observe path; ErrFoo passed at the
// call site is discharged.
func observedThroughSwitchInit(err error) error {
	switch ok := matchesErrFoo(ErrFoo); ok {
	case true:
		return nil
	}
	return err
}

// leakedAtUnrelatedSubject pins the per-subject discharge gate.
// matchesErrFoo declares only ErrFoo; passing ErrBar at the
// call site means the transducer is not observing this
// reference, and the leak path fires even though the call sits
// in an `if` condition.
func leakedAtUnrelatedSubject(err error) error {
	if matchesErrFoo(ErrBar) { // want `vow\[sentinel-error\]: sentinel error ErrBar leaked: needs observation or explicit propagation`
		return nil
	}
	return err
}
