// Package nilDeclFieldGeneric pins the field-nil surface on a generic
// struct type. The declaration is registered against the field of the
// generic type, while a field access through an instantiation reaches
// a substituted Object; resolving that Object back to its origin is
// what keeps a field-nil check reading the same declaration whether or
// not the container is instantiated.
package nilDeclFieldGeneric

// GenBox declares one field per nullness state on a generic type, so
// each state is pinned through an instantiation independently.
type GenBox[T any] struct {
	// vow:nil !
	Ref *T // want Ref:"fieldNil\\(!\\)"

	// vow:nil ?
	Maybe *T // want Maybe:"fieldNil\\(\\?\\)"

	// Plain carries no marker, so the platform state survives the
	// instantiation as well.
	Plain *T

	// vow:nil ?[]?
	Items []*T // want Items:"fieldNil\\(\\?\\)"
}

// consume pins its parameter as non-nil so a field read has a
// `!`-required position to ride into.
//
// vow:nil (!)
func consume(p *int) {} // want consume:"nilDecl\\(!\\)"

// consumeString is the same contract at a second element type so two
// instantiations of GenBox can be read in one package.
//
// vow:nil (!)
func consumeString(p *string) {} // want consumeString:"nilDecl\\(!\\)"

// nillableFieldThroughInstantiation passes a `?`-declared field of an
// instantiated container into a `!`-required slot. This is the case
// the origin resolution decides: the substituted field Object carries
// no decl of its own, and without resolving it back to the generic
// origin the declaration would go unread and the call would pass
// unreported.
func nillableFieldThroughInstantiation(g *GenBox[int]) {
	consume(g.Maybe) // want `vow\[nil-safety\]: argument 1 \(p\) is potentially nil \(field Maybe declared nillable via vow:nil\)`
}

// nillableFieldOtherInstantiation pins that a second instantiation
// resolves to the same declaration. The substituted Object differs per
// type argument, so this call proves the resolution is not specific to
// one instantiation.
func nillableFieldOtherInstantiation(g *GenBox[string]) {
	consumeString(g.Maybe) // want `vow\[nil-safety\]: argument 1 \(p\) is potentially nil \(field Maybe declared nillable via vow:nil\)`
}

// nonNilFieldThroughInstantiation passes a `!`-declared field into the
// same slot. The construction side owes that field a non-nil member,
// so the read stays silent — and stays silent for that reason rather
// than because the declaration went unread.
func nonNilFieldThroughInstantiation(g *GenBox[int]) {
	consume(g.Ref)
}

// platformFieldThroughInstantiation passes an unmarked field, which
// contributes no proof either way.
func platformFieldThroughInstantiation(g *GenBox[int]) {
	consume(g.Plain)
}

// PlainBox is the non-generic baseline: its field is already the
// Object the declaration was registered against, so it resolves on the
// first lookup and never reaches the origin retry.
type PlainBox struct {
	// vow:nil ?
	Maybe *int // want Maybe:"fieldNil\\(\\?\\)"
}

// nillableFieldOnPlainBox pins that the non-generic path is unchanged.
func nillableFieldOnPlainBox(b *PlainBox) {
	consume(b.Maybe) // want `vow\[nil-safety\]: argument 1 \(p\) is potentially nil \(field Maybe declared nillable via vow:nil\)`
}

// ---------- writing side ----------

// The write checks read the same declaration through the same lookup,
// so the origin resolution serves them too. These pin that leg of it:
// without the resolution the substituted field carries no decl and a
// nil written at a `!` field of an instantiated container passes.

