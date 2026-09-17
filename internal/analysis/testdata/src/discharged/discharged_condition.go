package discharged

import "errors"

// dischargedTwoArg returns a value pair through the barrier.
// vow:discharged
func dischargedTwoArg() (int, error) {
	return 0, errors.New("opaque")
}

// conditionCallerOpaque declares a per-position condition that the
// caller would normally have to satisfy at every return site. The
// return statement forwards the discharged tuple verbatim; the
// per-position validator opaque-passes each discharged call so the
// strict vow:cond is not blamed for the unanalyzable value.
//
// The condition shape only has effect when both results sit at
// distinct positions, so the test return constructs them explicitly
// rather than spreading a multi-value call.
// vow:cond * -> _, nonnil
func conditionCallerOpaque() (int, error) {
	n, e := dischargedTwoArg()
	return n, e
}
