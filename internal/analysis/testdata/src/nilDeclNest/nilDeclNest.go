// Package nilDeclNest pins the nest grammar that vow:nil now
// accepts: a slice form (`<top>[]<el>`), a map form
// (`<top>[<key>]<val>`), and recursive compositions of the two.
// The caller-side argument check fires on the top-level nullness
// the same way as the no-nest case; element- and key-level flow
// tracking is not provided through nest paths.
package nilDeclNest

// Key is the address-of demo type used by map fixtures.
type Key struct{}

// Value is the address-of demo type used by map and slice
// fixtures.
type Value struct{}

// sliceCallee pins a slice nest. The top-level slice is non-nil
// (`!`); the element layer is nillable (`?`). Caller-side check
// fires on the top-level slot.
//
// vow:nil (![]?)
func sliceCallee(items []*Value) {} // want sliceCallee:"nilDecl\\(!\\)"

// mapCallee pins a map nest. The top-level map is non-nil; the
// key layer is non-nil; the value layer is nillable.
//
// vow:nil (![!]?)
func mapCallee(idx map[*Key]*Value) {} // want mapCallee:"nilDecl\\(!\\)"

// recursiveCallee pins a recursive nest: a non-nil slice whose
// element is a map at platform-state top (no `!` / `?` between
// `[]` and `[!]`) with a non-nil key and a nillable value. The
// element-layer map top is therefore unconstrained at the call
// site; only the outer slice top is checked.
//
// vow:nil (![][!]?)
func recursiveCallee(data []map[*Key]*Value) {} // want recursiveCallee:"nilDecl\\(!\\)"

// nillableTopCallee pins a nillable top-level slice. Caller-side
// stays silent because the contract admits a nil slice.
//
// vow:nil (?[]?)
func nillableTopCallee(items []*Value) {} // want nillableTopCallee:"nilDecl\\(\\?\\)"

// okCalls supplies non-nil top-level containers and silent
// nillable shapes. None of these reach a diagnostic.
func okCalls() {
	items := []*Value{}
	idx := map[*Key]*Value{}
	sliceCallee(items)
	mapCallee(idx)
	nillableTopCallee(nil)
}

// nilTopViolations exercises the top-level non-nil check for
// every recognised nest top. The element / key / value layer
// diagnostics arrive together with the SSA forward walk in a
// here.
func nilTopViolations() {
	sliceCallee(nil)     // want `vow\[nil-safety\]: argument 1 \(items\) is nil`
	mapCallee(nil)       // want `vow\[nil-safety\]: argument 1 \(idx\) is nil`
	recursiveCallee(nil) // want `vow\[nil-safety\]: argument 1 \(data\) is nil`
}