// assignNilThroughInstantiation writes a literal nil at the `!` field
// of an instantiated container.
func assignNilThroughInstantiation(g *GenBox[int]) {
	g.Ref = nil // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}

// assignNilAtNillableField writes nil at the `?` field, which the
// declaration admits — so the resolution is not simply making every
// generic field report.
func assignNilAtNillableField(g *GenBox[int]) {
	g.Maybe = nil
}

// literalNilThroughInstantiation names the `!` field with nil in a
// composite literal over the instantiated type.
func literalNilThroughInstantiation(x *int) GenBox[int] {
	return GenBox[int]{Ref: nil, Maybe: x} // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}

// assignNilOtherInstantiation reaches the same declaration through a
// second instantiation, so the write leg is not specific to one.
func assignNilOtherInstantiation(g *GenBox[string]) {
	g.Ref = nil // want `vow\[nil-safety\]: field Ref assigned nil; vow:nil declared ! on this field`
}

// nillableFieldIntoNonNilReturn reads the same `?`-declared field into
// a `!` return. The flow resolver reaches the declaration through the
// substituted field Object, the same resolution the caller-side check
// above performs, so both surfaces read one declaration.
//
// vow:nil () !
func nillableFieldIntoNonNilReturn(g *GenBox[int]) *int { // want nillableFieldIntoNonNilReturn:"nilDecl\\(\\) !"
	return g.Maybe // want `vow\[nil-safety\]: return position 1 may be nil through flow; vow:nil declared ! at this return slot`
}

// nonNilFieldIntoNonNilReturn is the control on the same channel: the
// `!`-declared field satisfies the return contract.
//
// vow:nil () !
func nonNilFieldIntoNonNilReturn(g *GenBox[int]) *int { // want nonNilFieldIntoNonNilReturn:"nilDecl\\(\\) !"
	return g.Ref
}

// nillableElementIntoNonNilReturn reads the element layer of a nest
// decl carried by a substituted field. The element read resolves the
// container field through the same origin lookup the top-level read
// uses.
//
// vow:nil () !
func nillableElementIntoNonNilReturn(g *GenBox[int]) *int { // want nillableElementIntoNonNilReturn:"nilDecl\\(\\) !"
	return g.Items[0] // want `vow\[nil-safety\]: return position 1 may be nil through flow; vow:nil declared ! at this return slot`
}

// nillableFieldThroughLocalIntoNonNilReturn binds the field read to a
// local first, so the value reaches the return through SSA rather
// than as a selector expression at the return site.
//
// vow:nil () !
func nillableFieldThroughLocalIntoNonNilReturn(g *GenBox[int]) *int { // want nillableFieldThroughLocalIntoNonNilReturn:"nilDecl\\(\\) !"
	v := g.Maybe
	return v // want `vow\[nil-safety\]: return position 1 may be nil through flow; vow:nil declared ! at this return slot`
}

// MarkedBareBox declares a marker on the bare type-parameter field.
// That field is the one the call-side nest narrow reads through, so a
// marker here and a nest decl at the call site describe the same read
// and the two have to be ordered.
type MarkedBareBox[T any] struct {
	// vow:nil !
	Bare T // want Bare:"fieldNil\\(!\\)"
}

// MarkedBareNillableBox is the same shape at the other nullness.
type MarkedBareNillableBox[T any] struct {
	// vow:nil ?
	Bare T // want Bare:"fieldNil\\(\\?\\)"
}

// fieldDeclOverridesNestNillable pins the ordering in the direction
// that withholds a report: the call-side nest calls the layer
// nillable, the container calls the field non-nil, and the field
// declaration decides, so the read satisfies the `!` return.
//
// vow:nil ([?]) !
func fieldDeclOverridesNestNillable(box *MarkedBareBox[*int]) *int { // want fieldDeclOverridesNestNillable:"nilDecl\\(\\) !"
	return box.Bare
}

// fieldDeclOverridesNestNonNil pins the opposite direction: the
// call-side nest calls the layer non-nil, the container admits nil at
// the field, and the field declaration decides again.
//
// vow:nil ([!]) !
func fieldDeclOverridesNestNonNil(box *MarkedBareNillableBox[*int]) *int { // want fieldDeclOverridesNestNonNil:"nilDecl\\(\\) !"
	return box.Bare // want `vow\[nil-safety\]: return position 1 may be nil through flow; vow:nil declared ! at this return slot`
}
