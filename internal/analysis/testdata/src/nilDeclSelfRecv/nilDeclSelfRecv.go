// Package nilDeclSelfRecv pins the receiver-guard check that
// fires when a method whose decl declares the receiver as
// nillable (`?`) dereferences the receiver without a leading
// nil guard. The decl explicitly admits a nil receiver, so the
// body must guard the deref before the first use; an unguarded
// deref is the typical nil-receiver panic source.
package nilDeclSelfRecv

// Account is the receiver type for the fixtures.
type Account struct {
	Name string
}

// Echo is a helper that returns the receiver name so a method
// body can exercise a method-call deref instead of only a field
// access.
func (a *Account) Echo() string { return a.Name }

// unguardedFieldAccess pins the diagnostic: the receiver decl is
// `?`, the body opens with the field-access deref, no leading
// nil guard short-circuits before it. The diagnostic anchors at
// the first deref site.
//
// vow:nil ?.()
func (a *Account) unguardedFieldAccess() string { // want unguardedFieldAccess:"nilDecl\\?\\.\\(\\)"
	return a.Name // want `vow\[nil-safety\]: receiver a is dereferenced without a nil guard; vow:nil declared a \? at this position`
}

// unguardedMethodCall pins the same diagnostic on a method-call
// shape: `a.Echo()` rides into the same selector-expression
// recogniser.
//
// vow:nil ?.()
func (a *Account) unguardedMethodCall() string { // want unguardedMethodCall:"nilDecl\\?\\.\\(\\)"
	return a.Echo() // want `vow\[nil-safety\]: receiver a is dereferenced without a nil guard; vow:nil declared a \? at this position`
}

// guardedDerefOK pins the silent path: a leading
// `if a == nil { return ... }` short-circuit guards the deref so
// no diagnostic fires.
//
// vow:nil ?.()
func (a *Account) guardedDerefOK() string { // want guardedDerefOK:"nilDecl\\?\\.\\(\\)"
	if a == nil {
		return ""
	}
	return a.Name
}

// nonNilReceiverOK pins the silent path on a non-nil receiver:
// the decl pins `!`, so the deref is already proven safe at the
// signature level. The body needs no guard.
//
// vow:nil !.()
func (a *Account) nonNilReceiverOK() string { // want nonNilReceiverOK:"nilDecl!\\.\\(\\)"
	return a.Name
}

// platformReceiverOK pins the silent path on a platform receiver:
// the method carries no vow:nil marker, so the analyzer does not
// track the receiver and the deref stays silent regardless of
// guard presence.
func (a *Account) platformReceiverOK() string {
	return a.Name
}

// positiveGuardOK pins the silent path on the `if recv != nil`
// idiom — the most common Go form for handling a nillable receiver.
// The leading positive guard advertises that the body has
// considered nilness, so the deref inside the then-branch stays
// silent.
//
// vow:nil ?.()
func (a *Account) positiveGuardOK() string { // want positiveGuardOK:"nilDecl\\?\\.\\(\\)"
	if a != nil {
		return a.Name
	}
	return ""
}

// shortCircuitGuardOK pins the silent path when the receiver check
// leads an `||` chain that also inspects a field behind the same
// pointer. Go evaluates `a.Name` only when `a == nil` is false, so
// the leading comparison still guards it.
//
// vow:nil ?.()
func (a *Account) shortCircuitGuardOK() string { // want shortCircuitGuardOK:"nilDecl\\?\\.\\(\\)"
	if a == nil || a.Name == "" {
		return ""
	}
	return a.Name
}

// andPositiveGuardOK pins the same silent path on the positive
// form: `a != nil && …` only reaches the right operand when the
// receiver is non-nil.
//
// vow:nil ?.()
func (a *Account) andPositiveGuardOK() string { // want andPositiveGuardOK:"nilDecl\\?\\.\\(\\)"
	if a != nil && a.Name != "" {
		return a.Name
	}
	return ""
}

// derefBeforeCheckReports pins the boundary that keeps the
// left-operand rule honest: the deref sits to the left of the
// receiver comparison, so Go evaluates it first and the check
// never guards it. This is the shape a positional rule has to keep
// reporting — accepting a receiver check anywhere in the chain
// would silence a real nil-receiver panic.
//
// vow:nil ?.()
func (a *Account) derefBeforeCheckReports() string { // want derefBeforeCheckReports:"nilDecl\\?\\.\\(\\)"
	if a.Name == "" || a == nil { // want `vow\[nil-safety\]: receiver a is dereferenced without a nil guard; vow:nil declared a \? at this position`
		return ""
	}
	return a.Name
}

// closureShadowOK pins the silent path when a nested function
// literal shadows the receiver name. The inner `a` is a different
// types.Object than the receiver, so its deref does not borrow
// the receiver's identity; the receiver itself is properly
// guarded, so no diagnostic fires.
//
// vow:nil ?.()
func (a *Account) closureShadowOK() string { // want closureShadowOK:"nilDecl\\?\\.\\(\\)"
	if a == nil {
		return ""
	}
	echo := func(a *Account) string {
		return a.Name
	}
	return echo(a)
}
