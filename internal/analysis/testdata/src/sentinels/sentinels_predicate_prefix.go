package sentinels

// The fixtures below exercise the prefix-predicate surface: the
// analyzer's read path drives matching through Term.Pred, both for
// a bare `nonzero T` position and for a `nonzero T` member inside a
// pipe-led sum.

// predicatePrefixNonZeroOK returns a non-zero int through the
// `nonzero T` prefix authoring — silent.
//
// vow:cond * -> nonzero int
func predicatePrefixNonZeroOK() int {
	return 42
}

// predicatePrefixNonZeroLeak returns the int zero value at a
// position the prefix predicate declares non-zero — diagnose.
//
// vow:cond * -> nonzero int
func predicatePrefixNonZeroLeak() int {
	return 0 // want `vow\[sentinel-error\]: return position 0: returning the zero value at a position declared nonzero int`
}

// predicatePrefixStringLeak verifies the prefix predicate also
// reaches the string-typed zero (empty string).
//
// vow:cond * -> nonzero string
func predicatePrefixStringLeak() string {
	return "" // want `vow\[sentinel-error\]: return position 0: returning the zero value at a position declared nonzero string`
}

// predicatePrefixSumLeak places the prefix syntax inside a
// pipe-led sum so the matcher walks both the Sum AST and the
// per-member Pred. Returning a non-listed sentinel should report
// the sum-membership violation; the diagnostic prose mirrors the
// canonical predicate spelling.
//
// vow:cond * -> nonzero ErrFoo | nonzero ErrBar
func predicatePrefixSumLeak() error {
	return ErrBaz // want `vow\[sentinel-error\]: return position 0: returning ErrBaz but the position only accepts nonzero ErrFoo \| nonzero ErrBar` `vow\[sentinel-error\]: sentinel error ErrBaz leaked: needs observation or explicit propagation`
}
