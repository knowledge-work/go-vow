// Package nilDeclNestGenericNarrow pins the path that translates a
// `vow:nil` nest decl on a generic-instantiation
// parameter into the nilness of a read through the i-th type
// argument. The field's declared type must reach the container's
// i-th type parameter through any number of pointers, and the token
// run at that position states the layers of what a read yields: its
// first token answers for the read itself, and each token after it for
// one dereference further in. A layer whose type cannot hold nil falls
// through silently so the flow walker stays sound.
//
// The cases from nestOnlyFieldLeavesTheLayerToTheContainer on pin the
// other question a read raises: when the field carries a decl of its
// own, which of the two decls answers.
package nilDeclNestGenericNarrow

// Value is the demo element type. It holds nil nowhere itself, so the
// layers in this package are the pointers around it.
type Value struct{}

// Box is the demo single-type-parameter container.
type Box[T any] struct{ V T }

// PointerBox carries a `*T` field rather than a bare `T`, so a read
// of the field is one layer further out than the type argument.
type PointerBox[T any] struct{ V *T }

// returnNarrowNonNilSilent reads `box.V` and returns it; the
// `vow:nil` `InnerDecls[0]` is `!`, the type argument `*Value` is a
// nullable kind, so the narrow path proves `box.V` non-nil and the
// return slot's `!` declaration stays silent.
//
// vow:nil ([!]) !
func returnNarrowNonNilSilent(box *Box[*Value]) *Value { // want returnNarrowNonNilSilent:"nilDecl\\(\\) !"
	return box.V
}

// returnNarrowNillableReports reads `box.V` and returns it; the
// `vow:nil` `InnerDecls[0]` is `?`, so the narrow path classifies
// `box.V` as nillable while the return slot's `!` declaration
// demands non-nil — the flow walker reports the mismatch.
//
// vow:nil ([?]) !
func returnNarrowNillableReports(box *Box[*Value]) *Value { // want returnNarrowNillableReports:"nilDecl\\(\\) !"
	return box.V // want `vow\[nil-safety\]: return position 1 may be nil through flow; vow:nil declared ! at this return slot`
}

// returnPointerWrappedFieldReports reads a `*T` field, which the run's
// first token answers for: `box.V` is the read itself, whatever the
// type argument below it is. The gate on whether to answer at all
// reads the type at the layer — `*Value` here — and not the type
// argument, which is why this reports where returnNonNullableSilent
// below stays silent.
//
// vow:nil ([?]) !
func returnPointerWrappedFieldReports(box *PointerBox[Value]) *Value { // want returnPointerWrappedFieldReports:"nilDecl\\(\\) !"
	return box.V // want `vow\[nil-safety\]: return position 1 may be nil through flow; vow:nil declared ! at this return slot`
}

// returnPointerWrappedFieldNonNilSilent is the control on the same
// field: the first token pins the read non-nil, so it satisfies the
// return.
//
// vow:nil ([!]) !
func returnPointerWrappedFieldNonNilSilent(box *PointerBox[Value]) *Value { // want returnPointerWrappedFieldNonNilSilent:"nilDecl\\(\\) !"
	return box.V
}

// derefReadsTheSecondToken pins the layer the second token names: the
// run leaves `box.V` nillable and its dereference non-nil, so the
// dereference satisfies a `!` return.
//
// vow:nil ([?!]) !
func derefReadsTheSecondToken(box *PointerBox[*Value]) *Value { // want derefReadsTheSecondToken:"nilDecl\\(\\) !"
	return *box.V
}

// theOuterLayerKeepsItsOwnToken is the case that must not borrow from
// the layer below it. The same run leaves `box.V` nillable, so reading
// the field itself into a `!` return reports even though the token
// below says non-nil. A narrow that answered for `box.V` from the
// second token would send a reader through a pointer this decl admits
// can be nil.
//
// The return names two tokens because `**Value` holds nil at two
// layers and a run shorter than its type draws a diagnostic of its
// own; the flow check reads the outermost of them.
//
// vow:nil ([?!]) !!
func theOuterLayerKeepsItsOwnToken(box *PointerBox[*Value]) **Value { // want theOuterLayerKeepsItsOwnToken:"nilDecl\\(\\) !!"
	return box.V // want `vow\[nil-safety\]: return position 1 may be nil through flow; vow:nil declared ! at this return slot`
}

