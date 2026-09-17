package sentinels

// Fixtures for case-style multi-condition semantics.
//
// Multiple vow:cond lines on one function each describe an
// independent case. The analyzer collects every successfully-
// parsed line and, at each return, enforces the return
// requirement of every applicable case (Prereq holds). A return
// that matches no case's prereq is unchecked — the conservative
// single-condition narrowing carries over to the multi-line
// surface unchanged.
//
// Chain authorisation accumulates over every trivial-Prereq
// case: subjects listed in any vow:cond payload (lifted into
// a Condition with Prereq=nil) are authorised, with the union
// taken across all such cases.

// caseStyleDisjoint covers the canonical two-case pattern:
// `nonnil x` and `nil x` cases declare disjoint return-requirement
// sums. Each return sits inside exactly one prereq-holding
// branch, so the case-style matcher selects the matching return
// requirement and enforces it. Returning a member of the
// matching return requirement fires no diagnostic; returning a
// non-member would trip the sum-membership matcher of that case
// alone, leaving the other case unconcerned because its prereq
// does not hold here.
//
// vow:cond nonnil -> 1 | 2 | 3
// vow:cond nil -> 0
func caseStyleDisjoint(x *int) int {
	if x != nil {
		return 2
	}
	return 0
}

// caseStyleEnforcesAllApplicable returns 99 inside the `nonnil x`
// branch where one case applies. That case's return requirement
// `{ 1 | 2 | 3 }` rejects 99 and the sum-membership diagnostic
// fires. The `nil x` case does not apply here (its prereq does
// not hold), so it stays silent — case-style only enforces
// applicable cases.
//
// vow:cond nonnil -> 1 | 2 | 3
// vow:cond nil -> 0
func caseStyleEnforcesAllApplicable(x *int) int {
	if x != nil {
		return 99 // want `vow\[sentinel-error\]: return position 0: returning literal 99 but the position only accepts 1 \| 2 \| 3`
	}
	return 0
}

// caseStyleNoCaseHolds returns a value at a path where neither
// case's prereq is provably narrowed (the dominator walk cannot
// gate either branch via the bare `_ = x` reference). The
// under-approximation refuses to enforce any return requirement
// so no diagnostic fires. The fixture pins the "no case →
// unchecked" branch and guards against an accidental widening
// that would turn unrecognised control flow into a false
// positive.
//
// vow:cond nonnil -> 1 | 2 | 3
// vow:cond nil -> 0
func caseStyleNoCaseHolds(x *int) int {
	_ = x
	return 99
}

// caseStyleTrivialPrereqAccumulates carries one trivial-Prereq
// case (`* ->`) and one non-trivial vow:cond. The trivial case
// applies at every return so its return requirement
// `{ ErrFoo | nil }` enforces unconditionally; chain
// authorisation accumulates so ErrFoo is admitted for
// propagation. Returning ErrFoo inside `nonnil x` triggers no
// sum-membership diagnostic (both applicable cases list it) and
// no chain-auth leak (the trivial case authorises it).
//
// vow:cond * -> ErrFoo | nil
// vow:cond nonnil -> ErrFoo | nil
func caseStyleTrivialPrereqAccumulates(x *int) error {
	if x != nil {
		return ErrFoo
	}
	return nil
}
