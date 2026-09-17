package sentinels

// --- nil at nonzero error ---

// nilErrorBangLeak declares nonzero error at the single position but
// returns the bare nil identifier. The analyzer recognises nil as
// the zero value of an interface type and reports the violation.
//
// vow:cond * -> nonzero error
func nilErrorBangLeak() error {
	return nil // want `vow\[sentinel-error\]: return position 0: returning the zero value at a position declared nonzero error`
}

// nilErrorOptionalOK is the corresponding silent baseline: a nil
// return is permitted by the `?` qualifier.
//
// vow:cond * -> error?
func nilErrorOptionalOK() error {
	return nil
}

// --- float zero policy ---

// floatZeroLeak returns 0.0 at a nonzero float64 position.
//
// vow:cond * -> nonzero float64
func floatZeroLeak() float64 {
	return 0.0 // want `vow\[sentinel-error\]: return position 0: returning the zero value at a position declared nonzero float64`
}

// floatNonZeroOK is silent: 1.5 is non-zero.
//
// vow:cond * -> nonzero float64
func floatNonZeroOK() float64 {
	return 1.5
}

// floatNegativeZeroLeak: -0.0 is equal to +0.0 under IEEE 754, so the
// zero policy treats it as a zero too.
//
// vow:cond * -> nonzero float64
func floatNegativeZeroLeak() float64 {
	return -0.0 // want `vow\[sentinel-error\]: return position 0: returning the zero value at a position declared nonzero float64`
}

// --- complex zero policy ---

// complexZeroLeak returns 0i (== 0+0i) at a nonzero complex128 position.
//
// vow:cond * -> nonzero complex128
func complexZeroLeak() complex128 {
	return 0i // want `vow\[sentinel-error\]: return position 0: returning the zero value at a position declared nonzero complex128`
}

// complexNonZeroOK has a non-zero real component, so the position is
// satisfied.
//
// vow:cond * -> nonzero complex128
func complexNonZeroOK() complex128 {
	return 1 + 0i
}
