// Package suppress is the analysistest fixture for the caller-
// authored vow:suppress marker. Each function exercises one
// suppression position (function-level / line-level trailing / line-
// level leading), one failure mode (bare marker / empty reason /
// reason-with-non-must-consume diagnostic), or the baseline shape
// the marker is meant to silence — read top to bottom and the
// classifier's behavior reads off as a small table.
package suppress

import (
	"errors"
)

// ErrFoo is the subject the must-consume rule tracks. Returning it
// without an observation or vow:cond path leaks.
// vow:define @Sentinel
var ErrFoo = errors.New("foo")

// ErrBar exists so the condition-violation test can return a
// sentinel that is not listed in the vow:cond sum. Both subjects
// being labelled lets us prove that vow:suppress drops the must-
// consume diagnostic on ErrBar while leaving the return-position
// violation untouched.
// vow:define @Sentinel
var ErrBar = errors.New("bar")

// --- baseline: must-consume leaks without any suppress marker ---

func baselineLeak() error {
	return ErrFoo // want `vow\[sentinel-error\]: sentinel error ErrFoo leaked: needs observation or explicit propagation`
}

// --- function-level suppress drops must-consume ---

// funcSuppressed silences the must-consume diagnostic that would
// otherwise fire on the return below. The reason is the value the
// marker requires; analystest passes when no diagnostic surfaces.
//
// vow:suppress: documented in design doc
func funcSuppressed() error {
	return ErrFoo
}

// --- line-level suppress, trailing comment ---

func lineSuppressedTrailing() error {
	return ErrFoo // vow:suppress: documented in design doc
}

// --- line-level suppress, leading comment ---

func lineSuppressedLeading() error {
	// vow:suppress: documented in design doc
	return ErrFoo
}

// --- bare marker on function doc surfaces a parser error ---

// vow:suppress
func bareFuncSuppress() error { // want `vow\[sentinel-error\]: vow:suppress requires a reason: use vow:suppress: <reason>`
	return nil
}

// --- empty-reason marker on function doc surfaces a parser error ---

// vow:suppress:
func emptyReasonFuncSuppress() error { // want `vow\[sentinel-error\]: vow:suppress requires a reason: use vow:suppress: <reason>`
	return nil
}

// --- bare marker as a leading line comment anchors at fn.Pos() ---

func bareLineSuppressLeading() error { // want `vow\[sentinel-error\]: vow:suppress requires a reason: use vow:suppress: <reason>`
	// vow:suppress
	return nil
}

// --- bare marker as a trailing line comment also anchors at fn.Pos() ---

func bareLineSuppressTrailing() error { // want `vow\[sentinel-error\]: vow:suppress requires a reason: use vow:suppress: <reason>`
	return nil // vow:suppress
}

// --- suppress does not drop condition (return-position) diagnostics ---

// suppressDoesNotDropCondition combines a function-level suppress
// with a vow:cond contract. ErrBar is not listed in the sum, so
// the return-position rule fires; suppress drops the must-consume
// leak that would also fire on ErrBar but leaves the condition
// diagnostic untouched. The single want pins the only surviving
// diagnostic.
//
// vow:suppress: documented in design doc
// vow:cond * -> ErrFoo | nil
func suppressDoesNotDropCondition() error {
	return ErrBar // want `vow\[sentinel-error\]: return position 0: returning ErrBar but the position only accepts ErrFoo \| nil`
}
