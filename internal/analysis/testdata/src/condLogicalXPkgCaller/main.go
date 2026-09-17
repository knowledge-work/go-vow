// Package condLogicalXPkgCaller is the caller-side fixture for
// the cross-package logical-arrow vow:cond trace. The import
// target declares each per-call contract through a fact (see
// condLogicalXPkgCallee); the analyzer reads the fact through
// pass.ImportObjectFact and surfaces a caller-side diagnostic
// when the call's argument literals contradict the rule.
package condLogicalXPkgCaller

import "condLogicalXPkgCallee"

// callImplyDiagnose exercises the forward-implication contract
// across the package boundary. The first argument is a non-nil
// pointer (the address-of operator) and the second is the bare
// nil literal, so the implication's left-hand side holds and the
// right-hand side fails. The diagnostic surfaces at the call
// expression driven by the imported fact rather than by the
// callee's AST.
func callImplyDiagnose() {
	a := 1
	condLogicalXPkgCallee.RequireImply(&a, nil) // want `vow\[sentinel-error\]: call violates vow:cond x != nil => y != nil: x != nil holds at this call but y != nil does not`
}

// callImplySilentTrivial pairs the first argument as the nil
// literal so the implication's left-hand side fails and the
// rule holds trivially. The caller-side check stays silent.
func callImplySilentTrivial() {
	condLogicalXPkgCallee.RequireImply(nil, nil)
}

// callEquivDiagnose drives the biconditional contract: one slot
// is the address-of operator (non-nil) and the other is the bare
// nil literal, so the two sides diverge and the equivalence is
// contradicted.
func callEquivDiagnose() {
	a := 1
	condLogicalXPkgCallee.RequireEquiv(&a, nil) // want `vow\[sentinel-error\]: call violates vow:cond x != nil <=> y != nil: x != nil holds at this call but y != nil does not`
}

// callTagDiagnose pairs the matching tag literal with a nil
// payload so the literal comparison decides the left-hand side
// as true while the nil check decides the right-hand side as
// false. The implication is contradicted at the call site.
func callTagDiagnose() {
	condLogicalXPkgCallee.RequireTagPayload("live", nil) // want `vow\[sentinel-error\]: call violates vow:cond tag == "live" => payload != nil: tag == "live" holds at this call but payload != nil does not`
}

// callTagMismatchSilent supplies a non-matching tag literal so
// the comparison decides the left-hand side as false. The
// implication holds trivially and the caller-side check stays
// silent.
func callTagMismatchSilent() {
	condLogicalXPkgCallee.RequireTagPayload("draft", nil)
}
