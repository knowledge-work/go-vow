// Package testing shares both its name and a handle's type name with
// the standard library's testing package, so the completeness fixture
// can pin that neither name makes a handle without the import path
// `testing`.
package testing

type T struct{}
