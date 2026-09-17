package sentinels

// Fixtures for the `*` complete-wildcard term in return
// requirements and parameter requirements.
//
// A `*` member in a direct sum short-circuits the per-shape
// matcher so authors can write `{ ErrFoo | * }` for "list the
// interesting subjects but accept every other value" boundaries.
// A `*` Single position accepts every shape. A `*` parameter-
// requirement position is the trivial constraint that holds at
// every call site — the same semantics as a nil Prereq, but
// spelled explicitly in the surface form.
//
// Chain authorisation follows the same admission set: a `*`
// wildcard authorises propagation of every tracked subject so
// authors do not have to enumerate the open set twice (once for
// the sum-membership matcher and once for the leak detector).
// The `_` label-aware fall-through has different propagation
// semantics covered by sentinels_placeholder.go.

// wildcardSumAuthorisesEverySubject returns three different
// shapes at a position declared as `{ ErrFoo | * }`. The `*`
// member admits every shape AND authorises every subject, so
// returning ErrBar (not listed by name) raises no leak even
// though it is a tracked sentinel. The nil literal is admitted
// the same way.
//
// This is the `*` half of the `_` vs `*` invariant: the same
// ErrBar return at `{ ErrFoo | _ }` DOES leak — see
// placeholderRejectsLabelledSubject in sentinels_placeholder.go.
// The two fixtures pin the asymmetry as a direct contrast.
//
// vow:cond * -> ErrFoo | *
func wildcardSumAuthorisesEverySubject(which int) error {
	switch which {
	case 0:
		return ErrFoo
	case 1:
		return ErrBar
	}
	return nil
}

// wildcardSingleAcceptsAny returns a tracked sentinel at a Single
// position declared as `*`. The single-Term path short-circuits
// the zero-check (a wildcard has no per-value constraint) and the
// matcher admits every shape; chain authorisation likewise
// follows the wildcard so the leak detector stays silent. The
// fixture pins the joint "accepts every shape AND authorises
// every subject" invariant of the bare `*` Single position.
//
// vow:cond * -> *
func wildcardSingleAcceptsAny(which int) error {
	if which == 0 {
		return ErrFoo
	}
	return nil
}

// wildcardPrereqTrivialEnforces puts a `*` wildcard at the
// prereq position. The wildcard is the trivial constraint — it
// holds at every call site — so the return requirement is
// enforced at every return. The sum `{ 1 | 2 | 3 }` rejects 4,
// and the diagnostic fires regardless of which branch the return
// sits in, identically to a nil-Prereq (the unparameterised vow:cond)
// form.
//
// vow:cond * -> 1 | 2 | 3
func wildcardPrereqTrivialEnforces(x int) int {
	if x == 0 {
		return 4 // want `vow\[sentinel-error\]: return position 0: returning literal 4 but the position only accepts 1 \| 2 \| 3`
	}
	return 4 // want `vow\[sentinel-error\]: return position 0: returning literal 4 but the position only accepts 1 \| 2 \| 3`
}
