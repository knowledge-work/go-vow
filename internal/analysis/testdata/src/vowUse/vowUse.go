// Package vowUse is the analysistest fixture for the discharge
// contract carried by the std rule for consumption. The package
// declares one subject and each accompanying file exercises one
// boundary of the (a)(b)(c) discharge semantics.
package vowUse

import "errors"

// ErrFoo is the subject under test.
// vow:define @Sentinel
var ErrFoo = errors.New("foo")
