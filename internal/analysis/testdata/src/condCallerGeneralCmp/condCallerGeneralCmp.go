// Package condCallerGeneralCmp pins the caller-side dead-guard
// recogniser's behaviour on a `vow:cond` rule whose two
// operands are equality checks against non-nil literals. The
// nil-check axis handled by condCallerDeadGuard /
// condCallerNegativeForm is the special case where the
// literal is the nil marker; this package exercises the same
// dead-guard flow against integer literals so the general
// equality reasoning stays proven under the same truth table
// (redundant when the guard's outcome is already implied by
// the narrow, impossible when it contradicts).
package condCallerGeneralCmp

// Payload is the pointer the fixture calls narrow through.
type Payload struct{ v int }

// LookupPayload ties status and $1 with a general equality
// forward implication: when the callee's status return equals
// 1, the payload is non-nil. A caller who proves the same
// antecedent via a guard on status inherits the non-nil
// narrow on the paired payload.
//
// vow:cond status == 1 => $1 != nil
func LookupPayload() (payload *Payload, status int) { return &Payload{}, 1 } // want LookupPayload:"vow:cond\\(status == 1 => \\$1 != nil\\)"

// callerRedundantOuterBody exercises the body-narrow carry on
// the general equality axis: the outer guard proves status
// equals 1, which is the rule's antecedent, so the payload is
// narrowed to non-nil inside the body. The inner `!= nil`
// guard is redundant against that narrow.
func callerRedundantOuterBody() {
	payload, status := LookupPayload()
	if status == 1 {
		if payload != nil { // want `vow\[nil-safety\]: guard on payload is dead \(redundant\); vow:cond rule .* on LookupPayload narrows return position 1 to non-nil once the guard on status short-circuits`
			return
		}
	}
	_ = payload
}

// callerImpossibleOuterBody mirrors the redundant case with the
// opposite inner comparison: the payload is proven non-nil, so
// an `if payload == nil { return }` guard inside the body is
// impossible.
func callerImpossibleOuterBody() {
	payload, status := LookupPayload()
	if status == 1 {
		if payload == nil { // want `vow\[nil-safety\]: guard on payload is dead \(impossible\); vow:cond rule .* on LookupPayload narrows return position 1 to non-nil once the guard on status short-circuits`
			return
		}
	}
	_ = payload
}

// callerRedundantFallThrough exercises the fall-through carry:
// the short-circuit removes the `status != 1` branch, so the
// continuation runs under `status == 1` — the rule's antecedent
// — and the payload is narrowed to non-nil. The follow-up
// `!= nil` guard is redundant against that narrow.
func callerRedundantFallThrough() {
	payload, status := LookupPayload()
	if status != 1 {
		return
	}
	if payload != nil { // want `vow\[nil-safety\]: guard on payload is dead \(redundant\); vow:cond rule .* on LookupPayload narrows return position 1 to non-nil once the guard on status short-circuits`
		return
	}
	_ = payload
}

// callerImpossibleFallThrough exercises the opposite guard on
// the fall-through path: the payload is proven non-nil after
// the short-circuit, so a `== nil` guard is impossible.
func callerImpossibleFallThrough() {
	payload, status := LookupPayload()
	if status != 1 {
		return
	}
	if payload == nil { // want `vow\[nil-safety\]: guard on payload is dead \(impossible\); vow:cond rule .* on LookupPayload narrows return position 1 to non-nil once the guard on status short-circuits`
		return
	}
	_ = payload
}

// callerConverseSilent pins the converse direction as silent.
// The rule `status == 1 => $1 != nil` does not guarantee its
// converse `$1 != nil => status == 1`; a guard that proves
// the payload non-nil says nothing about status. The recogniser
// must stay silent on the follow-up guard on status.
func callerConverseSilent() {
	payload, status := LookupPayload()
	if payload != nil {
		if status == 1 {
			return
		}
	}
	_ = payload
	_ = status
}

// callerInverseSilent pins the inverse direction as silent.
// The rule `status == 1 => $1 != nil` says nothing about the
// `status != 1` branch, so a follow-up guard on the payload
// inside that branch must not be reported.
func callerInverseSilent() {
	payload, status := LookupPayload()
	if status != 1 {
		if payload == nil {
			return
		}
	}
	_ = payload
}

// callerDifferentLiteralDecidableButAnchorLimited pins two
// separate facts on the same fixture — the truth table
// classification and the caller-narrow anchor scope — so a
// future maintainer reads both layers from the same case.
//
// Truth-table layer: the outer guard proves `status == 1`, so
// under the deadGuardOutcome table the follow-up
// `if status == 2 { return }` sits in the Eq-narrow row with a
// value-mismatch column (case 2), which is `impossible` —
// status pinned to 1 cannot also equal 2.
//
// Anchor layer: the caller-narrow pass keys its bindings on
// the rule's target reference. In this rule (`status == 1 =>
// $1 != nil`) `status` is the guard-side reference (rule
// LHS), not the target — the target is `$1` bound to
// `payload`. So the pass records a pending binding on
// `payload`, not on `status`, and a follow-up guard on
// `status` does not read a narrowed binding at all. The
// truth-table row is `impossible`, but the pass is silent
// because there is no `status` binding to fire against.
//
// The fixture therefore stays silent under Stack 3a; a future
// extension that adds guard-side narrow tracking would flip
// it to fire without changing the outer truth table.
func callerDifferentLiteralDecidableButAnchorLimited() {
	_, status := LookupPayload()
	if status == 1 {
		if status == 2 {
			return
		}
	}
	_ = status
}
