// Package nilDeclCallerDeadGuard pins the caller-side dead-guard
// surface: a nil guard standing on a local bound to a call result the
// callee's vow:nil signature pins as `!`. The callee-side mirror lives
// in nilDeclSelfBasic, where the guard sits on a `!` parameter; here
// the contract arrives through the call's return slot instead.
//
// The silent fixtures pin the restrictions that keep the surface
// AST-only: a rewritten local, an address-of hand-off, a closure in
// the block, a fall-through guard body, an init clause on the if, and
// a guard nested in an inner block other than an if or else body all
// leave the diagnostic unemitted.
package nilDeclCallerDeadGuard

// Box is the value type every fixture allocates.
type Box struct{}

// newBox is the contract source for the single-value fixtures.
//
// vow:nil () !
func newBox() *Box { // want newBox:"nilDecl\\(\\) !"
	return &Box{}
}

// newBoxOrErr pairs a `!` first return with a nillable error slot so
// the multi-value binding path is exercised.
//
// vow:nil () !,?
func newBoxOrErr() (*Box, error) { // want newBoxOrErr:"nilDecl\\(\\) !,\\?"
	return &Box{}, nil
}

// nillableBox declares the return as `?` so a guard on its result is
// exactly what the contract invites.
//
// vow:nil () ?
func nillableBox() *Box { // want nillableBox:"nilDecl\\(\\) \\?"
	return nil
}

// unannotatedBox carries no vow declaration, so no contract reaches
// the call site.
func unannotatedBox() *Box {
	return &Box{}
}

// consume takes the address of a local so the address-of fixture has
// a sink.
func consume(**Box) {}

// run invokes f so the closure fixture has a sink.
func run(f func()) { f() }

// nilOrBox is the init-clause source for guardWithInitOK.
func nilOrBox() *Box { return nil }

// guardOnNonNilResult pins the reported shape: the local binds a `!`
// return slot and the guard short-circuits, so the branch can never
// run.
func guardOnNonNilResult() {
	b := newBox()
	if b == nil { // want `vow\[nil-safety\]: guard on b is dead \(impossible\); vow:nil declared ! at return position 1 of newBox \(the call cannot return nil\)`
		return
	}
	_ = b
}

// guardOnNonNilResultPanic pins the same shape with a panic body,
// matching the short-circuit shapes the callee-side recogniser
// accepts.
func guardOnNonNilResultPanic() {
	b := newBox()
	if b == nil { // want `vow\[nil-safety\]: guard on b is dead \(impossible\); vow:nil declared ! at return position 1 of newBox \(the call cannot return nil\)`
		panic("unreachable")
	}
	_ = b
}

// guardOnMultiValueResult pins the multi-value binding form. The
// error check between the binding and the guard leaves the binding
// intact because it rewrites nothing.
func guardOnMultiValueResult() error {
	b, err := newBoxOrErr()
	if err != nil {
		return err
	}
	if b == nil { // want `vow\[nil-safety\]: guard on b is dead \(impossible\); vow:nil declared ! at return position 1 of newBoxOrErr \(the call cannot return nil\)`
		return nil
	}
	_ = b
	return nil
}

// guardOnNillableResultOK pins the silent path: the callee declares
// the return as `?`, so the guard is the contract's intent.
func guardOnNillableResultOK() {
	b := nillableBox()
	if b == nil {
		return
	}
	_ = b
}

// guardOnUnannotatedResultOK pins the silent path on a callee that
// carries no vow declaration at all.
func guardOnUnannotatedResultOK() {
	b := unannotatedBox()
	if b == nil {
		return
	}
	_ = b
}

// guardAfterReassignOK pins the invalidation path: the local is
// rewritten between the binding and the guard, so the call's contract
// no longer describes the guarded value.
func guardAfterReassignOK(other *Box) {
	b := newBox()
	b = other
	if b == nil {
		return
	}
	_ = b
}

// guardAfterAddressOfOK pins the address-of invalidation: handing a
// pointer to the local puts later writes out of syntactic view.
func guardAfterAddressOfOK() {
	b := newBox()
	consume(&b)
	if b == nil {
		return
	}
	_ = b
}

// guardAfterFuncLitOK pins the closure path: a function literal in the
// block can rewrite the captured local on a path the statement walk
// cannot follow.
func guardAfterFuncLitOK() {
	b := newBox()
	run(func() { b = nil })
	if b == nil {
		return
	}
	_ = b
}

// guardWithoutShortCircuitOK pins the shape restriction: the guard's
// then-branch falls through, so the value still flows past the guard
// and the branch is not dead in the sense the diagnostic names.
func guardWithoutShortCircuitOK() {
	b := newBox()
	if b == nil {
		_ = b
	}
	_ = b
}

