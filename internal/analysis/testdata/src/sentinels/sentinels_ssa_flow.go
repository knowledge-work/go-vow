package sentinels

import "errors"

// The SSA-driven flow detector extends leak detection to value
// flows that the AST walk in flow.go cannot reach: multi-hop
// aliases, reassignment after a non-subject initialiser, and branch
// merges. Each scenario below isolates one of those shapes and pins
// the expected diagnostic at the return expression so the SSA pass
// can be evaluated independently of the per-Ident walk.

// sentinelForFlow is a fresh subject local to this file. Reusing
// ErrNotFound or the other shared sentinels would entangle the new
// fixture with the existing single-hop expectations.
//
// vow:define @Sentinel
var sentinelForFlow = errors.New("ssa-flow")

// multiHopLeak exercises the multi-hop alias chain. The per-Ident
// AST walk skips `sentinelForFlow` (RHS of `x := ...`, suppressed by
// isAliasBindingRHS) and skips `x` (also RHS of `y := x`). The
// one-hop AST resolver then sees `y` having a single define
// `y := x`, but x is a local — not a tracked subject — so the AST
// resolver gives up. The SSA pass walks the chain to its source and
// reports.
func multiHopLeak() error {
	x := sentinelForFlow
	y := x
	return y // want `vow\[sentinel-error\]: sentinel error sentinelForFlow leaked: needs observation or explicit propagation`
}

// reassignAfterInitLeak exercises reassignment after a non-subject
// initialiser. The local `e` is defined from a call expression (no
// Ident leak at the RHS), then reassigned to the sentinel via `=`.
// The AST walk reports `sentinelForFlow` at the `=` RHS, but the
// return expression itself is left undecorated because the AST
// resolver only follows the single `:=` define, whose RHS is not an
// Ident. SSA carries the leak forward to the return.
func reassignAfterInitLeak(label string) error {
	e := errors.New(label)
	e = sentinelForFlow // want `vow\[sentinel-error\]: sentinel error sentinelForFlow leaked: needs observation or explicit propagation`
	return e            // want `vow\[sentinel-error\]: sentinel error sentinelForFlow leaked: needs observation or explicit propagation`
}

// branchMergeLeak exercises the phi merge. `e` is declared without
// an initialiser, then assigned the sentinel inside one branch.
// The AST walk reports `sentinelForFlow` at its `=` RHS, but the
// return identifier has zero `:=` defines so resolveSubject finds
// nothing. SSA sees the phi merging the sentinel and the fallback
// value and reports the leak at the return.
func branchMergeLeak(c bool) error {
	var e error
	if c {
		e = sentinelForFlow // want `vow\[sentinel-error\]: sentinel error sentinelForFlow leaked: needs observation or explicit propagation`
	} else {
		e = errors.New("ok")
	}
	return e // want `vow\[sentinel-error\]: sentinel error sentinelForFlow leaked: needs observation or explicit propagation`
}

// branchMergeBothLeak exercises a phi where both edges feed
// sentinels. The SSA pass should deduplicate by subject name so a
// single diagnostic is emitted at the return, not two — the names
// map in collectSSASubjects guarantees this.
func branchMergeBothLeak(c bool) error {
	var e error
	if c {
		e = sentinelForFlow // want `vow\[sentinel-error\]: sentinel error sentinelForFlow leaked: needs observation or explicit propagation`
	} else {
		e = sentinelForFlow // want `vow\[sentinel-error\]: sentinel error sentinelForFlow leaked: needs observation or explicit propagation`
	}
	return e // want `vow\[sentinel-error\]: sentinel error sentinelForFlow leaked: needs observation or explicit propagation`
}

// noLeakFromSSA is the negative baseline: every flow into the return
// originates from non-subject calls. SSA should walk both edges of
// the phi, find no *ssa.Global pointing at a tracked subject, and
// stay silent.
func noLeakFromSSA(c bool) error {
	var e error
	if c {
		e = errors.New("left")
	} else {
		e = errors.New("right")
	}
	return e
}

// chainedMultiHopSilent pins the chain-authorization integration: the
// function lists sentinelForFlow in its vow:cond signature, so
// the SSA pass — which would otherwise resolve `y` back to
// `sentinelForFlow` and report — must consult
// functionAuthorizesPropagation and stay silent. Without the gate
// the SSA fallback would diagnose a leak even when the author
// explicitly authorised the propagation, undoing the chain contract
// for any flow shape AST cannot see.
//
// The multi-hop shape is used (rather than reassignment or phi) so
// every subject reference on the way to the return sits on the RHS
// of a short-form define and is suppressed by isAliasBindingRHS. The
// only place a diagnostic could fire is at `return y` through the
// SSA pass, which keeps the fixture's expectation surface a single
// pass/fail bit.
//
// vow:cond * -> sentinelForFlow | nil
func chainedMultiHopSilent() error {
	x := sentinelForFlow
	y := x
	return y
}
