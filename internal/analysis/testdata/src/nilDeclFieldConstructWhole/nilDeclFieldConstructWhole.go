// Package nilDeclFieldConstructWhole pins the construction obligation
// against a struct filled by one store of the whole value rather than
// by a store per field. The member arrives already built — from a
// parameter, a receiver, or a call result — so the party that owes its
// `!` fields is whoever built it, and the escape here carries no
// omission of its own. The cases that must stay silent are the ones
// this package exists for: an author who fills every field has no way
// to discharge a diagnostic raised against them.
package nilDeclFieldConstructWhole

// Dep is the pointee the `!` declarations below refer to.
type Dep struct{ N int }

// Registry carries two `!` fields so a whole-value store has more than
// one member to account for.
type Registry struct {
	// vow:nil !
	A *Dep // want A:"fieldNil\\(!\\)"

	// vow:nil !
	B *Dep // want B:"fieldNil\\(!\\)"
}

// Outer holds a Registry by value so a nested member is reached under
// the same store as a top-level one.
type Outer struct{ Inner Registry }

var globalSink Registry

var chSink = make(chan Registry, 1)

var mapSink = map[string]Registry{}

func consume(r Registry) {}

func build(a, b *Dep) Registry { return Registry{A: a, B: b} }

func transact(fn func(tx int) error) error { return fn(0) }

func (r Registry) read() *Dep { return r.A }

func (r Registry) emit() {}

// ---------- the value arrives built ----------

// paramAddress lets the address of a filled parameter out. The store
// the spill compiles to writes every member at once.
func paramAddress(src Registry) *Registry { return &src }

// receiverAddress reaches the same store through a value receiver,
// which is the shape a constructor-per-method registry takes.
func (r Registry) receiverAddress() *Registry { return &r }

// callResultAddress binds a value-returning constructor's result and
// lets its address out.
func callResultAddress(a, b *Dep) *Registry {
	r := build(a, b)
	return &r
}

// plainCopy writes the whole value through an ordinary assignment.
func plainCopy(src Registry) *Registry {
	var r Registry
	r = src
	return &r
}

// fieldRestored writes one member a second time through a selector.
// The whole-value store already accounts for both, so the selector
// changes nothing about what is owed.
func fieldRestored(src Registry, a *Dep) *Registry {
	r := src
	r.A = a
	return &r
}

// fieldReadThenCall is the shape without an explicit address-of: the
// field read is what keeps the receiver's storage addressable, and the
// value-receiver call is what lets it out.
func (r Registry) fieldReadThenCall() (*Dep, *Dep) { return r.B, r.read() }

// callWithoutFieldRead passes the receiver straight through, so no
// storage is reserved for it at all.
func (r Registry) callWithoutFieldRead() *Dep { return r.read() }

// fieldReadWithoutCall lets only the member out, never the struct.
func (r Registry) fieldReadWithoutCall() *Dep { return r.A }

// nestedMember pins that the store at the root accounts for a member
// reached through a field as well as one at the top level.
func (o Outer) nestedMember() (*Dep, *Dep) { return o.Inner.B, o.Inner.read() }

// ---------- one store, every way the value leaves ----------

// The fill question is asked once per allocation and the escape branch
// does not enter into it, so each way out is pinned against the same
// store rather than against a shape of its own.

func escapeCallArgument(src Registry) {
	r := src
	_ = r.A
	consume(r)
}

func escapeGlobalStore(src Registry) {
	r := src
	_ = r.A
	globalSink = r
}

func escapeChannelSend(src Registry) {
	r := src
	_ = r.A
	chSink <- r
}

func escapeMapUpdate(src Registry) {
	r := src
	_ = r.A
	mapSink["k"] = r
}

func escapeClosure(src Registry) func() Registry {
	r := src
	_ = r.A
	return func() Registry { return r }
}

func escapeDeferredMethod(src Registry) {
	r := src
	_ = r.A
	defer func() { r.emit() }()
}

