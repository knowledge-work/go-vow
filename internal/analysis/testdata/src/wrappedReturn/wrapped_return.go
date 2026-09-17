// Package wrappedReturn is the analysistest fixture for the
// fifth vow:emit destination — the wrapped-return hand-off. The
// sample passthrough-emit preset (see example/werror.yaml)
// registers `example.com/wraplib.Wrap` and
// `example.com/wraplib.Errorf` so a statement-scope vow:emit
// marker anchored to a return whose value flows through one of
// the registered calls reaches the recogniser as
// emitDestinationWrappedReturn instead of being reported as an
// uninferred destination. The test entry point loads the sample
// preset alongside the built-in set so the recogniser carries
// the wrap signatures at analysis time.
package wrappedReturn

import (
	"errors"

	"example.com/wraplib"
)

// ErrFoo is the sentinel the wrapped-return fixture hands off
// through a registered wrap call.
//
// vow:define @Sentinel
var ErrFoo = errors.New("foo")

// wrappedReturnSilent anchors a statement-scope vow:emit marker
// to a return that calls wraplib.Wrap. The wrap call matches the
// sample preset's registered pattern, so the recogniser resolves
// the destination as wrapped-return and no destination-inference
// diagnostic fires. The function-doc vow:cond declaration
// authorises ErrFoo for chain propagation so the sentinel
// must-consume pass stays silent.
//
// vow:cond * -> ErrFoo | nil
func wrappedReturnSilent() error {
	// vow:emit ErrFoo
	return wraplib.Wrap(ErrFoo, "context")
}

// wrappedReturnErrorfSilent anchors the marker on a return that
// calls wraplib.Errorf. The preset registers both Wrap and
// Errorf, so the recogniser matches either signature.
//
// vow:cond * -> ErrFoo | nil
func wrappedReturnErrorfSilent() error {
	// vow:emit ErrFoo
	return wraplib.Errorf(ErrFoo, "context %d", 42)
}

// unwrappedReturnDiagnose anchors the marker on a return that
// does not pass through a registered wrap function. The plain
// return statement does not match the wrapped-return pattern;
// the destination falls through to unknown and the discovery
// walker reports the missing-destination diagnostic. The
// function-doc vow:cond keeps the sentinel must-consume pass
// silent so the destination diagnostic reads in isolation.
//
// vow:cond * -> ErrFoo | nil
func unwrappedReturnDiagnose() error {
	x := ErrFoo
	// vow:emit ErrFoo // want `vow\[closable\]: vow:emit destination could not be inferred; place the marker next to a goroutine launch, callback argument, storage assignment, channel send, or a wrapped return registered through a passthrough-emit preset`
	return x
}
