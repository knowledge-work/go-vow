// Package nilDeclFieldConstruct pins the construction obligation a
// `!` field declaration carries. The party that builds a struct owes
// every `!`-declared field a non-nil member, because a reader of that
// field is entitled to skip the nil guard. A literal that omits such
// a field and then escapes the expression that built it surfaces a
// diagnostic; a literal bound to a local stays silent, since the
// field can still be filled before the value goes anywhere.
package nilDeclFieldConstruct

import "strings"

// Box carries one field per nullness state so the obligation is
// pinned against each state independently.
type Box struct {
	// vow:nil !
	Ref *int // want Ref:"fieldNil\\(!\\)"

	// vow:nil ?
	Maybe *int // want Maybe:"fieldNil\\(\\?\\)"

	// Plain has no marker, so the platform state leaves the
	// constructor free to omit it.
	Plain *int
}

// Pair carries two `!` fields so a single literal can owe more than
// one member.
type Pair struct {
	// vow:nil !
	First *int // want First:"fieldNil\\(!\\)"

	// vow:nil !
	Second *int // want Second:"fieldNil\\(!\\)"
}

// Holder holds a Box so a literal reached through another literal's
// element is judged under the same rule as its parent.
type Holder struct {
	Inner Box
}

// Tray holds an array of Box, so a write to one element is judged under
// the rules that reach an array element rather than a struct member.
type Tray struct {
	Boxes [2]Box
}

// consume takes a Box so a literal can escape as a call argument.
func consume(b Box) {}

// sink receives a Box so a literal can escape through a channel send.
var sink = make(chan Box, 1)

// escaped is a package-level var so an assignment to it counts as an
// escape rather than a local binding.
var escaped Box

// ---------- escaping positions ----------

