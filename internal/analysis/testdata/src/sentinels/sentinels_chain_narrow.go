package sentinels

// Fixtures for chain-authorisation narrowing.
//
// A non-trivial parameter-requirement case authorises chain
// propagation of the subjects listed in its return requirement,
// scoped to the returns whose path proves the parameter
// requirement. Returns outside the narrowed scope do not see the
// authorisation and the leak detector fires unless another case
// (typically a trivial-parameter-requirement `vow:cond`) lists
// the subject independently.

// chainNarrowAuthorisesInsideBranch carries a single non-trivial
// case that authorises ErrFoo only when the path proves
// `nonnil x`. The `return ErrFoo` inside the `if x != nil` branch
// sits in the narrowed scope, the chain authoriser admits the
// subject, and the leak detector stays silent. The return
// outside the branch (the implicit `x == nil` path) is not
// reachable from this control-flow shape — the function returns
// from inside the branch only — so the fixture pins the
// positive narrowing case in isolation.
//
// vow:cond nonnil -> ErrFoo | nil
func chainNarrowAuthorisesInsideBranch(x *int) error {
	if x != nil {
		return ErrFoo
	}
	return nil
}

// chainNarrowRefusesOutsideBranch returns ErrFoo at a path where
// the parameter requirement `nonnil x` does not hold (the dominator
// walk recognises the `if x == nil` then-side as the matching
// branch for `nil x`, the opposite of the requirement). With no
// other authorising case, the chain authoriser refuses the
// propagation and the leak detector fires.
//
// vow:cond nonnil -> ErrFoo | nil
func chainNarrowRefusesOutsideBranch(x *int) error {
	if x == nil { // want `vow\[nil-safety\]: guard on x is dead; callee declared nonnil x in vow:cond`
		return ErrFoo // want `vow\[sentinel-error\]: sentinel error ErrFoo leaked: needs observation or explicit propagation`
	}
	return nil
}

// chainNarrowDisjointCases shows two non-trivial cases with
// disjoint parameter requirements: one authorises ErrFoo under
// `nonnil x`, the other authorises ErrBar under `nil x`. Each
// return sits inside exactly one matching case and the chain
// authoriser admits the corresponding subject. No leak fires.
//
// vow:cond nonnil -> ErrFoo | nil
// vow:cond nil  -> ErrBar | nil
func chainNarrowDisjointCases(x *int) error {
	if x != nil {
		return ErrFoo
	}
	return ErrBar
}
