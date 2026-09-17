package useHint

import "errors"

// subjectSpecificDropsOnlyNamed asserts discharge for ErrFoo only,
// even though both ErrFoo and ErrBar reach the must-consume engine
// in the surrounding function. The ErrBar leak still surfaces
// because the assertion is subject-keyed, not category-keyed — the
// hint promises "I used ErrFoo here", not "silence every must-
// consume diagnostic anywhere near this line".
func subjectSpecificDropsOnlyNamed(branch int) error {
	if branch == 0 {
		return ErrFoo // vow:use ErrFoo
	}
	return ErrBar // want `vow\[sentinel-error\]: sentinel error ErrBar leaked: needs observation or explicit propagation`
}

// subjectSpecificFuncOnlyNamed pins the same invariant at the
// function-doc scope: a function-level assertion that names only
// ErrFoo leaves ErrBar's leak in place. The body discharges ErrFoo
// through errors.Is so the callee self-validation accepts the
// declaration; the ErrBar leak fires because the function-level
// assertion never covered it.
//
// vow:use ErrFoo
func subjectSpecificFuncOnlyNamed(probe error, branch int) error {
	if errors.Is(probe, ErrFoo) {
		return nil
	}
	if branch == 0 {
		return ErrFoo
	}
	return ErrBar // want `vow\[sentinel-error\]: sentinel error ErrBar leaked: needs observation or explicit propagation`
}
