// Package narrowScopeXPkgCaller exercises the per-call-site narrow
// filter against two cross-package callees. The harness pins
// narrowScopeXPkgCallee as a changed file and this package as a caller
// candidate; the narrow filter keeps the diagnostic at the call that
// targets the changed callee and drops the diagnostic at the call that
// targets the unchanged callee.
package narrowScopeXPkgCaller

import (
	"narrowScopeXPkgCallee"
	"narrowScopeXPkgOtherCallee"
)

// callsChangedCallee passes nil into the changed callee's non-nil slots.
// The per-call-site narrow filter keeps both diagnostics because the
// callee sits in a changed file.
func callsChangedCallee() {
	narrowScopeXPkgCallee.RequireNonNil(nil, nil) // want `vow\[nil-safety\]: argument 1 \(p\) is nil` `vow\[nil-safety\]: argument 2 \(q\) is nil`
}

// callsUnchangedCallee passes nil into the unchanged callee's non-nil
// slots. The narrow filter drops the resulting diagnostics because the
// callee does not sit in a changed file.
func callsUnchangedCallee() {
	narrowScopeXPkgOtherCallee.RequireNonNil(nil, nil)
}
