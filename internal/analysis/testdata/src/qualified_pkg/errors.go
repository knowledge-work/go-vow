// Package qualified_pkg exposes a sentinel error consumed by
// cross-package fixtures so the analyzer's qualified-name matching
// (both the syntactic short form and the types.Info-resolved fully
// qualified form) can be exercised end-to-end.
package qualified_pkg

import "errors"

// ErrFoo is the cross-package sentinel under test. The vow:define @Sentinel
// marker is intentionally absent in this file — the cross-package
// fixture infrastructure focuses on `vow:cond` references, and
// the cross-package subjects path waits until inter-procedural work
// gives findSubjects a multi-package view.
var ErrFoo = errors.New("foo")
