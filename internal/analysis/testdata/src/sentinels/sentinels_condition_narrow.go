package sentinels

// Fixtures for the SSA-narrowed enforcement of vow:cond.
//
// A non-trivial parameter requirement narrows return-requirement
// enforcement to the branches where the prereq provably holds
// (via dominator-chain reasoning over the SSA control-flow
// graph). Outside those branches the return requirement is
// unchecked so authors do not see false positives from an
// over-broad apply-everywhere reading.
//
// The fixtures here drive int-returning functions so the
// return-requirement-mismatch diagnostic stands alone — sentinel-
// error machinery (chain authorisation, must-consume) is
// orthogonal and is exercised by the sentinels_condition_marker.go
// fixture.

// conditionNarrowEnforcedInBranch returns the int 4 inside the
// prereq-holding `if x != nil` branch. The dominator walk finds
// the controlling `*ssa.If` and recognises the then-side as the
// branch that proves the `nonnil x` predicate; return-requirement
// enforcement fires there and rejects 4 because the sum
// `{ 1 | 2 | 3 }` does not list it. The trailing `return 0` sits
// outside the narrowed scope (the else side proves `x == nil`,
// the opposite of the prereq), so the return requirement stays
// unchecked there.
//
// vow:cond nonnil -> 1 | 2 | 3
func conditionNarrowEnforcedInBranch(x *int) int {
	if x != nil {
		return 4 // want `vow\[sentinel-error\]: return position 0: returning literal 4 but the position only accepts 1 \| 2 \| 3`
	}
	return 0
}

// conditionNarrowAllowedMatching returns a listed member from
// inside the prereq-holding branch. The dominator walk recognises
// the narrowing, the matcher accepts 2 against `{ 1 | 2 | 3 }`,
// and no diagnostic fires. The trailing `return 0` sits outside
// the narrowed scope and is unchecked.
//
// vow:cond nonnil -> 1 | 2 | 3
func conditionNarrowAllowedMatching(x *int) int {
	if x != nil {
		return 2
	}
	return 0
}

// conditionNarrowSkipsOutsideBranch returns a non-listed value
// outside the prereq-holding branch. The under-approximation
// refuses to enforce the return requirement when the prereq is
// not provably narrowed, so no diagnostic fires even though 99
// is not in `{ 1 | 2 | 3 }`. The prereq does not hold on this
// path (x is nil), so the contract simply does not apply.
//
// vow:cond nonnil -> 1 | 2 | 3
func conditionNarrowSkipsOutsideBranch(x *int) int {
	if x == nil { // want `vow\[nil-safety\]: guard on x is dead; callee declared nonnil x in vow:cond`
		return 99
	}
	return 1
}

// conditionNarrowGuardThenReturn covers the early-return guard
// pattern. The guard `if x == nil { return 0 }` establishes that
// every path past the guard has `x != nil`, so the trailing
// `return 99` sits inside the narrowed scope and trips the
// return-requirement check. The dominator walk recognises the
// guard via the else successor of the `if x == nil` instruction
// (`if x != nil` would be the canonical positive shape; the
// matcher also accepts the negated form so authors can write
// either style).
//
// vow:cond nonnil -> 1 | 2 | 3
func conditionNarrowGuardThenReturn(x *int) int {
	if x == nil { // want `vow\[nil-safety\]: guard on x is dead; callee declared nonnil x in vow:cond`
		return 0
	}
	return 99 // want `vow\[sentinel-error\]: return position 0: returning literal 99 but the position only accepts 1 \| 2 \| 3`
}

// conditionNarrowZeroPredicate covers the `nonzero` predicate. The
// matcher recognises `if y != 0` (and the mirror `if y == 0`
// guard) as the narrowing source for `nonzero y`. Returning the
// non-listed literal 5 inside the `if y != 0` branch trips the
// return requirement; the trailing `return 0` outside the narrow
// stays unchecked.
//
// vow:cond nonzero -> 1 | 2 | 3
func conditionNarrowZeroPredicate(y int) int {
	if y != 0 {
		return 5 // want `vow\[sentinel-error\]: return position 0: returning literal 5 but the position only accepts 1 \| 2 \| 3`
	}
	return 0
}

// conditionNarrowUnsupportedPattern returns a non-listed value
// at a return whose prereq cannot be narrowed via the dominator
// walk (no controlling `if` guards the path). The under-
// approximation refuses to enforce the return requirement so no
// diagnostic fires. This fixture pins the "unrecognised pattern
// → skip" branch and guards against an accidental widening of
// the matcher that would surface false positives on free-form
// control flow.
//
// vow:cond nonnil -> 1 | 2 | 3
func conditionNarrowUnsupportedPattern(x *int) int {
	_ = x
	return 99
}
