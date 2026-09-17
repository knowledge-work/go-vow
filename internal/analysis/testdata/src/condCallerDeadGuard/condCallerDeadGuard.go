// Package condCallerDeadGuard exercises the caller-side dead-
// guard recogniser driven by a vow:cond forward implication or
// biconditional rule. The callee ties its returned error to the
// nilness of the paired return slot, so an err-guard on the
// caller side that short-circuits the `err != nil` counter-
// branch leaves the continuation with the paired local proven
// non-nil, and any subsequent nil check on it is dead.
package condCallerDeadGuard

// Foo is the payload the fixtures dereference after the guards.
type Foo struct{ v int }

// Bar exists so a caller can exercise a method-call deref on the
// narrowed local.
func (f *Foo) Bar() int { return f.v }

// ErrWrapper carries an error inside a struct field so a path-
// bearing rule fixture below has somewhere to point.
type ErrWrapper struct{ Err error }

// NewFooBicond ties err and $1 with a biconditional so both
// forward and backward directions are available; the caller-
// narrow surface reads the forward direction (err == nil implies
// $1 != nil) once the paired err-guard short-circuits. The
// returns are named so the rule's `err` reference resolves
// against the callee signature.
//
// vow:cond err == nil <=> $1 != nil
func NewFooBicond() (foo *Foo, err error) { return &Foo{}, nil } // want NewFooBicond:"vow:cond\\(err == nil <=> \\$1 != nil\\)"

// NewFooImpl uses the forward implication only. The caller-side
// narrow reads the same direction, so the diagnostic surface is
// identical to the biconditional case; the backward half a
// biconditional would add stays unused here.
//
// vow:cond err == nil => $1 != nil
func NewFooImpl() (foo *Foo, err error) { return &Foo{}, nil } // want NewFooImpl:"vow:cond\\(err == nil => \\$1 != nil\\)"

// callerBicondRedundantGuard is the biconditional pattern-A
// baseline: the err-guard short-circuits the counter-branch, so
// the continuation runs under err == nil and the biconditional's
// forward direction proves foo non-nil. The second guard is
// dead.
func callerBicondRedundantGuard() {
	foo, err := NewFooBicond()
	if err != nil {
		return
	}
	if foo == nil { // want `vow\[nil-safety\]: guard on foo is dead \(impossible\); vow:cond rule .* on NewFooBicond narrows return position 1 to non-nil once the guard on err short-circuits`
		return
	}
	_ = foo.Bar()
}

// callerImplRedundantGuard exercises the forward-implication
// pattern: the analyzer reads the same forward direction the
// biconditional case reads, so the diagnostic surface matches.
func callerImplRedundantGuard() {
	foo, err := NewFooImpl()
	if err != nil {
		return
	}
	if foo == nil { // want `vow\[nil-safety\]: guard on foo is dead \(impossible\); vow:cond rule .* on NewFooImpl narrows return position 1 to non-nil once the guard on err short-circuits`
		return
	}
	_ = foo.Bar()
}

// callerNoErrGuardSilent leaves the pending binding unpromoted
// because no err-guard short-circuits the counter-branch. The
// nil check on foo stays silent — the rule's narrow only fires
// after the err-guard proves err == nil. The fixture avoids a
// selector deref on foo so it stays focused on the dead-guard
// surface; the safety-violation pass covers the deref-without-
// guard shape in its own testdata package.
func callerNoErrGuardSilent() {
	foo, err := NewFooBicond()
	_ = err
	if foo == nil {
		return
	}
	_ = foo
}

// callerFullyGuardedOKSilent is the positive baseline: err-guard
// first, then a direct method-call deref with no redundant nil
// check. No diagnostic fires because there is no dead guard to
// report.
func callerFullyGuardedOKSilent() {
	foo, err := NewFooBicond()
	if err != nil {
		return
	}
	_ = foo.Bar()
}

