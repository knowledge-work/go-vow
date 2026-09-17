// Package root is the analysis target of the scope-mode fixture.
// Its nil argument against dep's vow:nil contract is the one
// diagnostic the scoped and whole-closure runs must agree on.
package root

import "vowtest.example/scope/dep"

// Use passes a statically-nil argument to the non-nil slot so the
// caller-side check owes exactly one diagnostic here.
func Use() {
	dep.RequireNonNil(nil)
	_ = dep.Join(nil)
}
