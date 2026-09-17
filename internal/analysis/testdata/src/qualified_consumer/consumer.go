// Package qualified_consumer exercises the cross-package qualified
// identifier matching paths: the syntactic short name written at the
// call site, and the fully-qualified name resolved through
// types.Info. Each function declares a vow:cond sum that names
// qualified_pkg.ErrFoo and returns the corresponding value.
package qualified_consumer

import "qualified_pkg"

// returnsQualifiedOK returns the cross-package sentinel directly.
// The shape's syntactic name (`qualified_pkg.ErrFoo`) matches the
// annotation Term, so the return-annotation checker stays silent.
//
// vow:cond * -> qualified_pkg.ErrFoo | nil
func returnsQualifiedOK() error {
	return qualified_pkg.ErrFoo
}

// returnsQualifiedNil exercises the nil branch of the optional sum.
//
// vow:cond * -> qualified_pkg.ErrFoo | nil
func returnsQualifiedNil() error {
	return nil
}

// returnsWrongQualified returns the same cross-package sentinel but
// the annotation declares a different qualified name. The return
// value does not match any listed member, so the return-annotation
// checker reports the position.
//
// vow:cond * -> qualified_pkg.ErrBar | nil
func returnsWrongQualified() error {
	return qualified_pkg.ErrFoo // want `vow\[sentinel-error\]: return position 0: returning qualified_pkg.ErrFoo but the position only accepts qualified_pkg.ErrBar \| nil`
}
