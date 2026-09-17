// Package caller exercises the caller-side pass of the fact
// propagation test. Each call whose callee carries the marker fact
// contributes a diagnostic.
package caller

import "vowdrivertest.example/xpkg/callee"

// Use calls the marked and plain callees so the test analyzer's
// caller-side sweep reports one diagnostic (for the marked call).
func Use() {
	callee.Marked()
	callee.Plain()
	_ = callee.Var
	_ = callee.Const
	var m callee.Marker
	m.Field = 0
	_ = m
}
