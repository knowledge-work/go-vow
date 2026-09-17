package sentinels

// Fixtures for the statements the vow:cond prereq dead-guard pass reads:
// the run of `if` statements a body opens with. The first fixture pins
// where the run ends, the second that it spans consecutive `if`s.

// conditionGuardBelowAnotherStatement holds both halves of the comparison
// in one body. The first guard opens the body and is reported. The second
// sits below a statement that is not an `if`, which ends the run the pass
// reads, so nothing is reported for it. Reading that run as a filter over
// the whole body instead of a prefix of it makes the second guard report
// as well.
//
// Keeping both guards in one function is what makes the second one's
// silence attributable: they share the annotation, the parameter and the
// condition, so where each sits is what is left to explain why one reports
// and the other does not. It also ties the fixture to the annotation
// saying `nonnil` in particular, which a want on the trailing return alone
// does not, since that return is outside the sum under any predicate that
// the guard narrows.
//
// The first guard returns 0 because its then side proves x is nil, the
// opposite of the prereq, so the return requirement does not reach it. The
// second one is past the narrowing and has to return a listed value.
//
// vow:cond nonnil -> 1 | 2 | 3
func conditionGuardBelowAnotherStatement(x *int, limit int) int {
	if x == nil { // want `vow\[nil-safety\]: guard on x is dead; callee declared nonnil x in vow:cond`
		return 0
	}
	_ = limit
	if x == nil {
		return 1
	}
	return 99 // want `vow\[sentinel-error\]: return position 0: returning literal 99 but the position only accepts 1 \| 2 \| 3`
}

// conditionGuardSecondInLeadingRun places the dead guard second in the
// leading run. The run spans consecutive `if` statements, so the guard is
// still in reach and is reported. Reading only the first statement of the
// body instead of the whole leading run makes this fixture fall silent.
//
// This and conditionGuardBelowAnotherStatement bound the run from opposite
// ends, and neither stands in for the other: shortening the run to one
// statement leaves the other's second guard silent either way, and
// widening it to the whole body leaves this one reporting.
//
// vow:cond nonnil -> 1 | 2 | 3
func conditionGuardSecondInLeadingRun(x *int, limit int) int {
	if limit > 10 {
		return 3
	}
	if x == nil { // want `vow\[nil-safety\]: guard on x is dead; callee declared nonnil x in vow:cond`
		return 0
	}
	return 1
}
