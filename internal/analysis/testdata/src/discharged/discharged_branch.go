package discharged

import "errors"

// dischargedBranchSource returns an opaque error through the barrier.
// vow:discharged
func dischargedBranchSource() error {
	return errors.New("opaque")
}

// branchMergePartialBarrier exercises a branch merge where one path
// returns a discharged value and the other returns the subject
// directly. The SSA backward walk visits both phi edges; the
// discharged edge is muted but the direct ErrFoo edge still surfaces
// as a leak so the barrier does not silently mask unrelated
// leakage on the merged value.
func branchMergePartialBarrier(c bool) error {
	var e error
	if c {
		e = dischargedBranchSource()
	} else {
		e = ErrFoo // want `vow\[sentinel-error\]: sentinel error ErrFoo leaked: needs observation or explicit propagation`
	}
	return e // want `vow\[sentinel-error\]: sentinel error ErrFoo leaked: needs observation or explicit propagation`
}
