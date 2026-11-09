package qualified_consumer

import (
	"errors"

	"qualified_pkg"
)

// emitsQualified declares the cross-package sentinel under the
// qualified name its return spells, so the vow:emit self-validation
// finds the emission.
//
// vow:emit qualified_pkg.ErrFoo
func emitsQualified() error {
	return qualified_pkg.ErrFoo
}

// emitsQualifiedNever declares the qualified name but returns only
// nil, so the self-validation reports the empty declaration.
//
// vow:emit qualified_pkg.ErrFoo
func emitsQualifiedNever() error { // want `vow\[sentinel-error\]: vow:emit qualified_pkg.ErrFoo declaration is not self-validated: function body never returns qualified_pkg.ErrFoo`
	return nil
}

// usesQualifiedByIs declares the qualified name and discharges it with
// errors.Is, so the vow:use self-validation finds the discharge.
//
// vow:use qualified_pkg.ErrFoo
func usesQualifiedByIs(err error) bool {
	if errors.Is(err, qualified_pkg.ErrFoo) {
		return true
	}
	return false
}

// usesQualifiedByCompare declares the qualified name and discharges it
// by comparison in an if condition.
//
// vow:use qualified_pkg.ErrFoo
func usesQualifiedByCompare(err error) bool {
	if err == qualified_pkg.ErrFoo {
		return true
	}
	return false
}

// usesQualifiedNever declares the qualified name but never observes it,
// so the self-validation reports the declaration.
//
// vow:use qualified_pkg.ErrFoo
func usesQualifiedNever(err error) bool { // want `vow\[sentinel-error\]: vow:use qualified_pkg.ErrFoo declaration is not self-validated: function body has no discharge site for qualified_pkg.ErrFoo`
	return err != nil
}

// holder carries an error in a field whose name matches the sentinel's.
type holder struct{ ErrFoo error }

// emitsFieldSelector declares `h.ErrFoo`, a field selector rather than
// a qualified identifier. Only a qualified identifier is matched by its
// `pkg.Name` text, so returning `h.ErrFoo` does not self-validate the
// declaration.
//
// vow:emit h.ErrFoo
func emitsFieldSelector(h holder) error { // want `vow\[sentinel-error\]: vow:emit h.ErrFoo declaration is not self-validated: function body never returns h.ErrFoo`
	return h.ErrFoo
}

// emitsBareName declares the bare name while the body returns the
// qualified identifier, so the declaration names nothing the body
// returns and the self-validation reports it.
//
// vow:emit ErrFoo
func emitsBareName() error { // want `vow\[sentinel-error\]: vow:emit ErrFoo declaration is not self-validated: function body never returns ErrFoo`
	return qualified_pkg.ErrFoo
}

// usesBareName declares the bare name while the body observes the
// qualified identifier, so the self-validation reports it.
//
// vow:use ErrFoo
func usesBareName(err error) bool { // want `vow\[sentinel-error\]: vow:use ErrFoo declaration is not self-validated: function body has no discharge site for ErrFoo`
	if errors.Is(err, qualified_pkg.ErrFoo) {
		return true
	}
	return false
}
