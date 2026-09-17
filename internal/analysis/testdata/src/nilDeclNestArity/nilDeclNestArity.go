// Package nilDeclNestArity pins the per-position arity check that
// the analyzer applies to `vow:nil` nest bodies. The walker reads
// the Go container at each signature position and reports when the
// InnerDecls list does not match the container's expected shape:
// slices admit zero or one inner decl, maps require exactly one,
// and generic instantiations require one entry per type argument.
// Positions whose Go type the resolver cannot classify stay silent.
package nilDeclNestArity

// Key is the demo type for map fixtures.
type Key struct{}

// Value is the demo element type for slice / map / generic fixtures.
type Value struct{}

// Box is the demo single-type-parameter generic container.
type Box[T any] struct{ V T }

// Pair is the demo two-type-parameter generic container.
type Pair[A, B any] struct {
	A A
	B B
}

// sliceTooManyInner declares a slice nest with two inner decls,
// which exceeds the slice grammar's `[]` (0) and `[el]` (1) shapes.
//
// vow:nil ([!,?])
func sliceTooManyInner(items []*Value) {} // want sliceTooManyInner:"nilDecl\\(\\)" `vow\[nil-safety\]: vow:nil param position 0 slice nest carries 2 inner decls but the container expects zero or one entry`

// mapMissingKey declares a map nest with zero inner decls; the map
// grammar requires the key qualifier slot, so this is rejected.
//
// vow:nil ([])
func mapMissingKey(idx map[*Key]*Value) {} // want mapMissingKey:"nilDecl\\(\\)" `vow\[nil-safety\]: vow:nil param position 0 map nest carries 0 inner decls but the container expects 1 entry`

// genericArityMismatchOne declares a single-type-parameter generic
// with two inner decls. The grammar requires one decl per type
// argument, so the count must match `Box[T]`'s arity.
//
// vow:nil ([!,?])
func genericArityMismatchOne(box *Box[*Value]) {} // want genericArityMismatchOne:"nilDecl\\(\\)" `vow\[nil-safety\]: vow:nil param position 0 generic nest carries 2 inner decls but the container expects 1 entry`

// genericArityMismatchTwo declares a two-type-parameter generic
// with one inner decl. The grammar still requires one entry per
// type argument.
//
// vow:nil ([!])
func genericArityMismatchTwo(pair *Pair[*Key, *Value]) {} // want genericArityMismatchTwo:"nilDecl\\(\\)" `vow\[nil-safety\]: vow:nil param position 0 generic nest carries 1 inner decl but the container expects 2 entries`

// sliceWellFormed pins the silent path: a single inner decl on a
// slice is valid.
//
// vow:nil ([?])
func sliceWellFormed(items []*Value) {} // want sliceWellFormed:"nilDecl\\(\\)"

// mapWellFormed pins the silent path: a single inner decl is valid
// for a map.
//
// vow:nil ([!])
func mapWellFormed(idx map[*Key]*Value) {} // want mapWellFormed:"nilDecl\\(\\)"

// genericWellFormed pins the silent path: the inner decls count
// matches the type-argument arity.
//
// vow:nil ([!,?])
func genericWellFormed(pair *Pair[*Key, *Value]) {} // want genericWellFormed:"nilDecl\\(\\)"
