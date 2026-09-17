// Package nilDeclLayer is the analysistest fixture for the layer
// check the analyzer applies to a `vow:nil` decl. A decl names one
// nullness token per layer of the value it describes, outermost
// first, and a type holds nil at one layer per dereference a value
// admits: `int` at none, `*Value` at one, `*Error` at the pointer and
// at the interface it addresses. A token past the last such layer
// constrains nothing, so the check reports it at the declaration.
//
// The count is read from the Go type. An unsubstituted type parameter
// settles it only through its constraint, so a parameter an
// instantiation could still bind to a nillable type stays silent.
package nilDeclLayer

// Key is the demo map-key type.
type Key struct{}

// Value is the demo element type.
type Value struct{}

// Error is the demo interface type: an interface holds nil at its own
// layer, and a pointer to one holds nil at two.
type Error interface{ Error() string }

// Integer is the demo named constraint whose type set names
// non-nillable terms only.
type Integer interface{ ~int | ~int64 }

// Box holds its type argument directly.
type Box[T any] struct{ V T }

// PointerBox wraps its type argument in a pointer, so the value a
// field read yields holds nil at one layer more than the type
// argument does.
type PointerBox[T any] struct{ V *T }

// SliceBox stores its type argument behind a shape the walk cannot
// attribute to a single layer count.
type SliceBox[T any] struct{ S []T }

// MixedBox reaches one type parameter through two fields of different
// wrapping depth, so the layer count depends on which field the walk
// reads.
type MixedBox[T any] struct {
	Bare    T
	Wrapped *T
}

// Pair carries two type parameters whose instantiations hold nil at
// different depths, so an inner decl read off the wrong index names
// the wrong type.
type Pair[A, B any] struct {
	First  A
	Second B
}

// SelfPtr reaches itself through a pointer, which the Go spec admits
// because the pointer breaks the size recursion.
type SelfPtr *SelfPtr

// MutualA and MutualB are the two-step form of the same recursion.
type MutualA *MutualB

// MutualB closes the cycle MutualA opens.
type MutualB *MutualA

// SelfGeneric closes the same cycle through a type parameter. Go
// admits it because every turn returns to the same instantiation; the
// expanding form that would name a fresh one each turn
// (`type Loop[T any] *Loop[*T]`) is an instantiation cycle the type
// checker rejects.
type SelfGeneric[T any] *SelfGeneric[T]

// ---- the position's own layers ----

// intParam declares a layer on a parameter that has none.
//
// vow:nil (!)
func intParam(n int) {} // want intParam:"nilDecl\\(!\\)" `vow\[nil-safety\]: vow:nil param position 0 declares 1 nullness layer but int has none that can hold nil`

// pointerParam is the control: one token over one layer.
//
// vow:nil (!)
func pointerParam(v *Value) {} // want pointerParam:"nilDecl\\(!\\)"

// twoOverOne declares two layers on a single-pointer type.
//
// vow:nil (!?)
func twoOverOne(v *Value) {} // want twoOverOne:"nilDecl\\(!\\?\\)" `vow\[nil-safety\]: vow:nil param position 0 declares 2 nullness layers but \*Value has 1 layer that can hold nil`

// twoOverTwo is the control: a pointer to a pointer carries both.
//
// vow:nil (!?)
func twoOverTwo(v **Value) {} // want twoOverTwo:"nilDecl\\(!\\?\\)"

// pointerToInterface is the control for the interface layer: the
// pointer and the interface it addresses each hold nil.
//
// vow:nil (!?)
func pointerToInterface(e *Error) {} // want pointerToInterface:"nilDecl\\(!\\?\\)"

// sliceStopsTheWalk declares a second layer on a slice. No
// dereference reaches past a slice, so the element layer is the nest
// body's business rather than a second token's.
//
// vow:nil (??)
func sliceStopsTheWalk(items []*Value) {} // want sliceStopsTheWalk:"nilDecl\\(\\?\\?\\)" `vow\[nil-safety\]: vow:nil param position 0 declares 2 nullness layers but \[\]\*Value has 1 layer that can hold nil`