// guardWithInitOK pins the init-clause restriction: the clause can
// rebind the identifier the condition reads, so the binding recorded
// upstream no longer describes it.
func guardWithInitOK() {
	b := newBox()
	if b = nilOrBox(); b == nil {
		return
	}
	_ = b
}

// guardInNestedBlockOK pins the block-locality restriction: the guard
// sits in an inner block, which the pass walks on its own without the
// outer binding in scope.
func guardInNestedBlockOK() {
	b := newBox()
	{
		if b == nil {
			return
		}
	}
	_ = b
}

// guardInIfBody pins the body-narrow carry the shadow fixtures below
// rely on: the if-body scan inherits the outer binding, so a guard on
// the same local inside it is dead.
func guardInIfBody(c bool) {
	b := newBox()
	if c {
		if b == nil { // want `vow\[nil-safety\]: guard on b is dead \(impossible\); vow:nil declared ! at return position 1 of newBox \(the call cannot return nil\)`
			return
		}
	}
	_ = b
}

// guardOnVarShadowInIfBodyOK declares a new b inside the if-body, so
// the inner guard checks that variable rather than the newBox
// result.
func guardOnVarShadowInIfBodyOK(c bool) {
	b := newBox()
	if c {
		var b *Box
		if b == nil { // want `vow\[nil-safety\]: guard on b is dead \(impossible\); vow:nil declared ! at return position 1 of newBox \(the call cannot return nil\)`
			return
		}
		_ = b
	}
	_ = b
}

// guardOnVarShadowInElseBodyOK is the same shadow in an else-body,
// which inherits the outer binding like an if-body does.
func guardOnVarShadowInElseBodyOK(c bool) {
	b := newBox()
	if c {
		_ = b
	} else {
		var b *Box
		if b == nil { // want `vow\[nil-safety\]: guard on b is dead \(impossible\); vow:nil declared ! at return position 1 of newBox \(the call cannot return nil\)`
			return
		}
		_ = b
	}
	_ = b
}

// guardOnInitialisedVarShadowInIfBodyOK is the same shadow with an
// initialiser that can return nil.
func guardOnInitialisedVarShadowInIfBodyOK(c bool) {
	b := newBox()
	if c {
		var b = nillableBox()
		if b == nil { // want `vow\[nil-safety\]: guard on b is dead \(impossible\); vow:nil declared ! at return position 1 of newBox \(the call cannot return nil\)`
			return
		}
		_ = b
	}
	_ = b
}

// guardOnMultiNameVarShadowInIfBodyOK shadows b from a declaration that
// names more than one variable.
func guardOnMultiNameVarShadowInIfBodyOK(c bool) {
	b := newBox()
	if c {
		var n, b = 1, nillableBox()
		_ = n
		if b == nil { // want `vow\[nil-safety\]: guard on b is dead \(impossible\); vow:nil declared ! at return position 1 of newBox \(the call cannot return nil\)`
			return
		}
		_ = b
	}
	_ = b
}

// guardOnGroupedVarShadowInIfBodyOK shadows b from the second spec of
// a grouped declaration.
func guardOnGroupedVarShadowInIfBodyOK(c bool) {
	b := newBox()
	if c {
		var (
			n = 1
			b = nillableBox()
		)
		_ = n
		if b == nil { // want `vow\[nil-safety\]: guard on b is dead \(impossible\); vow:nil declared ! at return position 1 of newBox \(the call cannot return nil\)`
			return
		}
		_ = b
	}
	_ = b
}

// guardOnLabeledVarShadowInIfBodyOK shadows b from a labeled
// declaration, which declares b like an unlabeled one.
func guardOnLabeledVarShadowInIfBodyOK(c bool) {
	b := newBox()
	if c {
	retry:
		var b = nillableBox()
		if b == nil { // want `vow\[nil-safety\]: guard on b is dead \(impossible\); vow:nil declared ! at return position 1 of newBox \(the call cannot return nil\)`
			return
		}
		if c {
			goto retry
		}
		_ = b
	}
	_ = b
}

// guardBesideUnrelatedVarInIfBody declares a variable of another name,
// which leaves the binding of b in place.
func guardBesideUnrelatedVarInIfBody(c bool) {
	b := newBox()
	if c {
		var other *Box
		_ = other
		if b == nil { // want `vow\[nil-safety\]: guard on b is dead \(impossible\); vow:nil declared ! at return position 1 of newBox \(the call cannot return nil\)`
			return
		}
	}
	_ = b
}

// guardAfterVarShadowInIfBody pins that a shadow ends with its block:
// the outer guard still reads the newBox result.
func guardAfterVarShadowInIfBody(c bool) {
	b := newBox()
	if c {
		var b *Box
		_ = b
	}
	if b == nil { // want `vow\[nil-safety\]: guard on b is dead \(impossible\); vow:nil declared ! at return position 1 of newBox \(the call cannot return nil\)`
		return
	}
	_ = b
}
