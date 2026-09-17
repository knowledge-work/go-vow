// Package condInconsistency is the analysistest fixture for
// the rule-set inconsistency layer. The fixture exercises three
// detection paths the layer surfaces: contradictory rule pairs
// (cross-rule contradiction), rules whose operands the
// signature decides into a violating combination (signature
// cross-check), and signature positions the rules narrow only
// to nil across multiple author-written implications (nil-only
// induction hint). Each diagnose case pairs with a silent
// baseline that pins the no-false-positive invariant for the
// same detection path.
package condInconsistency

// implyContradictoryRhsSilent declares two implications whose
// right-hand sides agree on the same operand: under a matching
// left-hand side both rules demand the same outcome, so no
// contradiction surfaces. The silent baseline pins that the
// walker reports only on the negated-RHS shape and not on
// every shared-LHS pair.
//
// vow:cond a != nil => b != nil ; a != nil => b != nil
func implyContradictoryRhsSilent(a, b *int) {
	_ = a
	_ = b
}

// implyContradictoryRhsDiagnose declares two implications that
// share their left-hand side but require opposite right-hand
// side truth on the same operand. The walker reports the
// contradiction once at the function name; no return or call
// site is involved because the offence belongs to the rule
// set rather than to any single evaluation point.
//
// vow:cond a != nil => b != nil ; a != nil => b == nil
func implyContradictoryRhsDiagnose(a, b *int) { // want `vow\[sentinel-error\]: contradictory vow:cond rules on implyContradictoryRhsDiagnose: vow:cond a != nil => b != nil and vow:cond a != nil => b == nil share their left-hand side but require opposite right-hand side truth`
	_ = a
	_ = b
}

// equivContradictoryRhsDiagnose declares two biconditionals that
// share their left-hand side but require opposite right-hand
// side truth. The walker treats biconditional contradictions
// through the same pairwise check the forward-implication
// direction uses because both directions demand RHS truth
// align with LHS truth under a matching left-hand side.
//
// vow:cond a != nil <=> b != nil ; a != nil <=> b == nil
func equivContradictoryRhsDiagnose(a, b *int) { // want `vow\[sentinel-error\]: contradictory vow:cond rules on equivContradictoryRhsDiagnose: vow:cond a != nil <=> b != nil and vow:cond a != nil <=> b == nil share their left-hand side but require opposite right-hand side truth`
	_ = a
	_ = b
}

// mixedDirectionContradictoryDiagnose pairs a forward
// implication with a biconditional that shares its left-hand
// side and negates its right-hand side. The mixed-direction
// pair contradicts at the RHS level because a forward
// implication requires RHS hold whenever LHS holds and a
// biconditional strengthens that to equivalence; both demand
// the same RHS truth under matching LHS.
//
// vow:cond a != nil => b != nil ; a != nil <=> b == nil
func mixedDirectionContradictoryDiagnose(a, b *int) { // want `vow\[sentinel-error\]: contradictory vow:cond rules on mixedDirectionContradictoryDiagnose: vow:cond a != nil => b != nil and vow:cond a != nil <=> b == nil share their left-hand side but require opposite right-hand side truth`
	_ = a
	_ = b
}

// chainContradictoryDiagnose pairs three implications whose
// chain through the b-axis and through the c-axis each surface a
// contradiction with one of the author-written rules. The
// transitive closure layer derives `a != nil => c != nil`
// (chained from the first two rules) and `a != nil => b == nil`
// (chained from the third rule and the contrapositive of the
// second) so two pair-level contradictions surface: one at the
// b-axis between the first author rule and the chain through
// the third, and one at the c-axis between the chain through
// the first two and the third author rule. The two diagnostics
// announce distinct axes (b-axis and c-axis) under the same
// hypothesis (`a != nil`), so the root-axis dedup keeps them
// both visible because the right-hand operand pair differs.
//
// vow:cond a != nil => b != nil ; b != nil => c != nil ; a != nil => c == nil
func chainContradictoryDiagnose(a, b, c *int) { // want `vow\[sentinel-error\]: contradictory vow:cond rules on chainContradictoryDiagnose: vow:cond a != nil => b != nil and transitive closure of vow:cond a != nil => c == nil and contrapositive of vow:cond b != nil => c != nil share their left-hand side but require opposite right-hand side truth` `vow\[sentinel-error\]: contradictory vow:cond rules on chainContradictoryDiagnose: transitive closure of vow:cond a != nil => b != nil and vow:cond b != nil => c != nil and vow:cond a != nil => c == nil share their left-hand side but require opposite right-hand side truth`
	_ = a
	_ = b
	_ = c
}

