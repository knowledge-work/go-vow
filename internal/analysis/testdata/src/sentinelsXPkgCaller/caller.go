// Package sentinelsXPkgCaller is the caller-side fixture for the
// cross-package must-consume check. sentinelsXPkgCallee exports a fact
// on each var it marks with vow:define @Sentinel, so a reference here is
// classified the same way a reference in the declaring package is. The
// subject is named the way the reference spells it, and vow:emit,
// vow:cond, and vow:use in this package list it under that name.
package sentinelsXPkgCaller

import (
	"errors"
	"fmt"

	"sentinelsXPkgCallee"
)

// leakAcross returns the imported sentinel without observing it or
// declaring the propagation.
func leakAcross() error {
	return sentinelsXPkgCallee.ErrShared // want `vow\[sentinel-error\]: sentinel error sentinelsXPkgCallee.ErrShared leaked: needs observation or explicit propagation`
}

// plainAcross returns an imported error var that carries no marker, so
// no fact reaches this package and the return stays silent.
func plainAcross() error {
	return sentinelsXPkgCallee.ErrPlain
}

// wrappedAcross wraps the imported sentinel and returns the result. The
// reference is an argument outside any conditional, as it is when the
// declaring package wraps its own sentinel.
func wrappedAcross() error {
	return fmt.Errorf("lookup: %w", sentinelsXPkgCallee.ErrShared) // want `vow\[sentinel-error\]: sentinel error sentinelsXPkgCallee.ErrShared leaked: needs observation or explicit propagation`
}

// aliasAcross returns the imported sentinel through a single-define
// local alias. The leak is reported at the returned alias.
func aliasAcross() error {
	x := sentinelsXPkgCallee.ErrShared
	return x // want `vow\[sentinel-error\]: sentinel error sentinelsXPkgCallee.ErrShared leaked: needs observation or explicit propagation`
}

// aliasPassedAcross hands the imported sentinel, through a local alias,
// to a call that does not observe it. The leak is reported at the
// alias, as it is when the declaring package does the same.
func aliasPassedAcross() {
	x := sentinelsXPkgCallee.ErrShared
	fmt.Println(x) // want `vow\[sentinel-error\]: sentinel error sentinelsXPkgCallee.ErrShared leaked: needs observation or explicit propagation`
}

// chainedAliasAcross reaches the return through two aliases.
func chainedAliasAcross() error {
	x := sentinelsXPkgCallee.ErrShared
	y := x
	return y // want `vow\[sentinel-error\]: sentinel error sentinelsXPkgCallee.ErrShared leaked: needs observation or explicit propagation`
}

// observedByIsAcross discharges the imported sentinel through errors.Is
// in an if condition.
func observedByIsAcross(err error) error {
	if errors.Is(err, sentinelsXPkgCallee.ErrShared) {
		return nil
	}
	return err
}

// observedBySwitchAcross discharges the imported sentinel as a switch
// case value.
func observedBySwitchAcross(err error) bool {
	switch err {
	case sentinelsXPkgCallee.ErrShared:
		return true
	}
	return false
}

// observedByTaglessSwitchAcross discharges the imported sentinel through
// errors.Is in the case list of a tagless switch, the shape a handler
// that maps errors to status codes takes.
func observedByTaglessSwitchAcross(err error) int {
	switch {
	case errors.Is(err, sentinelsXPkgCallee.ErrShared):
		return 404
	}
	return 500
}

// emitAuthorisedAcross declares the propagation with the qualified
// spelling, which both authorises the return and self-validates.
//
// vow:emit sentinelsXPkgCallee.ErrShared
func emitAuthorisedAcross() error {
	return sentinelsXPkgCallee.ErrShared
}

// condAuthorisedAcross declares the propagation through a vow:cond sum
// that lists the qualified spelling.
//
// vow:cond * -> sentinelsXPkgCallee.ErrShared | nil
func condAuthorisedAcross() error {
	return sentinelsXPkgCallee.ErrShared
}

// bareNameNotAuthorisedAcross lists the unqualified name, which this
// package does not use for the imported sentinel. The marker neither
// self-validates nor authorises the return, so both are reported.
//
// vow:emit ErrShared
func bareNameNotAuthorisedAcross() error { // want `vow\[sentinel-error\]: vow:emit ErrShared declaration is not self-validated: function body never returns ErrShared`
	return sentinelsXPkgCallee.ErrShared // want `vow\[sentinel-error\]: sentinel error sentinelsXPkgCallee.ErrShared leaked: needs observation or explicit propagation`
}

// placeholderRejectsAcross returns the imported sentinel at a position
// declared as `_ | nil`. The `_` placeholder admits any value except a
// var marked vow:define @Sentinel, and the imported sentinel is one, so
// the return breaks the sum and, with nothing authorising it, leaks.
//
// vow:cond * -> _ | nil
func placeholderRejectsAcross() error {
	return sentinelsXPkgCallee.ErrShared // want `vow\[sentinel-error\]: return position 0: returning sentinelsXPkgCallee.ErrShared but the position only accepts _ \| nil` `vow\[sentinel-error\]: sentinel error sentinelsXPkgCallee.ErrShared leaked: needs observation or explicit propagation`
}

// isShared transduces the imported sentinel to bool. The vow:use marker
// names it with the qualified spelling, and the errors.Is call in the
// body self-validates the declaration.
//
// vow:use sentinelsXPkgCallee.ErrShared
func isShared(err error) bool {
	if errors.Is(err, sentinelsXPkgCallee.ErrShared) {
		return true
	}
	return false
}

// observedByTransducerAcross discharges the imported sentinel by passing
// it to the transducer inside an if condition.
func observedByTransducerAcross(err error) error {
	if isShared(sentinelsXPkgCallee.ErrShared) {
		return nil
	}
	return err
}

// lineUseAssertedAcross asserts the use on the returning statement with
// the qualified spelling.
func lineUseAssertedAcross() error {
	// vow:use sentinelsXPkgCallee.ErrShared
	return sentinelsXPkgCallee.ErrShared
}
