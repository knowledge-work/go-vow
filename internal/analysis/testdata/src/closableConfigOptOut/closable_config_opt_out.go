// Package closableConfigOptOut is the analysistest fixture for the
// per-directory `vow.yaml` config carrying
// `closable.built_in_closables: []`. The fixture lives next to its
// own vow.yaml so the discovery walk resolves the file when the
// analyzer reaches this package. With the embedded built-in list
// replaced by the empty list for this scope, the fixture pins three
// invariants:
//
//   - a stdlib pointer acquisition (`*os.File`) leaks silently
//     because the analyzer skips the built-in pointer list.
//   - an upcast return whose only Closable evidence is the
//     `io.Closer` constraint on a bare type parameter goes silent,
//     because the constraint walker no longer credits the parameter
//     as Closable.
//   - a user-declared `vow:define @Closable` type still reports its
//     leak, confirming that the opt-out narrows the default set
//     without disabling Closable semantics entirely.
package closableConfigOptOut

import (
	"io"
	"os"
)

// vow:define @Closable
type Conn struct{}

func (c *Conn) Close() error { return nil }

func openConn() *Conn { return &Conn{} }

// silentBuiltinPointerLeak opens an `*os.File` and never closes it.
// `*os.File` is a built-in closable type; the config drops the list
// for this directory and the analyzer reports nothing.
func silentBuiltinPointerLeak() {
	f, _ := os.Open("noop")
	_ = f
}

// silentBuiltinConstraintUpcast returns a bare type-parameter value
// whose constraint embeds `io.Closer`. The leak site keeps `T` as a
// `*types.TypeParam` so the constraint walker is the only path that
// can credit the value as Closable. With the built-in list cleared
// the constraint walker stays silent.
func silentBuiltinConstraintUpcast[T io.Closer](v T) any {
	return v
}

// diagnoseUserDeclaredLeak acquires a user-declared @Closable via a
// factory return. The config must not suppress diagnostics for
// types the author opted in to via vow:define.
func diagnoseUserDeclaredLeak() {
	c := openConn() // want `vow\[closable\]: c is acquired but not closed; add a defer Close call or declare the transfer with vow:emit`
	_ = c
}
