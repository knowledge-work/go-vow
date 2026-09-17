// Package nilDeclSelfNest pins the nest-layer flow checks that
// land on call-argument and return-value sites. A slice index or
// map lookup whose container's nest body declares the element
// layer as `?` lifts the element load into a Nillable proof; the
// resulting value rides into a `!` slot and surfaces a diagnostic.
package nilDeclSelfNest

// Bag carries two nest-decorated fields so the slice-element and
// map-value paths can be exercised independently.
type Bag struct {
	// vow:nil ![]?
	Items []*int // want Items:"fieldNil\\(!\\)"

	// vow:nil ![!]?
	Index map[*string]*int // want Index:"fieldNil\\(!\\)"

	// vow:nil ![]!
	NonNilItems []*int // want NonNilItems:"fieldNil\\(!\\)"
}

// consume pins both regular parameters as non-nil; the flow walk
// must surface a diagnostic on any nest-element value the call
// site rides into either position.
//
// vow:nil (!)
func consume(x *int) {} // want consume:"nilDecl\\(!\\)"

// sliceElementFlow pins the slice-element nest path: the element
// layer is declared `?`, so a direct index expression at the
// call argument surfaces a diagnostic.
func sliceElementFlow(b Bag, i int) {
	consume(b.Items[i]) // want `vow\[nil-safety\]: argument 1 \(x\) may be nil through flow; vow:nil declared ! at this position`
}

// mapValueFlow pins the map-value nest path: the value layer is
// declared `?` (with `!` at the key), so a direct lookup at the
// call argument surfaces a diagnostic on the value position.
func mapValueFlow(b Bag, k *string) {
	consume(b.Index[k]) // want `vow\[nil-safety\]: argument 1 \(x\) may be nil through flow; vow:nil declared ! at this position`
}

// silentNonNilElement pins the silent path: the element layer is
// declared `!`, so the nest grammar proves non-nil at the load and
// the caller-side check stays silent.
func silentNonNilElement(b Bag, i int) {
	consume(b.NonNilItems[i])
}
