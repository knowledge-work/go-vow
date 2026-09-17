package sentinels

// The fixture below pins the case-style accumulation rule.
// Multiple vow:cond lines stay independent (the "last successful
// parse wins" convention is not in effect); each line is an
// independent case and applicable cases all enforce their return
// requirements. Chain authorisation likewise unions every
// trivial-Prereq case's admission set.

// rulesMarkerAccumulates carries an earlier trivial-Prereq
// vow:cond that authorises ErrFoo and a later vow:cond that
// narrows the return requirement to the `nonnil x` branch. The
// trivial-Prereq rule continues to authorise ErrFoo for chain
// propagation at every return; the non-trivial vow:cond
// enforces its return requirement inside the `nonnil x` branch
// (where returning ErrFoo trips no diagnostic because ErrFoo is
// listed in the return-requirement sum). No diagnostic fires.
//
// vow:cond * -> ErrFoo | nil
// vow:cond nonnil -> ErrFoo | nil
func rulesMarkerAccumulates(x *int) error {
	if x != nil {
		return ErrFoo
	}
	return nil
}
