// Package closableGeneric is the analysistest fixture for the
// generic TypeParam Closable case (d). The fixture exercises the
// constraint walker that credits a generic type parameter whose
// constraint embeds (or names) a registered Closable interface
// the same way the analyzer already credits concrete pointer and
// interface forms. Two detection paths are pinned: a function
// whose parameter type is the type parameter (the acquisition
// site needs the same discharge as a concrete *Conn would), and
// a return-position upcast where the generic value flows out as
// a non-Closable interface.
package closableGeneric

import "io"

// vow:define @Closable
type Conn struct{}

func (c *Conn) Close() error { return nil }

// openGeneric returns a Closable value of the type parameter's
// declared type. The constraint embeds io.Closer so the analyzer
// credits T as Closable through case (d).
func openGeneric[T io.Closer](make func() T) T {
	return make()
}

// silentBaselineGeneric discharges the generic Closable
// acquisition with a defer Close call so the analyzer stays
// silent.
func silentBaselineGeneric() {
	c := openGeneric(func() *Conn { return &Conn{} })
	defer c.Close()
	_ = c
}

// diagnoseLeakGeneric acquires the generic Closable but never
// discharges it. The constraint-credit path treats the bound type
// parameter the same way it treats a concrete Closable, so the
// analyzer reports the leak at the acquisition site.
func diagnoseLeakGeneric() {
	c := openGeneric(func() *Conn { return &Conn{} }) // want `vow\[closable\]: c is acquired but not closed; add a defer Close call or declare the transfer with vow:emit`
	_ = c
}

// upcastReturnDiagnose returns a generic Closable as a bare
// `any`. The destination is a non-Closable interface, so the
// upcast check reports the drop at the return expression. The
// case (d) credit is what makes the source side count as
// Closable here.
func upcastReturnDiagnose[T io.Closer](v T) any {
	return v // want `vow\[closable\]: returning a Closable value as any drops the close obligation`
}

// silentNonClosableConstraint pins the no-false-positive
// invariant for case (d): a type parameter whose constraint is
// `any` carries no Closable interface in its constraint, so the
// walker leaves the parameter unconstrained and the acquisition
// site reports nothing.
func silentNonClosableConstraint[T any](make func() T) {
	v := make()
	_ = v
}