// distinctLhsSilent declares two implications whose left-hand
// sides differ. The walker reports nothing because the
// contradiction predicate requires a matching LHS to align two
// rule outcomes under the same hypothesis. The silent baseline
// pins that the walker does not over-report on every
// negated-RHS pair regardless of LHS.
//
// vow:cond a != nil => b != nil ; c != nil => b == nil
func distinctLhsSilent(a, b, c *int) {
	_ = a
	_ = b
	_ = c
}

// comparisonContradictoryDiagnose pairs two implications whose
// right-hand sides compare the same operand against the same
// literal under negated operators. The pairwise check negates
// the second rule's RHS through the comparison-operator flip
// helper and compares it to the first rule's RHS, so the
// contradiction surfaces under the same predicate that drives
// the nil-check shape.
//
// vow:cond tag == "live" => count == 0 ; tag == "live" => count != 0
func comparisonContradictoryDiagnose(tag string, count int) { // want `vow\[sentinel-error\]: contradictory vow:cond rules on comparisonContradictoryDiagnose: vow:cond tag == "live" => count == 0 and vow:cond tag == "live" => count != 0 share their left-hand side but require opposite right-hand side truth`
	_ = tag
	_ = count
}

// nilDeclMatchSilent declares both parameters as non-nil and
// pairs them with a rule the signature satisfies: under the `!`
// contract `a != nil` reads as true at the function boundary,
// and `b != nil` also reads as true, so the implication holds
// and the walker reports nothing. The silent baseline pins the
// no-false-positive invariant for the two-decisive-operand
// check.
//
// vow:nil (!,!)
// vow:cond a != nil => b != nil
func nilDeclMatchSilent(a, b *int) { // want nilDeclMatchSilent:"nilDecl\\(!,!\\)"
	_ = a
	_ = b
}

// nilDeclContradictsRhsDiagnose declares both parameters as
// non-nil but pairs them with an implication that demands the
// opposite. Under the `!` contract `a != nil` reads as true and
// `b == nil` reads as false, so the forward implication fails
// at the function boundary and the walker reports the
// inconsistency.
//
// vow:nil (!,!)
// vow:cond a != nil => b == nil
func nilDeclContradictsRhsDiagnose(a, b *int) { // want nilDeclContradictsRhsDiagnose:"nilDecl\\(!,!\\)" `vow\[sentinel-error\]: vow:cond rule on nilDeclContradictsRhsDiagnose contradicts signature: vow:cond a != nil => b == nil: a != nil holds at this function boundary but b == nil does not`
	_ = a
	_ = b
}

// nilDeclContradictsEquivDiagnose declares both parameters as
// non-nil and pairs them with a biconditional that demands they
// share a nil shape. Under the contract `a == nil` reads as
// false and `b != nil` reads as true; the biconditional fails
// because the two sides disagree at the function boundary.
//
// vow:nil (!,!)
// vow:cond a == nil <=> b != nil
func nilDeclContradictsEquivDiagnose(a, b *int) { // want nilDeclContradictsEquivDiagnose:"nilDecl\\(!,!\\)" `vow\[sentinel-error\]: vow:cond rule on nilDeclContradictsEquivDiagnose contradicts signature: vow:cond a == nil <=> b != nil: b != nil holds at this function boundary but a == nil does not`
	_ = a
	_ = b
}

