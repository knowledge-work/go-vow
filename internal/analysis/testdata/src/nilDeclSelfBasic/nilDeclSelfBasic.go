// Package nilDeclSelfBasic pins the callee-side body-level
// contractions that a vow:nil signature decl recognises from the
// function body alone: a `!` position reassigned or returned as
// nil, a `!` return slot supplied with a literal nil, or a leading
// `if x == nil` guard whose then-branch is unreachable because the
// decl already proves the value non-nil at entry. Transitive flow
// (a nillable local that reaches the slot) falls through unreported;
// the SSA forward pass does not cover it.
package nilDeclSelfBasic

// Box is the receiver type for the method fixtures.
type Box struct{}

// paramReassignNil pins a declared-`!` parameter overwritten with
// the bare nil literal inside the body. The contract states the
// position stays non-nil; the assignment contradicts the contract
// on first principles.
//
// vow:nil (!)
func paramReassignNil(p *int) { // want paramReassignNil:"nilDecl\\(!\\)"
	p = nil // want `vow\[nil-safety\]: p reassigned to nil; vow:nil declared p ! at this position`
}

// paramReassignTypedNil pins the same path on a typed-nil
// conversion right-hand side. The structural contradiction is
// identical regardless of how the author spells nil.
//
// vow:nil (!)
func paramReassignTypedNil(p *int) { // want paramReassignTypedNil:"nilDecl\\(!\\)"
	p = (*int)(nil) // want `vow\[nil-safety\]: p reassigned to nil; vow:nil declared p ! at this position`
}

// paramReassignNonNil pins the silent path: the value the body
// writes into the position is itself non-nil at the source. The
// reassign check never fires because the right-hand side carries
// no nil proof.
//
// vow:nil (!)
func paramReassignNonNil(p *int) { // want paramReassignNonNil:"nilDecl\\(!\\)"
	other := 1
	p = &other
}

// nillableParamReassignOK pins the silent path on a nillable
// parameter. The decl admits a nil value at the position, so an
// in-body reassignment to nil is consistent with the contract and
// stays unreported.
//
// vow:nil (?)
func nillableParamReassignOK(p *int) { // want nillableParamReassignOK:"nilDecl\\(\\?\\)"
	p = nil
}

// groupedParamReassign pins the index-alignment invariant between
// the decl's parameter slots and a grouped Go parameter list. The
// signature expands `a, b *int` into two name entries; the decl's
// `(!, !)` carries one entry per comma. The reassign check fires
// on both names independently, so the silent half (a non-nil-side
// reassign here would simply not appear) is not exercised on this
// function — it stays implicit through the contract.
//
// vow:nil (!,!)
func groupedParamReassign(a, b *int) { // want groupedParamReassign:"nilDecl\\(!,!\\)"
	a = nil // want `vow\[nil-safety\]: a reassigned to nil; vow:nil declared a ! at this position`
	b = nil // want `vow\[nil-safety\]: b reassigned to nil; vow:nil declared b ! at this position`
}

// recvReassignNil pins the receiver-side variant of the reassign
// check: the receiver is declared `!` and the body overwrites it
// with a nil literal.
//
// vow:nil !.()
func (b *Box) recvReassignNil() { // want recvReassignNil:"nilDecl!\\.\\(\\)"
	b = nil // want `vow\[nil-safety\]: b reassigned to nil; vow:nil declared b ! at this position`
}

// returnNonNilSlotNil pins a return statement whose first slot
// supplies the bare nil literal at a position the decl pins as
// `!`. The decl promises non-nil at the call site; the literal
// directly contradicts the promise.
//
// vow:nil () !
func returnNonNilSlotNil() *int { // want returnNonNilSlotNil:"nilDecl\\(\\) !"
	return nil // want `vow\[nil-safety\]: return position 1 is nil; vow:nil declared ! at this return slot`
}

// returnNonNilSlotTypedNil pins the same path on a typed-nil
// conversion. The structural contradiction is identical.
//
// vow:nil () !
func returnNonNilSlotTypedNil() *int { // want returnNonNilSlotTypedNil:"nilDecl\\(\\) !"
	return (*int)(nil) // want `vow\[nil-safety\]: return position 1 is nil; vow:nil declared ! at this return slot`
}

// returnNonNilSlotOK pins the silent path: the return expression
// is a non-nil literal. The decl is satisfied at the source.
func returnNonNilSlotOK() *int {
	x := 1
	return &x
}

// returnNillableSlotNilOK pins the silent path on a nillable
// return slot. The decl admits nil at the position, so the literal
// return stays consistent with the contract.
//
// vow:nil () ?
func returnNillableSlotNilOK() *int { // want returnNillableSlotNilOK:"nilDecl\\(\\) \\?"
	return nil
}

// returnMixedSlots pins the multi-slot case where one position is
// `!` and the other is `?`. The diagnostic fires on the `!` slot
// and stays silent on the `?` slot in the same statement.
//
// vow:nil () !,?
func returnMixedSlots() (*int, *int) { // want returnMixedSlots:"nilDecl\\(\\) !,\\?"
	return nil, nil // want `vow\[nil-safety\]: return position 1 is nil; vow:nil declared ! at this return slot`
}

// deadGuardOnNonNilParam pins a leading `if x == nil` guard whose
// then-branch short-circuits and whose parameter the decl pins as
// `!`. The decl already proves the value non-nil at entry, so the
// guard is dead and almost always signals the author meant `?`.
//
// vow:nil (!)
func deadGuardOnNonNilParam(p *int) { // want deadGuardOnNonNilParam:"nilDecl\\(!\\)"
	if p == nil { // want `vow\[nil-safety\]: guard on p is dead; vow:nil declared p ! at this position \(the value is already non-nil at entry\)`
		return
	}
	_ = *p
}

// deadGuardOnNonNilRecv pins the receiver-side variant of the
// dead-guard check.
//
// vow:nil !.()
func (b *Box) deadGuardOnNonNilRecv() { // want deadGuardOnNonNilRecv:"nilDecl!\\.\\(\\)"
	if b == nil { // want `vow\[nil-safety\]: guard on b is dead; vow:nil declared b ! at this position \(the value is already non-nil at entry\)`
		return
	}
}

// nillableParamGuardOK pins the silent path: the parameter is
// declared `?` so a leading nil guard is exactly what the decl
// invites the author to write.
//
// vow:nil (?)
func nillableParamGuardOK(p *int) { // want nillableParamGuardOK:"nilDecl\\(\\?\\)"
	if p == nil {
		return
	}
	_ = *p
}
