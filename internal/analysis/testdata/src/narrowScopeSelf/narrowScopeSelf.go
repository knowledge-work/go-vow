// Package narrowScopeSelf pins the file-level self-scope branch of the
// narrow filter: diagnostics emitted inside a file listed in
// --changed-files surface unchanged. The same-package call within
// selfCall exercises the keep branch with a real caller-side diagnostic;
// cross-package caller filtering lives in the companion
// narrowScopeXPkgCallee / narrowScopeXPkgOtherCallee / narrowScopeXPkgCaller
// fixtures.
package narrowScopeSelf

// callee declares a non-nil contract on its first parameter so callers
// that pass nil trigger a caller-side diagnostic.
//
// vow:nil (!)
func callee(p *int) {} // want callee:"nilDecl\\(!\\)"

// selfCall sits in the same changed file as callee. The narrow filter
// keeps the resulting diagnostic because the call site is in-scope.
func selfCall() {
	callee(nil) // want `vow\[nil-safety\]: argument 1 \(p\) is nil`
}
