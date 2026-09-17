// Package nilDeclFieldAssignFlow pins the flow layer of the write side
// of a `!` field declaration. The literal layer reads a nil written in
// the source; this one reads a value whose nil-ness comes from a
// declaration elsewhere — a `?`-declared field, a `?`-declared
// parameter, an alias of either. A guard that proves the value non-nil
// where the write runs discharges the site, and a write the guard
// matcher cannot be anchored at goes unreported rather than reported
// with no way out.
package nilDeclFieldAssignFlow

// Box carries the non-nil field the writes target and a nillable one to
// source values from.
type Box struct {
	// vow:nil !
	Ref *int // want Ref:"fieldNil\\(!\\)"

	// vow:nil ?
	Maybe *int // want Maybe:"fieldNil\\(\\?\\)"

	// Plain has no marker, so a nillable value written here carries no
	// contradiction.
	Plain *int
}

// mutate receives the whole Box, so it may write the field through the
// pointer it is handed.
func mutate(b *Box) {}

// ---------- reported ----------

// fieldToField writes a `?`-declared field read into the `!` field.
func fieldToField(p *Box, src *Box) {
	p.Ref = src.Maybe // want `vow\[nil-safety\]: field Ref may be assigned nil through flow; vow:nil declared ! on this field`
}

// nillableParam writes a `?`-declared parameter into the `!` field.
//
// vow:nil (,?)
func nillableParam(p *Box, q *int) { // want nillableParam:"nilDecl\\(,\\?\\)"
	p.Ref = q // want `vow\[nil-safety\]: field Ref may be assigned nil through flow; vow:nil declared ! on this field`
}

// aliasOfField writes an alias of a `?`-declared field read.
func aliasOfField(p *Box, src *Box) {
	m := src.Maybe
	p.Ref = m // want `vow\[nil-safety\]: field Ref may be assigned nil through flow; vow:nil declared ! on this field`
}

// literalFromField supplies the `!` field from a `?`-declared field read
// inside a composite literal.
func literalFromField(src *Box) Box {
	return Box{Ref: src.Maybe} // want `vow\[nil-safety\]: field Ref may be assigned nil through flow; vow:nil declared ! on this field`
}

// guardedThenWrittenAgain pins that the proof ends where the field can
// change: the guard holds, then the struct goes to a callee that may
// write it, so the second write is not covered by the first guard.
func guardedThenWrittenAgain(p *Box, src *Box) {
	if src.Maybe != nil {
		mutate(src)
		p.Ref = src.Maybe // want `vow\[nil-safety\]: field Ref may be assigned nil through flow; vow:nil declared ! on this field`
	}
}

// guardOnOtherField guards a different field, which proves nothing about
// the one being read.
func guardOnOtherField(p *Box, src *Box) {
	if src.Plain != nil {
		p.Ref = src.Maybe // want `vow\[nil-safety\]: field Ref may be assigned nil through flow; vow:nil declared ! on this field`
	}
}

// ---------- discharged by a guard ----------

// guardedFieldToField proves the source field non-nil before the write.
func guardedFieldToField(p *Box, src *Box) {
	if src.Maybe != nil {
		p.Ref = src.Maybe
	}
}

// earlyReturnGuard discharges the source field with a short-circuiting
// guard.
func earlyReturnGuard(p *Box, src *Box) {
	if src.Maybe == nil {
		return
	}
	p.Ref = src.Maybe
}

// guardedNillableParam proves the parameter non-nil before the write.
//
// The silence here is not sufficient on its own: an anchoring failure
// produces the same silence, because a write the matcher cannot anchor is
// not reported either. What pins the anchoring is nillableParam and
// aliasOfField above — break the store lookup and those two lose their
// diagnostics while this case reads unchanged.
//
// vow:nil (,?)
func guardedNillableParam(p *Box, q *int) { // want guardedNillableParam:"nilDecl\\(,\\?\\)"
	if q != nil {
		p.Ref = q
	}
}

// ---------- silent for other reasons ----------

// writeToNillableField writes a nillable value at the `?` field, which
// the declaration admits.
func writeToNillableField(p *Box, src *Box) {
	p.Maybe = src.Maybe
}

// writeToPlatformField writes at an unmarked field, which claims
// nothing.
func writeToPlatformField(p *Box, src *Box) {
	p.Plain = src.Maybe
}

// writeNonNilValue writes a value the resolver proves non-nil.
func writeNonNilValue(p *Box) {
	x := 1
	p.Ref = &x
}

// writeCallResult writes a call result. The resolver does not classify a
// call result, so this layer says nothing about it — reading a callee's
// return declaration is its own change.
func writeCallResult(p *Box) {
	p.Ref = mightBeNil()
}

// mightBeNil returns a nil pointer without declaring a contract.
func mightBeNil() *int { return nil }

// writeLiteralNil is the literal layer's case, not this one's; it must
// carry exactly one diagnostic rather than one from each layer.
func writeLiteralNil(p *Box) {
	p.Ref = nil // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}

// ---------- the boundary the anchoring rule draws ----------

// identInLiteralIsNotAnchored pins where reporting stops. An identifier
// right-hand side is narrowed by value identity, and the block to ask a
// guard about comes from the store the assignment compiles to. A
// composite-literal element performs no such store, so no guard could be
// matched against it — and by the rule that a write is reported only
// where a guard could have discharged it, this one is not reported.
//
// nillableParam above is the same value in the assignment form and is
// reported, so the pair shows that the boundary is the write's shape and
// not the value's.
//
// vow:nil (?)
func identInLiteralIsNotAnchored(q *int) Box { // want identInLiteralIsNotAnchored:"nilDecl\\(\\?\\)"
	return Box{Ref: q}
}

// fieldInLiteralIsAnchored contrasts with it: a field read is narrowed at
// its own location, which exists whether or not a store follows, so the
// literal form is reported for a field source.
func fieldInLiteralIsAnchored(src *Box) Box {
	return Box{Ref: src.Maybe} // want `vow\[nil-safety\]: field Ref may be assigned nil through flow; vow:nil declared ! on this field`
}
