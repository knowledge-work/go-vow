package useHint

import "errors"

// funcUseAsserted asserts discharge for ErrFoo across every leak
// site in the function body. The body discharges ErrFoo through
// errors.Is on the input probe; the trailing returns leak ErrFoo
// but are silenced by the function-level vow:use marker so the
// fixture reads off the suppression behaviour without any
// must-consume diagnostic surviving.
//
// vow:use ErrFoo
func funcUseAsserted(probe error, branch int) error {
	if errors.Is(probe, ErrFoo) {
		return nil
	}
	if branch == 0 {
		return ErrFoo
	}
	return ErrFoo
}

// funcUseAssertedMulti asserts discharge for two subjects at once.
// The comma-separated payload accumulates into the function's
// asserted-subject set, so leaks on either ErrFoo or ErrBar in the
// body are silenced. The body discharges both subjects through
// errors.Is so the callee self-validation accepts the declaration.
//
// vow:use ErrFoo, ErrBar
func funcUseAssertedMulti(probe error, branch int) error {
	if errors.Is(probe, ErrFoo) || errors.Is(probe, ErrBar) {
		return nil
	}
	if branch == 0 {
		return ErrFoo
	}
	return ErrBar
}
