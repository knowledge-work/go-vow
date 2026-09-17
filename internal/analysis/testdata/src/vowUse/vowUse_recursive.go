package vowUse

// mutualA and mutualB form a handoff cycle: each function defers
// discharge to the other. Both are accepted independently because
// the analyzer credits the handoff on the callee's declaration
// rather than chasing a fix-point across the cycle; an invalid
// callee surfaces its own missing-discharge diagnostic separately.
// vow:use ErrFoo
func mutualA(err error) {
	mutualB(err)
}

// vow:use ErrFoo
func mutualB(err error) {
	mutualA(err)
}
