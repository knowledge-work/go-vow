package sentinels

import "errors"

// Fixtures for the `_` placeholder's label-aware fall-through.
//
// The placeholder semantics distinguishes labelled subjects (vars
// the package marks with `vow:define @Sentinel`) from every other
// value. A `_` member admits literals, nil, and non-subject
// identifiers; a labelled subject must be listed by name. The
// complete wildcard `*` admits both groups; the placeholder `_`
// carves out the labelled subset.

// ErrLabel is a second labelled subject the placeholder fixtures
// pair against ErrFoo. Carrying the same marker as the other
// sentinels in this package keeps the obligation machinery
// uniform: the analyzer reaches ErrLabel through the same concept
// discovery path it reaches every other tracked sentinel.
//
// vow:define @Sentinel
var ErrLabel = errors.New("label")

// placeholderAdmitsNonSubject returns a non-subject error
// (locally constructed via errors.New, NOT bound to one of the
// package's labelled sentinels) at a position declared as
// `{ ErrFoo | _ }`. The `_` placeholder admits the non-subject
// identifier so the sum-membership matcher stays silent. ErrFoo
// is not listed in the return paths so the explicit-member match
// does not fire either.
func placeholderAdmitsNonSubject() error {
	local := errors.New("non-subject")
	return placeholderSumGate(local)
}

// placeholderSumGate is the function that carries the actual
// vow:cond * -> annotation. Splitting the caller from the gate
// keeps the resolveExprAlias machinery from short-circuiting the
// match on a direct subject reference.
//
// vow:cond * -> ErrFoo | _
func placeholderSumGate(err error) error {
	return err
}

// placeholderRejectsLabelledSubject returns ErrBar — a labelled
// subject the package declares — at a position declared as
// `{ ErrFoo | _ }`. The placeholder is label-aware: it admits
// non-subjects but excludes labelled subjects, so ErrBar (a
// tracked sentinel) must appear explicitly in the sum to avoid
// the diagnostic. The leak diagnostic fires alongside the sum-
// membership violation because the chain-auth path likewise
// refuses the placeholder for labelled subjects.
//
// This is the `_` half of the `_` vs `*` invariant: the identical
// ErrBar return at `{ ErrFoo | * }` raises NO leak — see
// wildcardSumAuthorisesEverySubject in sentinels_wildcard.go. The
// pair pins the asymmetry (distinct admission sets, distinct
// chain-authorisation) as a direct contrast.
//
// vow:cond * -> ErrFoo | _
func placeholderRejectsLabelledSubject() error {
	return ErrBar // want `vow\[sentinel-error\]: return position 0: returning ErrBar but the position only accepts ErrFoo \| _` `vow\[sentinel-error\]: sentinel error ErrBar leaked: needs observation or explicit propagation`
}

// placeholderAdmitsNil returns the nil literal at a position
// declared as `{ ErrFoo | _ }`. The placeholder admits the nil
// shape via the label-aware fall-through (nil is not a labelled
// subject), so the sum-membership matcher stays silent without
// the author having to list `nil` explicitly.
//
// vow:cond * -> ErrFoo | _
func placeholderAdmitsNil() error {
	return nil
}

// secondLabelChainAuthorised returns ErrLabel from a function
// whose chain-auth signature names it explicitly. Pairing this
// fixture with the placeholder-admits-* cases above keeps the
// label-aware fall-through covered against a second labelled
// subject — chain auth, the gate on the propagation surface, must
// still fire when the explicit member is named.
//
// vow:cond * -> ErrLabel | nil
func secondLabelChainAuthorised(which int) error {
	if which == 0 {
		return ErrLabel
	}
	return nil
}