// nilDeclNullableIndecisiveSilent declares both parameters as
// nillable. The signature cross-check stays silent because a
// `?` declaration permits either nil shape, so neither operand
// commits to a fixed truth at the function boundary and the
// two-decisive-operand requirement is not met. The silent
// baseline pins that nullable positions never trigger the
// signature-contradiction diagnostic.
//
// vow:nil (?,?)
// vow:cond a != nil => b != nil
func nilDeclNullableIndecisiveSilent(a, b *int) { // want nilDeclNullableIndecisiveSilent:"nilDecl\\(\\?,\\?\\)"
	_ = a
	_ = b
}

// nilOnlySingleNarrowSilent narrows the second parameter only
// to nil through a single forward implication. The induction
// hint requires the narrowing pattern across at least two
// rules; a single rule is a legitimate design choice and the
// walker stays silent. The silent baseline pins the threshold
// gate that keeps the hint from firing on every one-rule
// nil narrow.
//
// vow:cond a != nil => b == nil
func nilOnlySingleNarrowSilent(a, b *int) {
	_ = a
	_ = b
}

// nilOnlyMixedNarrowSilent narrows the second parameter to nil
// through one rule and to non-nil through another. The
// induction hint requires the non-nil narrowing count to stay
// at zero; the presence of any rule that narrows the position
// to non-nil cancels the hint because the position is not
// nil-only across the rule set. The silent baseline pins the
// no-nil-only invariant.
//
// vow:cond a != nil => b == nil ; c != nil => b != nil
func nilOnlyMixedNarrowSilent(a, b, c *int) {
	_ = a
	_ = b
	_ = c
}

// nilOnlyInductionDiagnose narrows the second parameter to nil
// through two distinct forward implications and never to
// non-nil. The induction hint surfaces at the function
// declaration with a suggestion to add a vow:nil declaration
// pinning the position as nullable so the contract the rules
// already enforce becomes explicit on the signature line.
//
// vow:cond a != nil => b == nil ; c != nil => b == nil
func nilOnlyInductionDiagnose(a, b, c *int) { // want `vow\[sentinel-error\]: b of nilOnlyInductionDiagnose is narrowed only to nil by 2 vow:cond rules; consider declaring vow:nil for b`
	_ = a
	_ = b
	_ = c
}

// nilOnlyAlreadyDeclaredSilent narrows the second parameter
// only to nil across two rules but the signature already
// declares the position as nullable. The induction hint stays
// silent because an explicit declaration already pins the
// contract; the hint would be redundant noise.
//
// vow:nil (!,?)
// vow:cond a != nil => b == nil ; a == nil => b == nil
func nilOnlyAlreadyDeclaredSilent(a, b *int) { // want nilOnlyAlreadyDeclaredSilent:"nilDecl\\(!,\\?\\)"
	_ = a
	_ = b
}

// holder is the receiver type for the receiver-axis fixtures
// below: a value with a single pointer field exercises the
// receiver name resolution path through `signatureNameContext`.
type holder struct{ field *int }

// nilOnlyReceiverInductionDiagnose narrows the receiver only to
// nil across two rules and never to non-nil. The receiver name
// resolves through the same signature-position predicate the
// parameter path uses, so the induction hint surfaces on the
// receiver identifier just like it surfaces on a regular
// parameter.
//
// vow:cond p != nil => h == nil ; q != nil => h == nil
func (h *holder) nilOnlyReceiverInductionDiagnose(p, q *int) { // want `vow\[sentinel-error\]: h of nilOnlyReceiverInductionDiagnose is narrowed only to nil by 2 vow:cond rules; consider declaring vow:nil for h`
	_ = h
	_ = p
	_ = q
}

