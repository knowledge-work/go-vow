// Package nilDeclFieldDeref pins the guard obligation a `?`-declared
// field places on its readers. The declaration admits nil, so
// following the pointer without first proving it non-nil is the panic
// the declaration warns about. A guard over the same field location
// discharges the site.
package nilDeclFieldDeref

// Inner is the pointee whose members a dereference reaches.
type Inner struct{ N int }

// ByValue has a value receiver, so naming it through a pointer has to
// dereference the pointer first.
func (i Inner) ByValue() int { return i.N }

// ByPointer has a pointer receiver, so naming it through a pointer
// passes the pointer along and dereferences nothing.
func (i *Inner) ByPointer() int { return 0 }

// Holder carries a nillable field, a second nillable field, and a
// non-nil one so each declaration state is pinned separately.
type Holder struct {
	// vow:nil ?
	Maybe *Inner // want Maybe:"fieldNil\\(\\?\\)"

	// vow:nil ?
	Other *Inner // want Other:"fieldNil\\(\\?\\)"

	// vow:nil !
	Sure *Inner // want Sure:"fieldNil\\(!\\)"

	// Plain carries no marker, so the platform state claims nothing
	// either way and the reader owes no guard.
	Plain *Inner
}

// mutate receives the whole Holder, so it may write the field.
func mutate(h *Holder) {}

// ---------- reported ----------

// unguardedFieldRead reaches a member through the nillable field with
// no guard standing.
func unguardedFieldRead(h *Holder) int {
	return h.Maybe.N // want `vow\[nil-safety\]: field Maybe is dereferenced without a nil guard; vow:nil declared \? on this field`
}

// unguardedStarDeref applies the dereference operator directly.
func unguardedStarDeref(h *Holder) Inner {
	return *h.Maybe // want `vow\[nil-safety\]: field Maybe is dereferenced without a nil guard; vow:nil declared \? on this field`
}

// unguardedValueReceiverCall names a value-receiver method, which has
// to dereference the pointer to build the receiver.
func unguardedValueReceiverCall(h *Holder) int {
	return h.Maybe.ByValue() // want `vow\[nil-safety\]: field Maybe is dereferenced without a nil guard; vow:nil declared \? on this field`
}

// guardOnOtherField guards a different field, which proves nothing
// about this one.
func guardOnOtherField(h *Holder) int {
	if h.Other != nil {
		return h.Maybe.N // want `vow\[nil-safety\]: field Maybe is dereferenced without a nil guard; vow:nil declared \? on this field`
	}
	return 0
}

// writeBetweenGuardAndDeref guards the field and then hands the struct
// to a callee that may write it, so the proof does not reach the
// dereference.
func writeBetweenGuardAndDeref(h *Holder) int {
	if h.Maybe != nil {
		mutate(h)
		return h.Maybe.N // want `vow\[nil-safety\]: field Maybe is dereferenced without a nil guard; vow:nil declared \? on this field`
	}
	return 0
}

// twoFieldsBothUnguarded pins that the report is per field: each
// nillable field owes its own guard.
func twoFieldsBothUnguarded(h *Holder) int {
	return h.Maybe.N + // want `vow\[nil-safety\]: field Maybe is dereferenced without a nil guard; vow:nil declared \? on this field`
		h.Other.N // want `vow\[nil-safety\]: field Other is dereferenced without a nil guard; vow:nil declared \? on this field`
}

// repeatedUnguardedReads reads the same field twice under one missing
// guard, which surfaces once rather than once per read.
func repeatedUnguardedReads(h *Holder) int {
	return h.Maybe.N + // want `vow\[nil-safety\]: field Maybe is dereferenced without a nil guard; vow:nil declared \? on this field`
		h.Maybe.N
}

// ---------- silent ----------

// guardedFieldRead proves the field non-nil before reaching a member.
func guardedFieldRead(h *Holder) int {
	if h.Maybe != nil {
		return h.Maybe.N
	}
	return 0
}

// earlyReturnGuard discharges the field with a short-circuiting guard.
func earlyReturnGuard(h *Holder) int {
	if h.Maybe == nil {
		return 0
	}
	return h.Maybe.N
}

// guardedStarDeref proves the field before applying the dereference
// operator.
func guardedStarDeref(h *Holder) Inner {
	if h.Maybe == nil {
		return Inner{}
	}
	return *h.Maybe
}

// guardedOnMethodReceiver pins the receiver as the base value.
func (h *Holder) guardedOnMethodReceiver() int {
	if h.Maybe != nil {
		return h.Maybe.N
	}
	return 0
}

// pointerReceiverCallIsNotADeref names a pointer-receiver method
// through the field. The pointer is passed along rather than
// dereferenced, so the call site owes no guard — the method's own body
// is where a nil receiver would have to be handled.
//
// This case is load-bearing twice over. It separates a value receiver
// from a pointer one, and it also pins which selector the deref
// predicate is asked about: applied to the field selector rather than
// to the selection built on top of it, the predicate answers "a field
// reached through a pointer" and every field read becomes a deref
// site — this call is the one that then reports.
func pointerReceiverCallIsNotADeref(h *Holder) int {
	return h.Maybe.ByPointer()
}

// nonNilFieldRead reaches a member through a `!`-declared field, whose
// construction side owes the non-nil member, so the reader skips the
// guard by contract.
func nonNilFieldRead(h *Holder) int {
	return h.Sure.N
}

// platformFieldRead reaches a member through an unmarked field. The
// declaration claims nothing, so this surface stays out of the way.
func platformFieldRead(h *Holder) int {
	return h.Plain.N
}

// fieldPassedNotDereferenced hands the field to a callee without
// following the pointer. That is the argument check's surface, not this
// one, and the callee here declares nothing.
func fieldPassedNotDereferenced(h *Holder) {
	accept(h.Maybe)
}

// accept takes the pointer without declaring a contract on it.
func accept(p *Inner) {}