// escapeClosureArgument hands the literal to a call rather than
// deferring or returning it, which is the transaction-callback shape.
func (r Registry) escapeClosureArgument() error {
	return transact(func(tx int) error {
		r.emit()
		return nil
	})
}

// closureReadsMemberOnly reaches a member through the capture without
// carrying the struct off, so the storage never leaves.
func (r Registry) closureReadsMemberOnly() error {
	return transact(func(tx int) error {
		_ = r.A
		return nil
	})
}

// ---------- what the store does not account for ----------

// rootZeroStore writes a zero struct at the root. A store addressed by
// the allocation is not an escape, because a later field write can still
// address that storage, so no other check judges what was written and the
// members it left nil are owed here.
func rootZeroStore() *Registry {
	var b Registry
	b = Registry{}
	return &b // want `vow\[nil-safety\]: field A left nil where this value escapes; vow:nil declared ! on this field` `vow\[nil-safety\]: field B left nil where this value escapes; vow:nil declared ! on this field`
}

// underfilledSource copies from a local whose own construction left B
// nil. The store carries what the source held, so A is accounted for
// and B is not — a per-member answer rather than one for the store.
func underfilledSource(a *Dep) *Registry {
	var s Registry
	s.A = a
	var r Registry
	r = s
	return &r // want `vow\[nil-safety\]: field B left nil where this value escapes; vow:nil declared ! on this field`
}

// ---------- the omission the check exists for ----------

// zeroLiteralEscapes owes both members with no store to account for
// them.
func zeroLiteralEscapes() Registry {
	return Registry{} // want `vow\[nil-safety\]: field A left nil where this value escapes; vow:nil declared ! on this field` `vow\[nil-safety\]: field B left nil where this value escapes; vow:nil declared ! on this field`
}

// nestedNeverWritten lets an Outer out with its nested member never
// addressed.
func nestedNeverWritten() Outer {
	var o Outer
	return o // want `vow\[nil-safety\]: field A left nil where this value escapes; vow:nil declared ! on this field` `vow\[nil-safety\]: field B left nil where this value escapes; vow:nil declared ! on this field`
}

// keyedLiteralOmits supplies one member and omits the other, which the
// per-field stores answer directly.
func keyedLiteralOmits(a *Dep) *Registry {
	return &Registry{A: a} // want `vow\[nil-safety\]: field B left nil where this value escapes; vow:nil declared ! on this field`
}

// ---------- a member loaded out of local storage ----------

// memberFromFilledLocal loads a member the enclosing storage filled, so
// the value it carries is accounted for under the path its own storage
// knows it by.
func memberFromFilledLocal(a, b *Dep) *Registry {
	var o Outer
	o.Inner.A = a
	o.Inner.B = b
	r := o.Inner
	return &r
}

// memberFromZeroLocal loads a member nothing wrote. Reaching it means
// walking from the load back to the storage it addresses, which is what
// keeps this apart from a value that arrived already built.
func memberFromZeroLocal() *Registry {
	var o Outer
	r := o.Inner
	return &r // want `vow\[nil-safety\]: field A left nil where this value escapes; vow:nil declared ! on this field` `vow\[nil-safety\]: field B left nil where this value escapes; vow:nil declared ! on this field`
}

// memberFromParameter loads a member of a struct that arrived built, so
// the walk reaches the parameter's own store and stops there.
func memberFromParameter(o Outer) *Registry {
	r := o.Inner
	return &r
}

// ---------- the gap this pass leaves open ----------

// zeroPackageVar is never assigned, so every member it carries is nil.

var zeroPackageVar Registry

// packageVarSource copies from a package-level var and stays silent. The
// stores that could fill one live in other functions — an initialiser,
// an init, any caller — and the dominance test that orders a store
// against an escape does not reach across a function boundary, so the
// members are taken as filled rather than reported against a store this
// pass cannot see.
func packageVarSource() *Registry {
	r := zeroPackageVar
	return &r
}
