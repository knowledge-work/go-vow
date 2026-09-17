// Package nilDeclFieldAssign pins the write side of the struct-field
// vow:nil surface: a field declared `!` never holds nil,so an
// assignment or a composite-literal element that supplies a literal
// nil at that field contradicts the declaration. Fields declared `?`
// and fields left at platform accept a nil write silently — the
// declaration made no non-nil claim about them.
package nilDeclFieldAssign

// Box carries one field per nullness state so each state's write
// behaviour is pinned independently.
type Box struct {
	// vow:nil !
	Ref *int // want Ref:"fieldNil\\(!\\)"

	// vow:nil ?
	Maybe *int // want Maybe:"fieldNil\\(\\?\\)"

	// Plain has no marker so the field stays at platform and a nil
	// write carries no contradiction to report.
	Plain *int
}

// Nested holds a Box so a write reached through a chained selector
// resolves to the same field Object a direct write resolves to.
type Nested struct {
	Inner Box
}

// assignThroughPointer writes nil through a pointer receiver.
func assignThroughPointer(b *Box) {
	b.Ref = nil // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}

// assignThroughValue writes nil on a local value receiver.
func assignThroughValue() {
	var b Box
	b.Ref = nil // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}

// assignThroughMethod writes nil from a method body, where the
// receiver rather than a parameter carries the struct.
func (b *Box) assignThroughMethod() {
	b.Ref = nil // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}

// assignChainedSelector writes nil through a two-hop selector so the
// recogniser is pinned as selector-shape-driven rather than
// dependent on the receiver being a bare identifier.
func assignChainedSelector(n *Nested) {
	n.Inner.Ref = nil // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}

// assignTypedNilConversion writes a typed-nil conversion, which
// carries the same literal proof of nil-ness the bare identifier
// carries.
func assignTypedNilConversion(b *Box) {
	b.Ref = (*int)(nil) // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}

// assignTuple writes nil at one position of a multi-assignment; the
// index-aligned right-hand side decides each position on its own.
func assignTuple(b *Box, x *int) {
	b.Maybe, b.Ref = x, nil // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}

// assignSilentStates writes nil at every position the declaration
// left unconstrained: the `?`-declared field admits nil by contract
// and the platform field carries no claim at all.
func assignSilentStates(b *Box) {
	b.Maybe = nil
	b.Plain = nil
}

// assignNonNilValue writes a non-nil value at the `!` field, which
// is exactly what the declaration asks for.
func assignNonNilValue(b *Box, x *int) {
	b.Ref = x
}

// assignFromCallResult writes a call result whose nil-ness needs
// dataflow to decide. The literal-only judgement stays silent here.
func assignFromCallResult(b *Box) {
	b.Ref = mightBeNil()
}

// mightBeNil returns a nil pointer without declaring a contract, so
// the literal-only judgement cannot see through the call.
func mightBeNil() *int { return nil }

// literalKeyedNil names the `!` field with nil in a keyed composite
// literal.
func literalKeyedNil() Box {
	return Box{Ref: nil} // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}

// literalPositionalNil supplies nil at the `!` field's position in a
// positional composite literal.
func literalPositionalNil() Box {
	return Box{nil, nil, nil} // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}

// literalAddressOf reaches the same write through a pointer-typed
// composite literal.
func literalAddressOf() *Box {
	return &Box{Ref: nil} // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}

// literalElidedType writes nil at the `!` field of an elided inner
// literal, whose type the outer slice's element type supplies.
func literalElidedType() []*Box {
	return []*Box{{Ref: nil}} // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}

// literalNestedStruct writes nil at the `!` field of a Box nested
// inside another struct's literal.
func literalNestedStruct() Nested {
	return Nested{Inner: Box{Ref: nil}} // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}

// literalSilentStates supplies nil at the `?` field and the platform
// field, which the declaration admits. The `!` field is supplied so
// the construction obligation stays out of the way and the write
// judgement is the only one on trial.
func literalSilentStates(x *int) Box {
	return Box{Ref: x, Maybe: nil, Plain: nil}
}

// literalMapValue pins that a map literal's own elements carry no
// field write; the Box literals inside it are visited on their own
// and only the `!` field's nil surfaces a diagnostic.
func literalMapValue(x *int) map[string]Box {
	return map[string]Box{
		"a": {Ref: nil}, // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
		"b": {Ref: x, Maybe: nil},
	}
}

// Wrapper embeds Box so Box's fields are promoted onto it.
type Wrapper struct {
	Box
}

// assignPromotedField writes nil through a promoted selector. The
// selector resolves to the field Object Box declares, so the
// promotion carries the declaration with it.
func assignPromotedField(w *Wrapper) {
	w.Ref = nil // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}

// literalEmbeddedNil reaches the same field through the embedded
// struct's own literal.
func literalEmbeddedNil() Wrapper {
	return Wrapper{Box{Ref: nil}} // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}
