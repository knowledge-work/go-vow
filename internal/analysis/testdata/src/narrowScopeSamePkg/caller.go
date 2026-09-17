package narrowScopeSamePkg

// Unchanged is declared in this file, which the test leaves out of
// ChangedFileSet.Files. A call site targeting it stays out of scope.
//
// vow:nil (!)
func Unchanged(p *int) {} // want Unchanged:"nilDecl\\(!\\)"

// callChanged sits in an unchanged file and targets a callee declared in
// the changed file, so the per-call-site rule keeps the diagnostic. Before
// the ChangedPackages fix this dropped: the package owns the changed file
// and therefore never appeared in CallerPackages.
func callChanged() {
	Changed(nil) // want `vow\[nil-safety\]: argument 1 \(p\) is nil; callee declared non-nil at this position via vow:nil`
}

// callUnchanged targets a callee declared in this same unchanged file, so
// the call site has no connection to what changed and stays silent.
func callUnchanged() {
	Unchanged(nil)
}
