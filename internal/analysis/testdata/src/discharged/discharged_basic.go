package discharged

import "errors"

// dischargedMakeErr returns an opaque error and is declared as a
// barrier callee. Caller-side verification stops at every call to
// this function.
// vow:discharged
func dischargedMakeErr() error {
	return errors.New("opaque")
}

// directReturn returns dischargedMakeErr's value without observing it.
// Without the barrier the value would be untracked; the barrier
// makes the absence of a diagnostic explicit so a future regression
// would surface as an unexpected leak emission.
func directReturn() error {
	return dischargedMakeErr()
}

// aliasedReturn binds the discharged return through a single alias
// before returning it. The barrier flows through alias resolution.
func aliasedReturn() error {
	e := dischargedMakeErr()
	return e
}