// callerReassignedFoo drops the binding when foo is reassigned
// between the err-guard and the nil check. The mutated local no
// longer carries the call's contract, so the analyzer stays
// silent on the follow-up guard.
func callerReassignedFoo() {
	foo, err := NewFooBicond()
	if err != nil {
		return
	}
	foo = nil
	if foo == nil {
		return
	}
	_ = foo
}

// callerFuncLitClearsBindings drops every binding when a
// statement carries a function literal. A closure can rewrite a
// captured local out of syntactic view, so the analyzer stays
// silent on the follow-up guard even though the err-guard is in
// place.
func callerFuncLitClearsBindings() {
	foo, err := NewFooBicond()
	_ = func() { _ = err }
	if err != nil {
		return
	}
	if foo == nil {
		return
	}
	_ = foo.Bar()
}

// callerBlankErrSilent binds the error to `_`, so no guardName
// pairs with the biconditional's LHS at record time and the
// binding never leaves pending. The nil check stays silent.
func callerBlankErrSilent() {
	foo, _ := NewFooBicond()
	if foo == nil {
		return
	}
	_ = foo
}

// callerInitClauseGuardSilent carries an if init clause on the
// nil check. The init clause can rebind the identifier the
// condition reads, so the analyzer skips the shape and stays
// silent even though the narrow is otherwise in place.
func callerInitClauseGuardSilent() {
	foo, err := NewFooBicond()
	if err != nil {
		return
	}
	if x := foo; x == nil {
		return
	}
	_ = foo
}

// NewFooBicondValueFirst spells the biconditional in value-first
// order, which is what an author reaches for when the value slot
// dominates the reading order. The recogniser accepts both operand
// orders because `<=>` commutes.
//
// vow:cond $1 != nil <=> err == nil
func NewFooBicondValueFirst() (foo *Foo, err error) { return &Foo{}, nil } // want NewFooBicondValueFirst:"vow:cond\\(\\$1 != nil <=> err == nil\\)"

// callerBicondValueFirstRedundantGuard exercises the value-first
// biconditional. The narrow reads the same forward direction — err
// == nil implies $1 != nil — regardless of whether the source
// spells the operands in error-first or value-first order.
func callerBicondValueFirstRedundantGuard() {
	foo, err := NewFooBicondValueFirst()
	if err != nil {
		return
	}
	if foo == nil { // want `vow\[nil-safety\]: guard on foo is dead \(impossible\); vow:cond rule .* on NewFooBicondValueFirst narrows return position 1 to non-nil once the guard on err short-circuits`
		return
	}
	_ = foo.Bar()
}

// callerRebindErrDoesNotFalseNarrow exercises the guardName
// cascade invalidation: `err` is reassigned by a second call
// before the err-guard runs, so the pending binding from the
// first call cannot be promoted by the guard that now names a
// different value. Without the cascade, the analyzer would
// falsely emit a dead-guard on the first call's bound local.
func callerRebindErrDoesNotFalseNarrow() {
	b, err := NewFooBicond()
	c, err := NewFooBicond()
	if err != nil {
		return
	}
	if b == nil {
		return
	}
	_ = b
	_ = c
}

// NewFooPathRule ties a path-bearing operand to $1. Path-bearing
// references fall outside the caller-narrow surface because the
// caller-side identifier alignment reads bare LHS names on the
// assignment; the rule stays parsed and the caller-side surface
// stays silent.
//
// vow:cond wrap.Err == nil <=> $1 != nil
func NewFooPathRule(wrap *ErrWrapper) *Foo { return &Foo{} } // want NewFooPathRule:"vow:cond\\(wrap.Err == nil <=> \\$1 != nil\\)"

// callerPathRuleSilent exercises the path-bearing exclusion. The
// caller mirrors the rule's guard on the wrapper's Err field, but
// the recogniser skips path-bearing operands so no dead-guard
// diagnostic fires.
func callerPathRuleSilent(wrap *ErrWrapper) {
	foo := NewFooPathRule(wrap)
	if wrap.Err != nil {
		return
	}
	if foo == nil {
		return
	}
	_ = foo
}
