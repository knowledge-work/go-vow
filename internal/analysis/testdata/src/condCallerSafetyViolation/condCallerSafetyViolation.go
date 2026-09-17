// Package condCallerSafetyViolation exercises the caller-side
// safety-violation recogniser: a selector-driven dereference of
// a call-result local whose paired vow:cond guard has not yet
// short-circuited its counter-branch. The dead-guard recogniser
// covers the mirror shape (a guard standing over an already-
// narrowed local); together the two surfaces catch the two ways
// a caller can misread a `vow:cond err == nil <=> $1 != nil`
// rule — keeping the check that has been answered, and skipping
// the check that is required.
package condCallerSafetyViolation

// Foo is the payload the fixtures dereference.
type Foo struct{ v int }

// Bar exists so a caller can drive a method-call deref on the
// pending local.
func (f *Foo) Bar() int { return f.v }

// NewFooBicond ties err and $1 with a biconditional so the
// narrow surface is available in both directions. Only the
// forward direction (`err == nil` implies `$1 != nil`) matters
// for the safety-violation cases: without the err-guard, the
// contract leaves $1 possibly-nil.
//
// vow:cond err == nil <=> $1 != nil
func NewFooBicond() (foo *Foo, err error) { return &Foo{}, nil } // want NewFooBicond:"vow:cond\\(err == nil <=> \\$1 != nil\\)"

// NewFooImpl uses the forward implication only. The safety
// surface reads the same direction the dead-guard surface reads,
// so a deref without a preceding err-guard triggers regardless
// of which arrow the callee spells.
//
// vow:cond err == nil => $1 != nil
func NewFooImpl() (foo *Foo, err error) { return &Foo{}, nil } // want NewFooImpl:"vow:cond\\(err == nil => \\$1 != nil\\)"

// callerBlankErrDeref binds the error to `_` and immediately
// dereferences the paired local. The rule leaves foo possibly-
// nil until the paired guard runs; binding the error to `_`
// means that guard can never run (Go rejects `if _ != nil`), so
// the deref reads a value the contract has not proven safe.
func callerBlankErrDeref() {
	foo, _ := NewFooBicond()
	_ = foo.Bar() // want `vow\[nil-safety\]: foo is dereferenced without a short-circuiting guard on _; vow:cond rule .* on NewFooBicond leaves return position 1 possibly-nil until that guard runs`
}

// callerDerefBeforeErrGuard dereferences the paired local
// before the err-guard runs. The guard sits later in the block,
// so the deref reads a still-pending binding and the rule leaves
// the value possibly-nil at the deref site.
func callerDerefBeforeErrGuard() {
	foo, err := NewFooBicond()
	_ = foo.Bar() // want `vow\[nil-safety\]: foo is dereferenced without a short-circuiting guard on err; vow:cond rule .* on NewFooBicond leaves return position 1 possibly-nil until that guard runs`
	if err != nil {
		return
	}
}

// callerImplDerefBeforeErrGuard exercises the forward-only
// implication rule with the deref-before-guard shape. Same
// diagnostic as the biconditional case because the safety
// surface reads the same forward direction.
func callerImplDerefBeforeErrGuard() {
	foo, err := NewFooImpl()
	_ = foo.Bar() // want `vow\[nil-safety\]: foo is dereferenced without a short-circuiting guard on err; vow:cond rule .* on NewFooImpl leaves return position 1 possibly-nil until that guard runs`
	if err != nil {
		return
	}
}

// callerDerefFieldAccess exercises the selector shape on a
// plain field read rather than a method call. Both shapes read
// the receiver, so the safety surface catches both.
func callerDerefFieldAccess() {
	foo, _ := NewFooBicond()
	_ = foo.v // want `vow\[nil-safety\]: foo is dereferenced without a short-circuiting guard on _; vow:cond rule .* on NewFooBicond leaves return position 1 possibly-nil until that guard runs`
}

// callerErrGuardBeforeDerefSilent is the positive baseline:
// the err-guard runs first, narrows foo to non-nil, and the
// subsequent deref reads a proven-safe value. No diagnostic.
func callerErrGuardBeforeDerefSilent() {
	foo, err := NewFooBicond()
	if err != nil {
		return
	}
	_ = foo.Bar()
}

// callerDirectFooGuardBeforeDerefSilent exercises the second
// promotion path: `if foo == nil { return }` short-circuits the
// target-nil branch, proving foo non-nil past the guard even
// when the paired error was bound to `_` and can never itself
// be checked. The subsequent deref reads a proven-safe value.
func callerDirectFooGuardBeforeDerefSilent() {
	foo, _ := NewFooBicond()
	if foo == nil {
		return
	}
	_ = foo.Bar()
}

// callerReassignedFooSilent drops the pending binding when foo
// is reassigned before the deref. The mutated local no longer
// carries the call's contract, so the safety surface stays
// silent even though no err-guard has run.
func callerReassignedFooSilent() {
	foo, _ := NewFooBicond()
	foo = &Foo{v: 1}
	_ = foo.Bar()
}

// callerFuncLitClearsSilent drops every binding when a
// statement carries a function literal. A closure can rewrite a
// captured local out of syntactic view, so the safety surface
// stays silent past the funcLit even without an err-guard.
func callerFuncLitClearsSilent() {
	foo, _ := NewFooBicond()
	_ = func() { _ = foo }
	_ = foo.Bar()
}

// callerNestedBlockDeref hides the deref inside an inner
// block. Under Stack 2's body-narrow carry the safety surface
// descends into the inner block with the outer pending state
// carried through, so a deref of a still-pending local inside
// the if-body reads the same possibly-nil value the outer
// block would have flagged. The blank paired guard (`_`) can
// never fire, so the pending stays pending and the deref is a
// safety violation regardless of the enclosing block.
func callerNestedBlockDeref() {
	foo, _ := NewFooBicond()
	if condTrue() {
		_ = foo.Bar() // want `vow\[nil-safety\]: foo is dereferenced without a short-circuiting guard on _; vow:cond rule .* on NewFooBicond leaves return position 1 possibly-nil until that guard runs`
	}
}

// condTrue is a helper that keeps the nested-block fixture
// compilable without pulling in a real condition source.
func condTrue() bool { return true }
