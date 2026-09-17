package sentinels

// Fixtures for phi-aware path-merge narrowing.
//
// When all incoming edges to a merge block independently prove
// a fact for a parameter, the fact holds at the merge even
// though no single dominator carries the proof on its own. The
// matcher recognises this join pattern in addition to the
// dominator-walk path: both branches of an outer if-else can
// guard against the same parameter shape before flowing into a
// shared continuation block.

// phiNarrowJoinAuthorisesAtMergeReturn carries a non-trivial
// case whose parameter requirement `nonnil err` is proved at the
// merge return via two independent `if err == nil { return nil }`
// guards — each arm of the outer if eliminates the nil case
// before flowing into the merge. The chain authoriser admits
// ErrFoo at the merge return.
//
// vow:cond nonnil -> ErrFoo | nil
func phiNarrowJoinAuthorisesAtMergeReturn(err error, cond bool) error {
	if cond {
		if err == nil {
			return nil
		}
	} else {
		if err == nil {
			return nil
		}
	}
	return ErrFoo
}

// phiNarrowJoinPartialPathLeaks shows that a partial proof does
// not suffice: only the `cond=true` arm guards against
// `err == nil`, so the merge has one predecessor (the
// `cond=false` arm) that lacks the proof. The join fails, the
// chain authoriser refuses ErrFoo, and the leak detector fires.
//
// vow:cond nonnil -> ErrFoo | nil
func phiNarrowJoinPartialPathLeaks(err error, cond bool) error {
	if cond {
		if err == nil {
			return nil
		}
	}
	return ErrFoo // want `vow\[sentinel-error\]: sentinel error ErrFoo leaked: needs observation or explicit propagation`
}

// phiNarrowJoinAcrossSwitchArms exercises the join across the
// arms of a switch — each non-default arm guards against the
// nil case before flowing into the post-switch merge, so the
// merge sees `nonnil err`. The shape mirrors the if-else join
// above; the SSA package lowers the switch to a chain of
// `if x == const` instructions and the merge follows the same
// rule as a hand-written if-else merge.
//
// vow:cond nonnil -> ErrFoo | nil
func phiNarrowJoinAcrossSwitchArms(err error, tag int) error {
	switch tag {
	case 0:
		if err == nil {
			return nil
		}
	case 1:
		if err == nil {
			return nil
		}
	default:
		if err == nil {
			return nil
		}
	}
	return ErrFoo
}