// structReturn declares a layer on a returned struct value.
//
// vow:nil () ?
func structReturn() Value { return Value{} } // want structReturn:"nilDecl\\(\\) \\?" `vow\[nil-safety\]: vow:nil return position 0 declares 1 nullness layer but Value has none that can hold nil`

// valueReceiver declares a layer on a receiver taken by value.
//
// vow:nil !.()
func (Value) valueReceiver() {} // want valueReceiver:"nilDecl!\\.\\(\\)" `vow\[nil-safety\]: vow:nil recv position 0 declares 1 nullness layer but Value has none that can hold nil`

// pointerReceiver is the control on the receiver axis.
//
// vow:nil !.()
func (*Value) pointerReceiver() {} // want pointerReceiver:"nilDecl!\\.\\(\\)"

// recursivePointerSilent pins the endless chain: a value of SelfPtr
// can be dereferenced forever, so no run of tokens outgrows it.
//
// vow:nil (!!!)
func recursivePointerSilent(p SelfPtr) {} // want recursivePointerSilent:"nilDecl\\(!!!\\)"

// mutualPointerSilent pins the same answer for the two-step cycle.
//
// vow:nil (!!!)
func mutualPointerSilent(p MutualA) {} // want mutualPointerSilent:"nilDecl\\(!!!\\)"

// selfGenericSilent pins the guard against the generic form of the
// cycle, where the type the walk returns to arrives through an
// instantiation rather than a plain name.
//
// vow:nil (!!!)
func selfGenericSilent(g SelfGeneric[int]) {} // want selfGenericSilent:"nilDecl\\(!!!\\)"

// recursivePointerControl is the firing sibling for the two above: the
// same run over a chain that ends.
//
// vow:nil (!!!)
func recursivePointerControl(p **Value) {} // want recursivePointerControl:"nilDecl\\(!!!\\)" `vow\[nil-safety\]: vow:nil param position 0 declares 3 nullness layers but \*\*Value has 2 layers that can hold nil`

// ---- type parameters ----

// unboundedParam leaves the count open: `any` admits a nillable
// instantiation.
//
// vow:nil (!)
func unboundedParam[T any](v T) {} // want unboundedParam:"nilDecl\\(!\\)"

// comparableParam leaves the count open too: the comparable type set
// includes pointers.
//
// vow:nil (!)
func comparableParam[T comparable](v T) {} // want comparableParam:"nilDecl\\(!\\)"

// unionParam settles the count: every term of the union names a kind
// that cannot hold nil.
//
// vow:nil (!)
func unionParam[T ~int | ~int64](v T) {} // want unionParam:"nilDecl\\(!\\)" `vow\[nil-safety\]: vow:nil param position 0 declares 1 nullness layer but T has none that can hold nil`

// mixedUnionParam leaves the count open: one term of the union names
// a pointer.
//
// vow:nil (!)
func mixedUnionParam[T ~int | *Value](v T) {} // want mixedUnionParam:"nilDecl\\(!\\)"

// namedConstraintParam settles the count through a named constraint's
// embedded union.
//
// vow:nil (!)
func namedConstraintParam[T Integer](v T) {} // want namedConstraintParam:"nilDecl\\(!\\)" `vow\[nil-safety\]: vow:nil param position 0 declares 1 nullness layer but T has none that can hold nil`

// ---- the layers a nest body reaches ----

// sliceElement declares a layer on the element of a slice of ints.
//
// vow:nil (![]!)
func sliceElement(items []int) {} // want sliceElement:"nilDecl\\(!\\)" `vow\[nil-safety\]: vow:nil param position 0 element declares 1 nullness layer but int has none that can hold nil`

// sliceElementOK is the control on the element layer.
//
// vow:nil (![]!)
func sliceElementOK(items []*Value) {} // want sliceElementOK:"nilDecl\\(!\\)"

// mapKey declares a layer on a key that has none.
//
// vow:nil ([!])
func mapKey(idx map[string]*Value) {} // want mapKey:"nilDecl\\(\\)" `vow\[nil-safety\]: vow:nil param position 0 map key declares 1 nullness layer but string has none that can hold nil`

