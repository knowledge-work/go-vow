// Package condRangeNarrow is the analysistest fixture for the
// integer range narrowing layer that refines an ordered or
// equality comparison against an integer literal through the
// SSA dominator walk. The fixture exercises four detection
// paths the layer surfaces: a covered range proves the rule
// holds (silent baseline), a covered range proves the rule
// violates (diagnose), an unbounded range leaves the rule
// undecided (silent), and an equality guard pins the range to
// a single point so the rule commits on the matching constant
// (silent + diagnose). Each diagnose case pairs with a silent
// baseline that pins the no-false-positive invariant for the
// same detection path.
package condRangeNarrow

// rangeNarrowedRuleHoldsSilent declares an implication whose
// left-hand side reads a parameter the dominator walk pins to
// `[2, +∞)` on the dominating branch. The return statement
// sits inside the guarded block so the SSA refinement commits
// the lhs as true; the rhs evaluates to a non-nil shape
// through the AST-literal path so the implication holds and
// the walker stays silent.
//
// vow:cond v >= 2 => out != nil
func rangeNarrowedRuleHoldsSilent(v int) (out *int) {
	if v >= 2 {
		x := 1
		out = &x
		return out
	}
	return nil
}

// rangeNarrowedRuleViolatesDiagnose mirrors the silent
// baseline but returns a nil pointer from inside the guarded
// block. The SSA refinement still commits the lhs as true;
// the rhs evaluates to nil through the AST-literal path so
// the implication fails and the walker reports the violation
// at the offending return statement.
//
// vow:cond v >= 2 => out != nil
func rangeNarrowedRuleViolatesDiagnose(v int) (out *int) {
	if v >= 2 {
		return nil // want `vow\[sentinel-error\]: return violates vow:cond v >= 2 => out != nil: v >= 2 holds at this return but out != nil does not`
	}
	x := 1
	out = &x
	return out
}

// rangeNarrowedVacuousLhsSilent returns nil from inside an
// `if v < 2` guard so the SSA refinement commits the rule's
// lhs `v >= 2` as false at that return statement. The
// forward-implication shape stays vacuously true under a
// false lhs so the walker reports no violation even though
// the rhs is nil. The fall-through return reaches the rule
// as a true-lhs / non-nil-rhs pair so the second return
// holds the rule directly.
//
// vow:cond v >= 2 => out != nil
func rangeNarrowedVacuousLhsSilent(v int) (out *int) {
	if v < 2 {
		return nil
	}
	x := 1
	out = &x
	return out
}

// rangeUnboundedSilent omits any guard around the return
// statement so the dominator walk reaches the function entry
// without intersecting an interval against the parameter.
// The accumulating bound stays empty so the lhs reports
// undecided; the rule walker drops the rule for this return
// and stays silent.
//
// vow:cond v >= 2 => out != nil
func rangeUnboundedSilent(v int) (out *int) {
	_ = v
	x := 1
	out = &x
	return out
}

// rangeEqualityGuardRuleHoldsSilent pins both ends of the
// interval to the same constant through an equality guard.
// The dominator walk pins `v == 3` to the [3, 3] interval on
// the dominating side and the rule's `v == 3` lhs commits as
// true; the non-nil rhs holds so the walker stays silent.
//
// vow:cond v == 3 => out != nil
func rangeEqualityGuardRuleHoldsSilent(v int) (out *int) {
	if v == 3 {
		x := 1
		out = &x
		return out
	}
	return nil
}

// rangeEqualityGuardRuleViolatesDiagnose mirrors the silent
// baseline but returns a nil pointer from inside the
// equality-guarded block. The lhs commits as true and the rhs
// evaluates to nil so the implication fails and the walker
// reports the violation.
//
// vow:cond v == 3 => out != nil
func rangeEqualityGuardRuleViolatesDiagnose(v int) (out *int) {
	if v == 3 {
		return nil // want `vow\[sentinel-error\]: return violates vow:cond v == 3 => out != nil: v == 3 holds at this return but out != nil does not`
	}
	x := 1
	out = &x
	return out
}
