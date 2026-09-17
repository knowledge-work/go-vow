// Package narrowScopeXPkgCallee provides the changed-file callee for the
// cross-package narrow filter fixture. The companion narrowScopeXPkgCaller
// invokes RequireNonNil with nil arguments so the caller-side diagnostic
// triggers; the narrow filter keeps the diagnostic when this file appears
// in --changed-files.
package narrowScopeXPkgCallee

// RequireNonNil pins both parameters as non-nil so cross-package callers
// that pass nil literals trigger caller-side diagnostics.
//
// vow:nil (!,!)
func RequireNonNil(p, q *int) {} // want RequireNonNil:"nilDecl\\(!,!\\)"
