// Package vowEmit is the analysistest fixture for the vow:emit marker and
// the vow:emit shorthand. Each function exercises one aspect of the
// declaration: caller-side chain authorisation, callee self-
// validation, the comma-list shorthand, and interaction with the
// pre-existing chain-auth surfaces.
package vowEmit

import (
	"errors"
)

// ErrFoo and ErrBar are the subjects the vow:emit declarations name.
// Two subjects let the subject-specificity fixtures prove that an
// `vow:emit ErrFoo` declaration does not silently authorise ErrBar.
//
// vow:define @Sentinel
var ErrFoo = errors.New("foo")

// vow:define @Sentinel
var ErrBar = errors.New("bar")
