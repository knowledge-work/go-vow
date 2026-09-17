// Package condValueSum is the analysistest fixture for the
// value-level direct sum on a vow:cond comparison's right-hand
// side. The fixture exercises the two distributions the
// equality and inequality operators carry over a `|`-separated
// sum: `==` distributes as OR (any matching member proves the
// comparison held); `!=` distributes as AND by De Morgan (every
// member must differ for the comparison to hold).
//
// Each detection path pairs a silent baseline that holds the
// rule with a diagnose case that violates the rule from the
// matching distribution branch. The fixtures use integer
// members because the per-rule evaluator commits on the
// integer literal path the existing condRangeNarrow fixture
// also drives.
package condValueSum

// sumEqLhsHeldRhsHoldsSilent declares an implication whose
// left-hand side is a `==` sum over two integer literals. The
// call path narrows the parameter to the literal 0, which
// matches the first member, so the OR distribution decides the
// lhs as true. The return statement yields a non-nil pointer
// so the rhs holds and the walker stays silent.
//
// vow:cond op == 0 | 1 => out != nil
func sumEqLhsHeldRhsHoldsSilent(op int) (out *int) {
	if op == 0 {
		x := 1
		out = &x
		return out
	}
	x := 2
	out = &x
	return out
}

// sumEqLhsHeldRhsViolatesDiagnose mirrors the silent baseline
// but returns nil from inside the guarded block. The lhs
// commits as true through the OR-distribution match against
// member 0; the rhs evaluates as nil through the AST-literal
// path so the implication fails and the walker reports the
// violation at the offending return.
//
// vow:cond op == 0 | 1 => out != nil
func sumEqLhsHeldRhsViolatesDiagnose(op int) (out *int) {
	if op == 0 {
		return nil // want `vow\[sentinel-error\]: return violates vow:cond op == 0 \| 1 => out != nil: op == 0 \| 1 holds at this return but out != nil does not`
	}
	x := 2
	out = &x
	return out
}

// sumEqLhsOutsideSetSilent guards on an integer the sum does
// not list. The OR distribution decides the lhs as false at
// the inside-guard return, so the forward implication stays
// vacuously true and the walker stays silent even when the
// return value is nil.
//
// vow:cond op == 0 | 1 => out != nil
func sumEqLhsOutsideSetSilent(op int) (out *int) {
	if op == 2 {
		return nil
	}
	x := 3
	out = &x
	return out
}

// sumNeqRhsHoldsSilent declares an implication whose right-
// hand side is a `!=` sum: the rule holds when the return
// differs from every member of the sum. The function returns
// 5, which differs from both 1 and 2, so the AND distribution
// commits as true.
//
// vow:cond op == 0 => $1 != 1 | 2
func sumNeqRhsHoldsSilent(op int) int {
	if op == 0 {
		return 5
	}
	return 0
}

// sumNeqRhsViolatesDiagnose mirrors the silent baseline but
// returns 1 from the guarded block. The lhs commits as true
// through the equality match on 0; the rhs sums `1 | 2` and
// the AND distribution fails because the return matches the
// first member, so the implication fails and the walker
// reports the violation at the offending return.
//
// vow:cond op == 0 => $1 != 1 | 2
func sumNeqRhsViolatesDiagnose(op int) int {
	if op == 0 {
		return 1 // want `vow\[sentinel-error\]: return violates vow:cond op == 0 => \$1 != 1 \| 2: op == 0 holds at this return but \$1 != 1 \| 2 does not`
	}
	return 5
}

// sumEqWithNilMemberSilent mixes a literal member with the
// bare `nil` token on the right-hand side. The rule holds
// when the return matches any member of the sum; a nil return
// matches the nil member and decides the lhs as true.
//
// vow:cond op == 0 => out == nil | 0
func sumEqWithNilMemberSilent(op int) (out *int) {
	if op == 0 {
		return nil
	}
	x := 1
	out = &x
	return out
}