// returnEmpty returns an empty literal, so every `!` field is nil at
// the boundary.
func returnEmpty() Box {
	return Box{} // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// returnPartial fills only the nillable field, leaving the `!` field
// at its zero value.
func returnPartial(x *int) Box {
	return Box{Maybe: x} // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// returnTwoOwed owes both `!` fields, so the literal reports once per
// field in declaration order.
func returnTwoOwed() Pair {
	return Pair{} // want `vow\[nil-safety\]: field First left nil where this value escapes; vow:nil declared ! on this field` `vow\[nil-safety\]: field Second left nil where this value escapes; vow:nil declared ! on this field`
}

// returnAddressOf escapes a pointer to the literal, which carries the
// same omission.
func returnAddressOf() *Box {
	return &Box{} // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// callArgument hands the literal to a callee, which puts it out of
// reach of any later field write.
func callArgument() {
	consume(Box{}) // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// channelSend hands the literal to a receiver.
func channelSend() {
	sink <- Box{} // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// assignToField stores the literal into another struct's field.
func assignToField(h *Holder) {
	h.Inner = Box{} // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// assignToPackageVar stores the literal into a package-level var,
// which this pass cannot follow to a later field write.
func assignToPackageVar() {
	escaped = Box{} // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// declaredPackageVar reaches the same destination as assignToPackageVar
// through the declaration rather than through an assignment. The two
// sit next to each other because the escape is identical and only the
// syntax differs — a rule that reported one and not the other would be
// inconsistent with itself.
var declaredPackageVar = Box{} // want `vow\[nil-safety\]: field Ref left nil by this literal; vow:nil declared ! on this field`

// declaredPackageVarGroup pins the grouped declaration form, where one
// GenDecl carries several specs.
var (
	groupedFirst  = Box{}  // want `vow\[nil-safety\]: field Ref left nil by this literal; vow:nil declared ! on this field`
	groupedSecond = Pair{} // want `vow\[nil-safety\]: field First left nil by this literal; vow:nil declared ! on this field` `vow\[nil-safety\]: field Second left nil by this literal; vow:nil declared ! on this field`
)

// declaredPackageVarBlank discards the value at package scope, so no
// reader can observe the omitted field. This matches the discarded
// case inside a function body — the blank identifier holds the value
// back on both forms.
var _ = Box{}

// assignToDeref stores the literal through a pointer dereference.
func assignToDeref(p *Box) {
	*p = Box{} // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// assignToElement stores the literal into a slice element.
func assignToElement(xs []Box) {
	xs[0] = Box{} // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// nestedLiteral omits the `!` field of a Box built inside an escaping
// Holder literal, so the recursion reaches the inner literal.
func nestedLiteral() Holder {
	return Holder{Inner: Box{}} // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// nestedSupplied supplies the nested `!` field, which is what the
// declaration asks for.
func nestedSupplied(x *int) Holder {
	return Holder{Inner: Box{Ref: x}}
}

// nestedFilledBeforeEscape writes the nested `!` field through the
// local before the value leaves, which discharges the obligation on
// the same terms a top-level field does.
func nestedFilledBeforeEscape(x *int) Holder {
	var h Holder
	h.Inner.Ref = x
	return h
}

// nestedWholeMemberSupplied replaces the whole nested struct with a
// value built elsewhere, so its members are out of reach here.
func nestedWholeMemberSupplied(b Box) Holder {
	var h Holder
	h.Inner = b
	return h
}

// localWholeMemberZero writes a zero struct into the nested member. The
// write neither fills the field inside it nor lets the value out, so the
// omission surfaces — once — where the value escapes. The two rules meet
// here, so this pins that neither drops it nor doubles it.
func localWholeMemberZero() Holder {
	var h Holder
	h.Inner = Box{}
	return h // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// boundNestedLiteral binds the nested literal to a local and lets it out
// with the `!` field still nil. The diagnostic lands at the escape rather
// than at the literal, which is what keeps this and the direct return
// under one rule instead of two.
func boundNestedLiteral() Holder {
	h := Holder{Inner: Box{}}
	return h // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// boundNestedLiteralFilled writes the nested field of the bound literal
// before the value leaves. Binding holds a nested omission open to a
// later write on the same terms as a top-level one.
func boundNestedLiteralFilled(x *int) Holder {
	h := Holder{Inner: Box{}}
	h.Inner.Ref = x
	return h
}

// copiedIntoLocal writes a zero into one local's member and copies the
// whole value into another before letting it out. The omission survives
// the copy and surfaces once, where the value leaves.
func copiedIntoLocal() Holder {
	var src Holder
	src.Inner = Box{}
	dst := src
	return dst // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// memberWholeZeroThenFilled writes a zero into the nested member and
// fills the member's `!` field before the value leaves. The write keeps
// the member in reach, so the fill closes it and nothing is reported.
// boundNestedLiteralFilled reads the same way but reaches the member
// through a store addressed to the allocation, so this case is the one
// that holds the member-addressed route open to a later write.
func memberWholeZeroThenFilled(x *int) Holder {
	var h Holder
	h.Inner = Box{}
	h.Inner.Ref = x
	return h
}

// allocMemberWholeZeroHandedToCall writes a zero into the member of an
// allocation the callee receives. The escape rule leaves such an
// allocation alone, since the callee holds the pointer and may fill the
// field, so nothing judges this storage later and the write is judged
// here.
func allocMemberWholeZeroHandedToCall() {
	h := &Holder{}
	h.Inner = Box{} // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
	holdHolder(h)
}

// holdHolder stands in for a callee that may fill the field.
func holdHolder(*Holder) {}

// sliceOfLiterals omits the `!` field in an elided element literal.
func sliceOfLiterals() []Box {
	return []Box{{}} // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// sliceOfMixedLiterals supplies the `!` field in one element and omits
// it in another, so the judgement is per element rather than per
// allocation. The report names the field once however many elements
// owe it.
func sliceOfMixedLiterals(x *int) []Box {
	return []Box{{Ref: x}, {}} // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// arrayNeverAddressed lets an array out with no element ever written.
func arrayNeverAddressed() []Box {
	var xs [2]Box
	return xs[:] // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// arrayElementWholeZero writes a zero into each element through the
// element's own address. Every element is in reach of a later field
// write, so the omission surfaces once, where the array leaves.
func arrayElementWholeZero() []Box {
	var xs [2]Box
	xs[0] = Box{}
	xs[1] = Box{}
	return xs[:] // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// structArrayElementWholeZero writes a zero into an element of an array
// held as a struct member. The escape rule reads one array deep from an
// allocation and does not reach an array behind a member, so nothing
// judges this storage later and the write is judged here.
func structArrayElementWholeZero() Tray {
	var t Tray
	t.Boxes[0] = Box{} // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
	return t
}

// computedIndexWholeZero writes a zero through an index this pass cannot
// resolve. The escape rule declines such an element, so again nothing
// judges the storage later and the write is judged here.
func computedIndexWholeZero(at int) []Box {
	var xs [2]Box
	xs[at] = Box{} // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
	return xs[:]
}

// nestedArrayElementWholeZero writes a zero into an element of an array
// of arrays. The escape rule reads one array deep from an allocation, so
// an element of the inner one is out of its reach and the write is judged
// here.
func nestedArrayElementWholeZero() [2][2]Box {
	var g [2][2]Box
	g[0][1] = Box{} // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
	return g
}

// arrayInMemberNeverWritten lets an array held as a struct member out
// with nothing written. No diagnostic is reported: the fields reachable
// by value stop at the array, so the `!` field inside an element is not
// among the ones this rule asks about. The case is here so that a change
// which starts reaching them fails rather than passes quietly.
func arrayInMemberNeverWritten() Tray {
	var t Tray
	return t
}

// ---------- satisfied and exempt positions ----------

// sliceOfSuppliedLiterals supplies the `!` field of every element.
func sliceOfSuppliedLiterals(x *int) []Box {
	return []Box{{Ref: x}}
}

// arrayFilledByComputedIndex writes through an index this pass cannot
// resolve, which leaves it unable to say which element the write filled.
func arrayFilledByComputedIndex(x *int, at int) []Box {
	var xs [2]Box
	xs[at].Ref = x
	return xs[:]
}

// closureFillsCapture captures the value through a variable the literal
// writes, so the closure is free to fill the field wherever it runs.
func closureFillsCapture(x *int) Box {
	var b Box
	fill := func() { b.Ref = x }
	fill()
	return b
}

// closureFilledBeforeCapture fills the field ahead of the capture.
func closureFilledBeforeCapture(x *int) func() Box {
	var b Box
	b.Ref = x
	return func() Box { return b }
}

// returnSatisfied supplies the `!` field, which is what the
// declaration asks for. The platform field stays free to omit.
func returnSatisfied(x *int) Box {
	return Box{Ref: x}
}

// returnPositional supplies every field by Go's own rule for a
// positional literal, so nothing is omitted.
func returnPositional(x *int) Box {
	return Box{x, nil, nil}
}

// localThenFill binds the literal to a local and fills the `!` field
// before the value escapes, so the construction obligation is
// discharged and the literal stays silent.
func localThenFill(x *int) Box {
	b := Box{}
	b.Ref = x
	return b
}

// localReassign binds through a plain assignment rather than a short
// declaration, which keeps the value reachable just the same.
func localReassign(x *int) Box {
	var b Box
	b = Box{}
	b.Ref = x
	return b
}

// localDeclaredVar binds through a local var declaration, which is the
// same build-then-fill shape the short declaration carries. The local
// and package-scope declaration forms share one locality predicate, so
// this case and declaredPackageVar cannot drift apart.
func localDeclaredVar(x *int) Box {
	var b = Box{}
	b.Ref = x
	return b
}

// discarded throws the literal away, so no reader can observe the
// omitted field.
func discarded() {
	_ = Box{}
}

// nonStructLiteral pins that a slice, map, or array literal carries no
// field obligation of its own.
func nonStructLiteral() []*int {
	return []*int{nil, nil}
}

// Read reports the `!` field, so the method body is a reader that the
// declaration entitles to skip the guard.
func (b Box) Read() *int { return b.Ref }

// methodReceiver invokes a method on the literal, which hands the
// value to a body that reads the omitted field.
func methodReceiver() *int {
	return Box{}.Read() // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// methodReceiverAddressOf reaches the same body through a pointer to
// the literal.
func methodReceiverAddressOf() *int {
	return (&Box{}).Read() // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// packageQualifiedCall pins that a selector naming a package rather
// than a value contributes no receiver to judge: the selector's left
// side is the package identifier, which is not a literal, so the
// receiver walk finds nothing and the argument walk decides alone.
func packageQualifiedCall() string {
	return strings.TrimSpace(" x ")
}

// mapValueStore puts a zero struct into a map. A map element is not
// addressable, so `m["k"].Ref = x` will not compile and the field can
// never be filled afterwards — the opposite of a local slot.
func mapValueStore(m map[string]Box) {
	m["k"] = Box{} // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// closureCapture returns a function holding the zero value. The literal
// only reads the capture, so nothing can fill the field once the closure
// carries it away.
func closureCapture() func() Box {
	var b Box
	return func() Box { return b } // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
}

// closureBuiltBeforeFill fills the field after the closure captures the
// value. The capture is by reference, so the closure does observe the
// fill — this reports anyway, and the shape to write instead is the fill
// ahead of the closure, as closureFilledBeforeCapture has it.
func closureBuiltBeforeFill(x *int) Box {
	var b Box
	read := func() Box { return b } // want `vow\[nil-safety\]: field Ref left nil where this value escapes; vow:nil declared ! on this field`
	b.Ref = x
	return read()
}