// mapValue declares a layer on an int value layer; the key layer is
// the control alongside it.
//
// vow:nil ([!]!)
func mapValue(idx map[*Key]int) {} // want mapValue:"nilDecl\\(\\)" `vow\[nil-safety\]: vow:nil param position 0 map value declares 1 nullness layer but int has none that can hold nil`

// typeArgument declares a layer on a type argument that has none.
//
// vow:nil ([!])
func typeArgument(box *Box[int]) {} // want typeArgument:"nilDecl\\(\\)" `vow\[nil-safety\]: vow:nil param position 0 type argument 0 declares 1 nullness layer but int has none that can hold nil`

// typeArgumentOK is the control on the type-argument layer.
//
// vow:nil ([!])
func typeArgumentOK(box *Box[*Value]) {} // want typeArgumentOK:"nilDecl\\(\\)"

// wrappedTypeArgumentOK pins the field wrapper: the value read at the
// type-argument position is `*Value`, so one token fits even though
// the type argument itself holds nil at no layer.
//
// vow:nil ([!])
func wrappedTypeArgumentOK(box *PointerBox[Value]) {} // want wrappedTypeArgumentOK:"nilDecl\\(\\)"

// wrappedTypeArgument declares one layer past the wrapper.
//
// vow:nil ([!!])
func wrappedTypeArgument(box *PointerBox[Value]) {} // want wrappedTypeArgument:"nilDecl\\(\\)" `vow\[nil-safety\]: vow:nil param position 0 type argument 0 declares 2 nullness layers but \*Value has 1 layer that can hold nil`

// wrappedTypeArgumentTwoOK is the control: the wrapper and a pointer
// type argument carry two layers between them.
//
// vow:nil ([!!])
func wrappedTypeArgumentTwoOK(box *PointerBox[*Value]) {} // want wrappedTypeArgumentTwoOK:"nilDecl\\(\\)"

// typeArgumentIndex declares one token at each type-argument
// position of a container whose two instantiations hold nil at
// different depths, so the diagnostic names the second position and
// the type standing there.
//
// vow:nil ([!,!])
func typeArgumentIndex(pair *Pair[*Value, int]) {} // want typeArgumentIndex:"nilDecl\\(\\)" `vow\[nil-safety\]: vow:nil param position 0 type argument 1 declares 1 nullness layer but int has none that can hold nil`

// deepestFieldWins pins the rule for a container that reaches one
// type parameter through two fields: the value read at that position
// is the `*Value` the Wrapped field holds, so one token fits even
// though the Bare field holds a `Value` that has no layer at all.
//
// vow:nil ([!])
func deepestFieldWins(box *MixedBox[Value]) {} // want deepestFieldWins:"nilDecl\\(\\)"

// deepestFieldWinsControl is the firing sibling: a second token
// outgrows the deepest field too.
//
// vow:nil ([!!])
func deepestFieldWinsControl(box *MixedBox[Value]) {} // want deepestFieldWinsControl:"nilDecl\\(\\)" `vow\[nil-safety\]: vow:nil param position 0 type argument 0 declares 2 nullness layers but \*Value has 1 layer that can hold nil`

// unattributableFieldSilent pins the silent path for a container
// whose field routes the type argument through a shape the walk
// cannot attribute, so no count is derived and no token is judged.
//
// vow:nil ([!!])
func unattributableFieldSilent(box *SliceBox[Value]) {} // want unattributableFieldSilent:"nilDecl\\(\\)"

// nestedTypeArgument pins the descent: the check reaches the type
// argument of the container held at a type-argument position.
//
// vow:nil ([[!]])
func nestedTypeArgument(box *Box[*Box[int]]) {} // want nestedTypeArgument:"nilDecl\\(\\)" `vow\[nil-safety\]: vow:nil param position 0 type argument 0 type argument 0 declares 1 nullness layer but int has none that can hold nil`

// ---- runs that stop short of the layers a type carries ----

// List is a named slice, so a pointer to one holds nil at the pointer
// and again at the slice it addresses.
type List []Value

// shortOnDoublePointer names the outer pointer and leaves the one it
// addresses untracked.
//
// vow:nil (?)
func shortOnDoublePointer(x **int) {} // want shortOnDoublePointer:"nilDecl\\(\\?\\)" `vow\[nil-safety\]: vow:nil param position 0 declares 1 nullness layer but \*\*int has 2 layers that can hold nil`

