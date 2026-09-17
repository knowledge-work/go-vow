package sentinels

import "errors"

// ErrFoo, ErrBar, ErrBaz are additional subjects used to exercise
// vow:cond * -> direct-sum positions. They are declared in this file
// (rather than sentinels.go) so the vow:cond scenarios stand on
// their own and read as a coherent table.

// vow:define @Sentinel
var ErrFoo = errors.New("foo")

// vow:define @Sentinel
var ErrBar = errors.New("bar")

// vow:define @Sentinel
var ErrBaz = errors.New("baz")

// chainBySumOK forwards ErrFoo or ErrBar; both are listed in the
// direct sum, so the must-consume rule treats them as chain. Returning
// nil is also permitted because the sum is Optional.
//
// vow:cond * -> _, (ErrFoo | ErrBar)?
func chainBySumOK(which int) (int, error) {
	switch which {
	case 0:
		return 0, ErrFoo
	case 1:
		return 0, ErrBar
	}
	return 0, nil
}

// chainBySumLeak returns ErrBaz, which is not listed in the declared
// sum. Two diagnostics are expected at the same identifier:
//
//   - the vow:cond checker reports the position-level violation.
//   - the must-consume rule reports a leak because the authorization
//     does not cover ErrBaz, so it falls back to the leak class.
//
// vow:cond * -> _, (ErrFoo | ErrBar)?
func chainBySumLeak() (int, error) {
	return 0, ErrBaz // want `vow\[sentinel-error\]: return position 1: returning ErrBaz but the position only accepts ErrFoo \| ErrBar` `vow\[sentinel-error\]: sentinel error ErrBaz leaked: needs observation or explicit propagation`
}

// chainBySumNilLeak declares a non-Optional sum but returns nil. The
// annotation checker reports the nil at the position; the must-consume
// rule is silent because nil is not a subject reference.
//
// vow:cond * -> _, ErrFoo
func chainBySumNilLeak() (int, error) {
	return 0, nil // want `vow\[sentinel-error\]: return position 1: returning nil but the position is not optional`
}
