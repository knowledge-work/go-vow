// Package payload holds the type a tuple sum in another package
// names. Its name differs from its import path on purpose: a
// fixture where the two coincide cannot tell the path-rendering
// axes from the name-rendering one.
package payload

// Foo is the payload the importing package's constructors return.
type Foo struct{ V int }

// New builds a Foo the way a constructor in the importing package
// reaches for one.
func New() *Foo { return &Foo{V: 1} }
