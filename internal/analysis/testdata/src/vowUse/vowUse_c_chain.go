package vowUse

// chainHandoff defers discharge to another callee that already
// declares the same contract. The handoff path credits the
// discharge without requiring a direct subject reference in this
// function's body: the callee's declaration covers the contract.
// vow:use ErrFoo
func chainHandoff(err error) {
	directViaEquality(err)
}
