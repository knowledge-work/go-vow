// Package condCallerNegativeForm pins the caller-side dead-guard
// recogniser's behaviour on a negative-form `vow:cond` rule
// (`err != nil => $1 == nil`) and, for symmetry, on the
// positive-form outer-body shape (`err == nil => $1 != nil` in
// an outer if-body). Stack 2's positive-branch narrow carry
// makes the four negative-form patterns (outer body × 2,
// fall-through × 2) and the symmetric positive-form outer-body
// pattern fire; the converse and inverse directions stay silent
// because the arrow does not guarantee them and no carry
// mechanism should derive a narrow from them.
package condCallerNegativeForm

// Foo is the payload the fixtures reason about.
type Foo struct{ v int }

// NewFooNeg ties err and $1 with a negative-form forward
// implication: a non-nil error implies the first return is nil.
// The rule shape lands on narrowToNil in the recogniser; the
// promote path leaves it pending until Stack 2 carries the
// narrow into an outer body or fall-through position where the
// direction becomes observable.
//
// vow:cond err != nil => $1 == nil
func NewFooNeg() (foo *Foo, err error) { return &Foo{}, nil } // want NewFooNeg:"vow:cond\\(err != nil => \\$1 == nil\\)"

// NewFooPositive is the symmetric positive-form counterpart. The
// forward implication carries a nilness precondition into a non-
// nilness conclusion, so the target narrows to non-nil.
//
// vow:cond err == nil => $1 != nil
func NewFooPositive() (foo *Foo, err error) { return &Foo{}, nil } // want NewFooPositive:"vow:cond\\(err == nil => \\$1 != nil\\)"

// callerNegPattern1Redundant — outer body
// `if err != nil { if foo == nil { return } }`.
// The outer guard's body-narrow carry proves foo nil inside
// the if-body (negative rule's antecedent `err != nil` holds
// on entry), so the inner `if foo == nil { return }` guard is
// redundant against the already-proven narrow.
func callerNegPattern1Redundant() {
	foo, err := NewFooNeg()
	if err != nil {
		if foo == nil { // want `vow\[nil-safety\]: guard on foo is dead \(redundant\); vow:cond rule .* on NewFooNeg narrows return position 1 to nil once the guard on err short-circuits`
			return
		}
	}
	_ = foo
}

// callerNegPattern2Impossible — outer body
// `if err != nil { if foo != nil { return } }`.
// Same body-narrow direction as pattern 1 (foo narrowed to nil
// inside the outer body); the inner `if foo != nil { return }`
// guard uses the opposite comparison so the branch is
// impossible (a nil-narrowed value never satisfies `!= nil`).
func callerNegPattern2Impossible() {
	foo, err := NewFooNeg()
	if err != nil {
		if foo != nil { // want `vow\[nil-safety\]: guard on foo is dead \(impossible\); vow:cond rule .* on NewFooNeg narrows return position 1 to nil once the guard on err short-circuits`
			return
		}
	}
	_ = foo
}

// callerNegPattern3Redundant — fall-through
// `if err == nil { return }; if foo == nil { return }`.
// The short-circuit body removes the err-nil branch, so the
// fall-through runs under `err != nil` — the negative rule's
// antecedent holds and the target narrows to nil. The inner
// guard's `== nil` check is redundant against that narrow.
func callerNegPattern3Redundant() {
	foo, err := NewFooNeg()
	if err == nil {
		return
	}
	if foo == nil { // want `vow\[nil-safety\]: guard on foo is dead \(redundant\); vow:cond rule .* on NewFooNeg narrows return position 1 to nil once the guard on err short-circuits`
		return
	}
	_ = foo
}

// callerNegPattern4Impossible — fall-through
// `if err == nil { return }; if foo != nil { return }`.
// Same fall-through narrow as pattern 3 (foo pinned to nil);
// the inner `!= nil` guard is impossible against the narrow.
func callerNegPattern4Impossible() {
	foo, err := NewFooNeg()
	if err == nil {
		return
	}
	if foo != nil { // want `vow\[nil-safety\]: guard on foo is dead \(impossible\); vow:cond rule .* on NewFooNeg narrows return position 1 to nil once the guard on err short-circuits`
		return
	}
	_ = foo
}

// callerNegConverseSilent pins the converse direction as
// silent. The rule `err != nil => $1 == nil` does not
// guarantee `$1 == nil => err != nil`, so an
// `if foo == nil { if err != nil { return } }` shape must not
// derive a narrow on err. Stays silent under every carry
// mechanism.
func callerNegConverseSilent() {
	foo, err := NewFooNeg()
	if foo == nil {
		if err != nil {
			return
		}
	}
	_ = foo
}

// callerNegInverseSilent pins the inverse direction as silent.
// The rule `err != nil => $1 == nil` does not guarantee
// `err == nil => $1 != nil`, so an
// `if err == nil { if foo == nil { return } }` shape must not
// derive a narrow on foo. Stays silent under every carry
// mechanism.
func callerNegInverseSilent() {
	foo, err := NewFooNeg()
	if err == nil {
		if foo == nil {
			return
		}
	}
	_ = foo
}

// callerPositivePattern2Impossible — outer body
// `if err == nil { if foo == nil { return } }` on the positive
// form. The outer guard's body-narrow proves foo non-nil
// inside the if-body (positive rule's antecedent `err == nil`
// holds on entry), so the inner `== nil` guard is impossible
// against the narrow.
func callerPositivePattern2Impossible() {
	foo, err := NewFooPositive()
	if err == nil {
		if foo == nil { // want `vow\[nil-safety\]: guard on foo is dead \(impossible\); vow:cond rule .* on NewFooPositive narrows return position 1 to non-nil once the guard on err short-circuits`
			return
		}
	}
	_ = foo
}

// callerPositiveConverseSilent pins the converse of the
// positive rule (`$1 != nil => err == nil`) as silent. Even
// under a full carry, the analyzer must never derive a narrow
// on err from a guard on foo alone.
func callerPositiveConverseSilent() {
	foo, err := NewFooPositive()
	if foo != nil {
		if err == nil {
			return
		}
	}
	_ = foo
}

// callerPositiveInverseSilent pins the inverse of the positive
// rule (`err != nil => $1 == nil`) as silent. The positive rule
// says nothing about the non-nil-err branch, so no narrow on
// foo can be derived there.
func callerPositiveInverseSilent() {
	foo, err := NewFooPositive()
	if err != nil {
		if foo == nil {
			return
		}
	}
	_ = foo
}

// callerNegErrGuardFallThroughSilent pins the false-positive
// regression on the negative-form err-guard fall-through. The
// caller writes `if err != nil { return }` on a negative-form
// binding; after the short-circuit the fall-through runs under
// `err == nil`, which is the negative rule's counter-antecedent
// (the implication becomes vacuous, so `foo` stays possibly-
// nil). The Path 1 promote must therefore gate on
// `narrowDir == narrowToNonNil` and skip this binding, or the
// subsequent `if foo == nil { return }` would be reported as
// redundant against a narrow that was never proven. Any diagnostic
// on the inner guard here is a regression.
func callerNegErrGuardFallThroughSilent() {
	foo, err := NewFooNeg()
	if err != nil {
		return
	}
	if foo == nil {
		return
	}
	_ = foo
}
