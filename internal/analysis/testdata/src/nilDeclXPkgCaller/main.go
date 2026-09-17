// Package nilDeclXPkgCaller is the caller-side fixture for the
// cross-package vow:nil trace. The import target declares each
// per-position contract through a fact (see nilDeclXPkgCallee);
// the analyzer reads the fact through pass.ImportObjectFact and
// surfaces a caller-side diagnostic when the call site supplies a
// statically nil argument, or a nillable-declared field, at a
// covered position.
package nilDeclXPkgCaller

import "nilDeclXPkgCallee"

// callWithLiteralNil exercises the cross-package detection on a
// bare nil literal at a `!`-pinned position. The callee's
// signatureNilFact carries the per-position contract; the literal
// nil at the call site contradicts the first slot, so the
// diagnostic must surface even though the callee's declaration
// lives in another package.
func callWithLiteralNil() {
	a := 1
	nilDeclXPkgCallee.RequireNonNil(nil, &a) // want `vow\[nil-safety\]: argument 1 \(p\) is nil`
}

// callWithTypedNil pins the typed-nil-conversion shape on the same
// cross-package path. The contract violation is identical
// regardless of whether the argument is the bare literal or a
// typed conversion.
func callWithTypedNil() {
	a := 1
	nilDeclXPkgCallee.RequireNonNil((*int)(nil), &a) // want `vow\[nil-safety\]: argument 1 \(p\) is nil`
}

// callBothNil pins the two-position case: both slots are `!`-
// pinned, both arguments are statically nil, so two diagnostics
// surface in declaration order.
func callBothNil() {
	nilDeclXPkgCallee.RequireNonNil(nil, nil) // want `vow\[nil-safety\]: argument 1 \(p\) is nil` `vow\[nil-safety\]: argument 2 \(q\) is nil`
}

// callFirstOnlyMixed pins the partial-pin case: only the first
// slot is `!`-pinned, so a literal nil at the platform second slot
// stays silent while the first slot still surfaces a diagnostic.
func callFirstOnlyMixed() {
	nilDeclXPkgCallee.FirstOnly(nil, nil) // want `vow\[nil-safety\]: argument 1 \(p\) is nil`
}

// callNillableSlot pins the silent baseline for a nillable-pinned
// position: the cross-package contract explicitly admits the nil
// argument, so no diagnostic surfaces.
func callNillableSlot() {
	nilDeclXPkgCallee.NillableOK(nil)
}

// callWithNonNil is the silent baseline: non-nil arguments flow
// through the same per-position contract without surfacing a
// diagnostic.
func callWithNonNil() {
	a := 1
	b := 2
	nilDeclXPkgCallee.RequireNonNil(&a, &b)
}

// callWithNillableFieldArg pins the cross-package field path: the
// Customer.Email field is `?`-declared in the callee's package, so
// passing it into a `!`-required slot surfaces the nillable-field
// diagnostic. The fact for Email rides through fieldNilFact at the
// field's type-checker Object.
func callWithNillableFieldArg() {
	var c nilDeclXPkgCallee.Customer
	nilDeclXPkgCallee.Consume(c.Email, c.ID) // want `vow\[nil-safety\]: argument 1 \(id\) is potentially nil \(field Email declared nillable via vow:nil\)`
}

// callWithNonNilFieldArg pins the silent baseline for a `!`-
// declared field flowing into a `!`-required slot: the declaring
// package's construction side owes the field a non-nil member, and
// that obligation is what lets the importing caller pass the field
// on with no guard of its own.
func callWithNonNilFieldArg() {
	var c nilDeclXPkgCallee.Customer
	nilDeclXPkgCallee.Consume(c.ID, c.ID)
}

// callWithPlatformFieldArg pins the silent baseline for a
// platform-state field flowing into a `!`-required slot: the
// field contributes no proof, so the caller-side check stays
// silent even though Phone is a *string that could be nil at
// runtime.
func callWithPlatformFieldArg() {
	var c nilDeclXPkgCallee.Customer
	nilDeclXPkgCallee.Consume(c.Phone, c.Phone)
}

