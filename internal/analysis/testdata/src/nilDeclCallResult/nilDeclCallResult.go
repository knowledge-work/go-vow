// Package nilDeclCallResult pins the nullness a call's result carries by
// declaration. A callee whose `vow:nil` signature pins its single return
// slot as `?` hands back a value the contract says may be nil, and every
// check that reads a `!`-required position reports it. A callee with no
// declaration is at platform and says nothing, so its result is not read
// as possibly-nil.
package nilDeclCallResult

// Box carries the non-nil field the write side targets.
type Box struct {
	// vow:nil !
	Ref *int // want Ref:"fieldNil\\(!\\)"
}

// declaredNillable pins its one return slot as nillable.
//
// vow:nil () ?
func declaredNillable() *int { return nil } // want declaredNillable:"nilDecl\\(\\) \\?"

// declaredNonNil pins its return slot as non-nil, which is a proof
// rather than an admission.
//
// vow:nil () !
func declaredNonNil() *int { // want declaredNonNil:"nilDecl\\(\\) !"
	x := 1
	return &x
}

// undeclared carries no signature, so its result stays at platform.
func undeclared() *int { return nil }

// twoResults returns more than one value, which no per-slot mapping
// reaches at a single-value use site.
//
// vow:nil () ?,?
func twoResults() (*int, *int) { return nil, nil } // want twoResults:"nilDecl\\(\\) \\?,\\?"

// consume pins its parameter as non-nil.
//
// vow:nil (!)
func consume(p *int) {} // want consume:"nilDecl\\(!\\)"

// ---------- argument position ----------

// argFromDeclared hands a declared-nillable result to a `!` parameter.
func argFromDeclared() {
	consume(declaredNillable()) // want `vow\[nil-safety\]: argument 1 \(p\) may be nil through flow`
}

// argFromDeclaredNonNil hands a declared-non-nil result to the same
// parameter, which the contract proves.
func argFromDeclaredNonNil() {
	consume(declaredNonNil())
}

// argFromUndeclared hands an undeclared result over. The absence of a
// declaration is not an admission that the value may be nil.
func argFromUndeclared() {
	consume(undeclared())
}

// argFromAliasOfDeclared binds the result first, which carries the same
// declaration through the alias.
func argFromAliasOfDeclared() {
	r := declaredNillable()
	consume(r) // want `vow\[nil-safety\]: argument 1 \(p\) may be nil through flow`
}

// argFromGuardedAlias is the remedy the declaration asks for: bind the
// result, prove it non-nil, then use it.
func argFromGuardedAlias() {
	r := declaredNillable()
	if r == nil {
		return
	}
	consume(r)
}

// ---------- return position ----------

// returnDeclaredThroughNonNilSlot returns a declared-nillable result at a
// slot its own signature pins as non-nil.
//
// vow:nil () !
func returnDeclaredThroughNonNilSlot() *int { // want returnDeclaredThroughNonNilSlot:"nilDecl\\(\\) !"
	return declaredNillable() // want `vow\[nil-safety\]: return position 1 may be nil through flow`
}

// returnUndeclaredThroughNonNilSlot returns an undeclared result at the
// same slot, which stays silent.
//
// vow:nil () !
func returnUndeclaredThroughNonNilSlot() *int { // want returnUndeclaredThroughNonNilSlot:"nilDecl\\(\\) !"
	return undeclared()
}

// ---------- field write ----------

// writeDeclaredToNonNilField stores a declared-nillable result at a `!`
// field.
func writeDeclaredToNonNilField(p *Box) {
	p.Ref = declaredNillable() // want `vow\[nil-safety\]: field Ref may be assigned nil through flow; vow:nil declared ! on this field`
}

// writeUndeclaredToNonNilField stores an undeclared result there, which
// carries no admission.
func writeUndeclaredToNonNilField(p *Box) {
	p.Ref = undeclared()
}

// writeGuardedResult binds the result and proves it before the write, so
// the guard discharges the site through the stored value.
func writeGuardedResult(p *Box) {
	r := declaredNillable()
	if r != nil {
		p.Ref = r
	}
}

// ---------- shapes the classification declines ----------

// multiResultCallIsNotRead pins that a multi-result callee is not read
// even with `?` at both slots: reaching a use site through a spread needs
// a per-slot mapping the surrounding checks do not perform, so attaching
// a nullness here would name the wrong slot's contract.
func multiResultCallIsNotRead() {
	a, b := twoResults()
	consume(a)
	consume(b)
}

// literalFromDeclaredIsNotAnchored pins the boundary the anchoring rule
// draws: a composite-literal element performs no store, so no guard can
// be matched against it and the write goes unreported. The assignment
// form above is reported, so the pair shows the boundary is the write's
// shape rather than the value's.
func literalFromDeclaredIsNotAnchored() Box {
	return Box{Ref: declaredNillable()}
}
