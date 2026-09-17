package vowUse

// callerHandsOff passes the subject identifier to a discharge
// callee directly. Without the caller-side credit the subject
// reference at the argument site would surface as a leak; the
// callee's discharge contract makes the reference an observation
// instead.
func callerHandsOff() {
	directViaEquality(ErrFoo)
}
