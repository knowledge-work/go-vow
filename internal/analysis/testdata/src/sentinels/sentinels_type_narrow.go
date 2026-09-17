package sentinels

import (
	"errors"
)

// Fixtures for type-based parameter-requirement narrowing.
//
// Three condition shapes prove `nonnil err` for an interface
// parameter when their truthy side is taken:
//
//   - `errors.As(err, &target)` — a true result means errors.As
//     unwrapped err to a value of the target type, which is only
//     possible if err is non-nil.
//   - `_, ok := err.(T)` — the `ok` half of a comma-ok type
//     assertion is true only when err carries a concrete dynamic
//     type, so the nil-interface case is ruled out.
//   - `switch err.(type) { case T: ... }` — Go lowers the type
//     switch to a chain of comma-ok type assertions, so each
//     non-default case body sits on the truthy side of the same
//     shape the type-assertion rule covers.

type typeNarrowMyError struct{}

func (typeNarrowMyError) Error() string { return "my" }

// typeNarrowErrorsAs carries a non-trivial case that authorises
// ErrFoo when `nonnil err` holds. The `errors.As` call on the
// dominating if establishes the fact on the then side; the
// chain authoriser admits ErrFoo at the inner return.
//
// vow:cond nonnil -> ErrFoo | nil
func typeNarrowErrorsAs(err error) error {
	var target *typeNarrowMyError
	if errors.As(err, &target) {
		return ErrFoo
	}
	return nil
}

// typeNarrowTypeAssertOk uses the comma-ok type assertion form
// as the dominating if. The `ok` half is true only when err is
// non-nil and carries the asserted dynamic type, so the chain
// authoriser admits ErrFoo on the then side.
//
// vow:cond nonnil -> ErrFoo | nil
func typeNarrowTypeAssertOk(err error) error {
	if _, ok := err.(*typeNarrowMyError); ok {
		return ErrFoo
	}
	return nil
}

// typeNarrowTypeSwitchCase exercises the type-switch case form.
// Go SSA lowers the case clause to the same comma-ok type
// assertion shape, so the same matcher rule narrows the case
// body to `nonnil err` and the chain authoriser admits ErrFoo.
//
// vow:cond nonnil -> ErrFoo | nil
func typeNarrowTypeSwitchCase(err error) error {
	switch err.(type) {
	case *typeNarrowMyError:
		return ErrFoo
	}
	return nil
}

// typeNarrowOutsideErrorsAsBranchLeaks shows the lenient
// boundary: on the else side of an `errors.As` guard, the
// fact is not proved (errors.As returns false for nil err
// AND for type-mismatched non-nil err, so the else side
// carries no narrowing). The chain authoriser refuses ErrFoo
// and the leak detector fires.
//
// vow:cond nonnil -> ErrFoo | nil
func typeNarrowOutsideErrorsAsBranchLeaks(err error) error {
	var target *typeNarrowMyError
	if errors.As(err, &target) {
		return nil
	}
	return ErrFoo // want `vow\[sentinel-error\]: sentinel error ErrFoo leaked: needs observation or explicit propagation`
}