// derefBeyondTheRunSilent pins the run's end: with no third token, the
// layer two dereferences in carries no declaration and the read of it
// satisfies nothing and violates nothing. The run is short of the three
// layers `***Value` carries, which the layer check reports on its own.
//
// vow:nil ([?!]) !
func derefBeyondTheRunSilent(box *PointerBox[**Value]) *Value { // want derefBeyondTheRunSilent:"nilDecl\\(\\) !" `vow\[nil-safety\]: vow:nil param position 0 type argument 0 declares 2 nullness layers but \*\*\*Value has 3 layers that can hold nil`
	return **box.V
}

// returnNonNullableSilent reads `box.V` where the type argument is
// `int` — a non-nullable Go kind. The narrow path skips the
// `InnerDecls[0]` lookup because the kind taxonomy excludes the
// layer, so no flow diagnostic fires even though the inner qualifier
// is `?`. The declaration names a layer at two positions the type
// cannot hold nil at, which the layer check reports on its own.
//
// vow:nil ([?]) !
func returnNonNullableSilent(box *Box[int]) int { // want returnNonNullableSilent:"nilDecl\\(\\) !" `vow\[nil-safety\]: vow:nil param position 0 type argument 0 declares 1 nullness layer but int has none that can hold nil` `vow\[nil-safety\]: vow:nil return position 0 declares 1 nullness layer but int has none that can hold nil`
	return box.V
}

// NestMarkedBox marks its field with a nest body and no token of its
// own. The marker states the element layer of whatever the field
// holds and states nothing about the field itself. It states `!`
// there, so a read answered from this marker by mistake would come
// back non-nil and stay silent.
type NestMarkedBox[T any] struct {
	// vow:nil []!
	V T // want V:"fieldNil\\(\\)"
}

// TokenMarkedBox is the same container under a decl that does state
// the field.
type TokenMarkedBox[T any] struct {
	// vow:nil !
	V T // want V:"fieldNil\\(!\\)"
}

// NillableTokenBox states the field too, at the other nullness.
type NillableTokenBox[T any] struct {
	// vow:nil ?
	V T // want V:"fieldNil\\(\\?\\)"
}

// consumeSlice pins its parameter so a read can ride into it.
//
// vow:nil (!)
func consumeSlice(items []*Value) {} // want consumeSlice:"nilDecl\\(!\\)"

// nestOnlyFieldLeavesTheLayerToTheContainer pins which decl answers a
// direct read: the field's own decl states nothing about the field,
// so the container's nest decl does, and its `?` reaches the `!`
// return. The two decls disagree on purpose — the field's marker says
// `!` one layer down — so the report can only have come from the
// container. The layer check stays out of it: a read of the field
// yields `[]*Value`, one layer, which the one token names.
//
// vow:nil ([?]) !
func nestOnlyFieldLeavesTheLayerToTheContainer(box *NestMarkedBox[[]*Value]) []*Value { // want nestOnlyFieldLeavesTheLayerToTheContainer:"nilDecl\\(\\) !"
	return box.V // want `vow\[nil-safety\]: return position 1 may be nil through flow; vow:nil declared ! at this return slot`
}

// nestOnlyFieldAtACallPosition pins the same handover where the read
// rides into a parameter rather than a return.
//
// vow:nil ([?])
func nestOnlyFieldAtACallPosition(box *NestMarkedBox[[]*Value]) { // want nestOnlyFieldAtACallPosition:"nilDecl\\(\\)"
	consumeSlice(box.V) // want `vow\[nil-safety\]: argument 1 \(items\) may be nil through flow; vow:nil declared ! at this position`
}

// fieldTokenAnswersOverTheContainer is the control on the other side,
// differing in the field's decl alone: that decl states the layer,
// so it answers instead of the container, and what it states is `!`,
// which the `!` return accepts.
//
// vow:nil ([?]) !
func fieldTokenAnswersOverTheContainer(box *TokenMarkedBox[[]*Value]) []*Value { // want fieldTokenAnswersOverTheContainer:"nilDecl\\(\\) !"
	return box.V
}

// fieldTokenAnswersEvenAgainstANonNilContainer is what makes the
// silence above mean the field answered rather than nothing did: the
// nullnesses are swapped, so the field's `?` is what reaches the `!`
// return while the container says `!`.
//
// vow:nil ([!]) !
func fieldTokenAnswersEvenAgainstANonNilContainer(box *NillableTokenBox[[]*Value]) []*Value { // want fieldTokenAnswersEvenAgainstANonNilContainer:"nilDecl\\(\\) !"
	return box.V // want `vow\[nil-safety\]: return position 1 may be nil through flow; vow:nil declared ! at this return slot`
}
