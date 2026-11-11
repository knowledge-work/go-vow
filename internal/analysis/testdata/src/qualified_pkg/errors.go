// Package qualified_pkg exposes a sentinel error consumed by
// cross-package fixtures so the analyzer's qualified-name matching
// (both the syntactic short form and the types.Info-resolved fully
// qualified form) can be exercised end-to-end.
package qualified_pkg

import "errors"

// ErrFoo is the cross-package sentinel under test. It carries no
// subject marker, so the consumer's returns exercise the qualified-name
// matching alone; a marked sentinel would add the must-consume check
// to each of them, which sentinelsXPkgCaller covers.
var ErrFoo = errors.New("foo")
