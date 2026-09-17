// Package useHint is the analysistest fixture for the caller-
// authored vow:use discharge assertion. Each file exercises one
// dimension (line vs. function scope, subject specificity,
// interaction with discharged / suppress, malformed marker). Together
// they read off as a small table of the marker's behaviour.
package useHint

import (
	"errors"
)

// ErrFoo and ErrBar are the subjects the must-consume rule tracks.
// Two distinct subjects are needed so the subject-specificity
// fixtures can prove a vow:use ErrFoo line leaves an ErrBar leak
// untouched.
//
// vow:define @Sentinel
var ErrFoo = errors.New("foo")

// vow:define @Sentinel
var ErrBar = errors.New("bar")

// baselineLeak pins the must-consume diagnostic the vow:use
// fixtures below silence. Without the marker, the leak fires.
func baselineLeak() error {
	return ErrFoo // want `vow\[sentinel-error\]: sentinel error ErrFoo leaked: needs observation or explicit propagation`
}

// lineUseTrailing asserts discharge for ErrFoo with the vow:use
// marker placed as a trailing comment on the leak's own line. The
// must-consume diagnostic that baselineLeak surfaces is dropped.
func lineUseTrailing() error {
	return ErrFoo // vow:use ErrFoo
}

// lineUseLeading asserts discharge for ErrFoo with the vow:use
// marker placed on the line immediately above the leak. The
// trailing-vs-leading resolution mirrors vow:suppress: a comment
// whose own line carries no statement claims the next statement's
// line, so the diagnostic on the return below is dropped.
func lineUseLeading() error {
	// vow:use ErrFoo
	return ErrFoo
}