// shortOnPointerToSlice is the same shape through a slice rather than
// a second pointer.
//
// vow:nil (!)
func shortOnPointerToSlice(b *[]byte) {} // want shortOnPointerToSlice:"nilDecl\\(!\\)" `vow\[nil-safety\]: vow:nil param position 0 declares 1 nullness layer but \*\[\]byte has 2 layers that can hold nil`

// shortOnPointerToNamedSlice pins the shape a named slice type hides:
// the name reads as one value and the type holds nil at two layers.
//
// vow:nil () !
func shortOnPointerToNamedSlice() *List { return &List{} } // want shortOnPointerToNamedSlice:"nilDecl\\(\\) !" `vow\[nil-safety\]: vow:nil return position 0 declares 1 nullness layer but \*List has 2 layers that can hold nil`

// shortOnPointerToInterface names the pointer and leaves the interface
// it addresses untracked.
//
// vow:nil (!)
func shortOnPointerToInterface(e *Error) {} // want shortOnPointerToInterface:"nilDecl\\(!\\)" `vow\[nil-safety\]: vow:nil param position 0 declares 1 nullness layer but \*Error has 2 layers that can hold nil`

// shortOnPointerToTypeParam reports on the lower bound alone: the
// count past the type parameter is open, and the two pointers below it
// are there whatever T binds.
//
// vow:nil (!)
func shortOnPointerToTypeParam[T any](x **T) {} // want shortOnPointerToTypeParam:"nilDecl\\(!\\)" `vow\[nil-safety\]: vow:nil param position 0 declares 1 nullness layer but \*\*T has at least 2 layers that can hold nil`

// shortOnMapValue reports at a layer the nest body reaches.
//
// vow:nil ([!]!)
func shortOnMapValue(idx map[*Key]*[]byte) {} // want shortOnMapValue:"nilDecl\\(\\)" `vow\[nil-safety\]: vow:nil param position 0 map value declares 1 nullness layer but \*\[\]byte has 2 layers that can hold nil`

// singleLayerTypeParamOK is the control on the lower-bound path: one
// pointer is one layer, and what T adds below it is open.
//
// vow:nil (!)
func singleLayerTypeParamOK[T any](x *T) {} // want singleLayerTypeParamOK:"nilDecl\\(!\\)"

// platformSlotOK records where the boundary of this check sits rather
// than guarding a path through it. The parser yields a nil decl for an
// empty slot, so the walk returns at its first gate and no change to
// the walk alone turns this position into a report. What the case
// catches is a later decision to fold the demand for an undeclared
// position into this walk: that is the change this position falls to
// first, and the exemption is the one the completeness rule's opt-in
// otherwise carries.
//
// vow:nil (,!)
func platformSlotOK(x **int, v *Value) {} // want platformSlotOK:"nilDecl\\(,!\\)"

// nestOnlyDeclOK guards the second gate, where a decl exists and
// carries no token of its own: the position's layers go unjudged
// while the nest body's tokens are read. Unlike the empty slot above,
// this one falls to a one-line change in the walk.
//
// vow:nil ([?])
func nestOnlyDeclOK(box **Box[*Value]) {} // want nestOnlyDeclOK:"nilDecl\\(\\)"

// ---- the interface-method surface ----

// Reader pins the check on an interface method, whose diagnostics
// anchor at the method element.
type Reader interface {
	// vow:nil (!)
	Read(n int) // want Read:"nilDecl\\(!\\)" `vow\[nil-safety\]: vow:nil param position 0 declares 1 nullness layer but int has none that can hold nil`

	// vow:nil (!)
	ReadPointer(v *Value) // want ReadPointer:"nilDecl\\(!\\)"
}

// ---- the field-inline surface ----

// Counted pins the field surface as out of scope: the `!` stands at a
// layer `int` does not have, and the zero-value escape checks read
// the same token whatever the field's type, so the layer check leaves
// the declaration alone.
type Counted struct {
	// vow:nil !
	Total int // want Total:"fieldNil\\(!\\)"
}
