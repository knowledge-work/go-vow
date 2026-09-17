// Package narrowScopeSamePkg pins the narrow-scope behaviour on call
// sites that sit in an unchanged file of a changed file's own package.
// The test publishes lib.go as the changed file and this package as a
// changed package; caller.go stays unchanged and holds the call sites.
package narrowScopeSamePkg

// Changed is the contract source. The test publishes this file through
// ChangedFileSet.Files, so a call site targeting Changed is in scope
// regardless of which file in the package hosts it.
//
// vow:nil (!)
func Changed(p *int) {} // want Changed:"nilDecl\\(!\\)"