// positionalReturnInductionDiagnose narrows the first named
// return only to nil through the `$1` positional reference
// across two rules. The positional form resolves against the
// return list through the same signature-position predicate the
// identifier form uses, so the induction hint surfaces on the
// `$1` reference. The $N form is valid for return slots; an
// out-of-range positional reference would not surface as a
// signature-position candidate.
//
// vow:cond p != nil => $1 == nil ; q != nil => $1 == nil
func positionalReturnInductionDiagnose(p, q *int) (out *int) { // want `vow\[sentinel-error\]: \$1 of positionalReturnInductionDiagnose is narrowed only to nil by 2 vow:cond rules; consider declaring vow:nil for \$1`
	_ = p
	_ = q
	return out
}

// namedReturnInductionDiagnose narrows a named return only to
// nil through its declared identifier across two rules. The
// named-return path shares the same signature-position
// predicate the parameter and receiver paths use; the
// diagnostic anchors on the identifier the author wrote.
//
// vow:cond p != nil => result == nil ; q != nil => result == nil
func namedReturnInductionDiagnose(p, q *int) (result *int) { // want `vow\[sentinel-error\]: result of namedReturnInductionDiagnose is narrowed only to nil by 2 vow:cond rules; consider declaring vow:nil for result`
	_ = p
	_ = q
	return result
}

// postfixPathSilent narrows a postfix-chained reference only to
// nil across two rules. The induction hint stays silent because
// the signature contract attaches to the head position rather
// than to a field below it; a chained reference is not a
// candidate for a signature-level declaration on the head slot
// alone. The silent baseline pins the head-only invariant.
//
// vow:cond p != nil => h.field == nil ; q != nil => h.field == nil
func postfixPathSilent(h *holder, p, q *int) {
	_ = h
	_ = p
	_ = q
}

// contrapositiveSelfContradictionDiagnose pairs two implications
// whose b-axis contracts collapse through the contrapositive
// derivation: the contrapositive of `b != nil => a == nil` is
// `a != nil => b == nil`, which shares the a-axis left-hand
// side with the original `a != nil => b != nil` and demands the
// opposite b-axis right-hand side truth. The diagnostic
// surfaces through the original-and-derived-contrapositive
// pairing the contradiction walker already drives so the
// author sees the unsatisfiable shape at the function name.
//
// vow:cond a != nil => b != nil ; b != nil => a == nil
func contrapositiveSelfContradictionDiagnose(a, b *int) { // want `vow\[sentinel-error\]: contradictory vow:cond rules on contrapositiveSelfContradictionDiagnose: vow:cond a != nil => b != nil and contrapositive of vow:cond b != nil => a == nil share their left-hand side but require opposite right-hand side truth`
	_ = a
	_ = b
}

// longChainSilent declares a four-rule chain whose closure walks
// `a != nil => b != nil`, `b != nil => c != nil`, `c != nil =>
// d != nil`, `d != nil => e != nil` without any self-referential
// or negated terminal. The closure produces every transitive
// rule along the chain, none of which contradict the author-
// written rules. The silent baseline pins the no-false-positive
// invariant on long chains so the contradiction walker stays
// quiet whenever the chain agrees on the same polarity end-to-
// end.
//
// vow:cond a != nil => b != nil ; b != nil => c != nil ; c != nil => d != nil ; d != nil => e != nil
func longChainSilent(a, b, c, d, e *int) {
	_ = a
	_ = b
	_ = c
	_ = d
	_ = e
}

// nilDeclEquivBothNegatedSilent pairs a non-nil signature
// declaration with a biconditional whose operands both read as
// false at the function boundary. The signature decides
// `a == nil` as false and `b == nil` as false, and a
// biconditional holds when its sides share their truth, so two
// false sides agree and the cross-check stays silent. The
// silent baseline pins the no-false-positive invariant for the
// cross-check on biconditionals whose operands negate the
// declared polarity symmetrically.
//
// vow:nil (!,!)
// vow:cond a == nil <=> b == nil
func nilDeclEquivBothNegatedSilent(a, b *int) { // want nilDeclEquivBothNegatedSilent:"nilDecl\\(!,!\\)"
	_ = a
	_ = b
}
