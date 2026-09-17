// Package nilDeclReassign pins the value a call or a return actually
// hands over when the enclosing function assigns to a local after the
// definition that introduces it. The declaration resolver answers from
// that definition, so these cases are the ones where the definition and
// the handover disagree.
package nilDeclReassign

// requireNonNil rejects nil at its only parameter, which is what makes
// each subject below either report or stay silent.
//
// vow:nil (!)
func requireNonNil(p *int) {} // want requireNonNil:"nilDecl\\(!\\)"

// maybeNil is the nillable source the reassignment subjects draw from
// when the assigned value is itself nil-capable.
//
// vow:nil () ?
func maybeNil() *int { return nil } // want maybeNil:"nilDecl\\(\\) \\?"

// aliasReports is the control: nothing assigns to a after the
// definition, so the nillable parameter really is what the call hands
// over. A subject that stops reporting here means the alias resolver
// stopped answering, not that a reassignment was honoured.
//
// vow:nil (?)
func aliasReports(p *int) { // want aliasReports:"nilDecl\\(\\?\\)"
	a := p
	requireNonNil(a) // want `vow\[nil-safety\]: argument 1 \(p\) may be nil through flow`
}

// plainOverwriteOK replaces the value before the call, so nothing
// nillable reaches the callee.
//
// vow:nil (?)
func plainOverwriteOK(p *int) { // want plainOverwriteOK:"nilDecl\\(\\?\\)"
	a := p
	x := 1
	a = &x
	requireNonNil(a)
}

// guardedDefaultOK substitutes a value when the original is nil. The
// call is reached from two edges: one carries the substitute, the
// other carries the original on the side where the nil test failed.
//
// vow:nil (?)
func guardedDefaultOK(p *int) { // want guardedDefaultOK:"nilDecl\\(\\?\\)"
	a := p
	if a == nil {
		x := 1
		a = &x
	}
	requireNonNil(a)
}

// overwriteWithNillableReports assigns a second nillable value, so the
// handover is still nil-capable. Suppressing a subject merely because
// an assignment follows the definition would lose this one.
//
// vow:nil (?)
func overwriteWithNillableReports(p *int) { // want overwriteWithNillableReports:"nilDecl\\(\\?\\)"
	a := p
	a = maybeNil()
	requireNonNil(a) // want `vow\[nil-safety\]: argument 1 \(p\) may be nil through flow`
}

// conditionalNillableReports assigns a nillable value on one side of a
// branch, so one edge into the call carries nil.
//
// vow:nil (?,)
func conditionalNillableReports(p *int, cond bool) { // want conditionalNillableReports:"nilDecl\\(\\?,\\)"
	a := p
	if cond {
		a = maybeNil()
	}
	requireNonNil(a) // want `vow\[nil-safety\]: argument 1 \(p\) may be nil through flow`
}

// nilLiteralOnOneBranchReports assigns the nil literal on one side of
// a branch, so one edge into the call carries a value that is nil by
// construction rather than by declaration.
//
// vow:nil (?,)
func nilLiteralOnOneBranchReports(p *int, cond bool) { // want nilLiteralOnOneBranchReports:"nilDecl\\(\\?,\\)"
	a := p
	if cond {
		a = nil
	}
	requireNonNil(a) // want `vow\[nil-safety\]: argument 1 \(p\) may be nil through flow`
}

// returnAliasReports is the return-position control.
//
// vow:nil (?) !
func returnAliasReports(p *int) *int { // want returnAliasReports:"nilDecl\\(\\?\\) !"
	a := p
	return a // want `vow\[nil-safety\]: return position 1 may be nil through flow`
}

// returnPlainOverwriteOK replaces the value before the return runs.
//
// vow:nil (?) !
func returnPlainOverwriteOK(p *int) *int { // want returnPlainOverwriteOK:"nilDecl\\(\\?\\) !"
	a := p
	x := 1
	a = &x
	return a
}

// returnGuardedDefaultOK is the substitute-when-nil shape at a return
// position.
//
// vow:nil (?) !
func returnGuardedDefaultOK(p *int) *int { // want returnGuardedDefaultOK:"nilDecl\\(\\?\\) !"
	a := p
	if a == nil {
		x := 1
		a = &x
	}
	return a
}

// returnOverwriteWithNillableReports keeps the return position
// reporting when the assigned value is itself nil-capable.
//
// vow:nil (?) !
func returnOverwriteWithNillableReports(p *int) *int { // want returnOverwriteWithNillableReports:"nilDecl\\(\\?\\) !"
	a := p
	a = maybeNil()
	return a // want `vow\[nil-safety\]: return position 1 may be nil through flow`
}