// callStrictReceiverWithNil pins the cross-package receiver
// strict branch: the callee's signatureNilFact carries the
// Recv field with the `!.()` token, so a statically-nil
// receiver at the call site surfaces the strict diagnostic
// driven by the imported fact rather than by the callee's AST.
func callStrictReceiverWithNil() {
	(*nilDeclXPkgCallee.Service)(nil).RecvNonNil() // want `vow\[nil-safety\]: receiver of RecvNonNil is nil; callee declared receiver as non-nil via vow:nil`
}

// callNillableReceiverWithNil pins the silent baseline for a
// cross-package `?.()` recv decl: the contract explicitly
// admits a nil receiver, so the call stays silent.
func callNillableReceiverWithNil() {
	(*nilDeclXPkgCallee.Service)(nil).RecvNillable()
}

// callInterfaceMethodWithNil pins the cross-package interface-
// method surface: the callee interface's Do method carries a
// signature-mirror marker exported as a signatureNilFact at the
// method's Object, and TypesInfo.ObjectOf resolves an interface-
// typed call site's selector to that same Object, so the imported
// fact drives the diagnostic without the interface's AST in scope.
func callInterfaceMethodWithNil(d nilDeclXPkgCallee.Doer) {
	d.Do(nil) // want `vow\[nil-safety\]: argument 1 \(p\) is nil`
}

// assignNilToXPkgNonNilField pins the cross-package write side: the
// Customer.ID field is `!`-declared in the callee's package, so a
// literal nil written at that field surfaces the field-assign
// diagnostic driven by the imported fieldNilFact.
func assignNilToXPkgNonNilField(c *nilDeclXPkgCallee.Customer) {
	c.ID = nil    // want `vow\[nil-safety\]: field ID assigned nil; vow:nil declared ! on this field`
	c.Email = nil // the `?` decl admits nil, so the write stays silent.
}

// literalNilAtXPkgNonNilField reaches the same imported fact through
// a composite literal's keyed element.
func literalNilAtXPkgNonNilField() nilDeclXPkgCallee.Customer {
	return nilDeclXPkgCallee.Customer{ID: nil} // want `vow\[nil-safety\]: field ID assigned nil; vow:nil declared ! on this field`
}

// callWithNillableGenericFieldArg pins the cross-package field trace
// through a generic instantiation: GenBox.Maybe is `?`-declared in the
// callee's package, and the field access here resolves to the Object
// the `GenBox[int]` instantiation substituted rather than to the one
// the fact is keyed by. Reading the declaration therefore depends on
// resolving the substituted field back to its generic origin, and
// without that step this call passes unreported.
//
// callWithNillableFieldArg above is the control for this case: it
// exercises the same imported-fact path on a non-generic field, so a
// failure there separates "the fact trace is broken" from "the origin
// resolution is broken".
func callWithNillableGenericFieldArg(g nilDeclXPkgCallee.GenBox[int], q *int) {
	nilDeclXPkgCallee.RequireNonNil(g.Maybe, q) // want `vow\[nil-safety\]: argument 1 \(p\) is potentially nil \(field Maybe declared nillable via vow:nil\)`
}

// returnXPkgNillableFieldIntoNonNil reads a `?`-declared field of the
// callee's package into a `!` return. The flow resolver reads the
// declaration through the same imported fact the caller-side argument
// check reads, so the package boundary costs the contract nothing.
//
// vow:nil () !
func returnXPkgNillableFieldIntoNonNil(c *nilDeclXPkgCallee.Customer) *string { // want returnXPkgNillableFieldIntoNonNil:"nilDecl\\(\\) !"
	return c.Email // want `vow\[nil-safety\]: return position 1 may be nil through flow; vow:nil declared ! at this return slot`
}

// returnXPkgNonNilFieldIntoNonNil is the control on the same channel.
//
// vow:nil () !
func returnXPkgNonNilFieldIntoNonNil(c *nilDeclXPkgCallee.Customer) *string { // want returnXPkgNonNilFieldIntoNonNil:"nilDecl\\(\\) !"
	return c.ID
}
