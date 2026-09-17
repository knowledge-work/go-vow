// Package discharged is the analysistest fixture for the
// all-verification barrier introduced by the discharged marker. The
// package declares one subject and a handful of barrier callees;
// each accompanying file exercises one boundary of the semantics.
package discharged

import "errors"

// ErrFoo is the subject under test.
// vow:define @Sentinel
var ErrFoo = errors.New("foo")
