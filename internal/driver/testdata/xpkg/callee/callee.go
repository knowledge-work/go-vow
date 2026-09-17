// Package callee holds the callee-side declarations the driver
// tests use to exercise fact propagation and the visibility filter.
package callee

// Marked is an exported package-level function; the test analyzer
// exports a fact against it so the caller-side pass can detect it
// through the fact table.
func Marked() {}

// Plain is exported but the test analyzer does not attach a fact
// to it, letting the caller-side pass verify a negative case.
func Plain() {}

// helper is unexported. The test analyzer attaches a fact to it
// so the visibility filter has an entry to drop across the import
// boundary.
func helper() {}

// Marker is a type-name. Type names inherit unconditionally, so a
// fact attached here should survive the filter.
type Marker struct {
	// Field is a struct field. Fields inherit unconditionally.
	Field int
}

// Const is a constant. Constants inherit unconditionally.
const Const = 42

// Var is an exported package-level variable. It inherits because
// its declaring package is the same as dep.
var Var = 7

// _ references helper so the compiler does not drop it. The test
// only cares about helper's fact entry, not its call graph.
var _ = helper
