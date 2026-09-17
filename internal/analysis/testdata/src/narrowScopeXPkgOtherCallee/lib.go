// Package narrowScopeXPkgOtherCallee provides an unchanged-file callee
// for the narrow filter fixture. The companion narrowScopeXPkgCaller
// invokes RequireNonNil with nil arguments, but because this file does
// not appear in --changed-files the narrow filter drops the resulting
// caller-side diagnostic.
package narrowScopeXPkgOtherCallee

// RequireNonNil pins both parameters as non-nil; cross-package callers
// passing nil literals would otherwise produce caller-side diagnostics.
//
// vow:nil (!,!)
func RequireNonNil(p, q *int) {} // want RequireNonNil:"nilDecl\\(!,!\\)"
